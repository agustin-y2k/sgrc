-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Un curso es un año, más una división y una modalidad opcionales
-- ═══════════════════════════════════════════════════════════════════════
--
-- Hasta acá el nombre de un curso era `{año}°{división}` con las dos partes
-- obligatorias, y el sistema lo COMPONÍA al crearlo y lo DESARMABA con un
-- substring cuando necesitaba el año. Eso funciona en una escuela media con
-- años y divisiones, y sólo ahí.
--
-- La realidad tiene más formas. Una secundaria técnica agrupa sus cursos
-- superiores por MODALIDAD —Construcción, Electromecánica, Bachiller en
-- Economía—, con distinta cantidad de años cada una. Un terciario o una
-- universidad agrupa por CARRERA y muchas veces no divide sus cursos: su
-- segundo año es "2°" a secas. Una primaria no tiene modalidades y llega a 7°.
--
-- El principio que ordena esto: **el sistema no debe exigir un concepto que la
-- institución no tiene.** Pero el AÑO lo tienen todos —primaria, secundaria,
-- terciario y universidad organizan sus cursos por año—, así que es lo único
-- obligatorio:
--
--   anio       OBLIGATORIO  1, 2, 3…            todos los ámbitos lo tienen
--   division   opcional     "A", "2", "1ra"     una universidad puede no tenerla
--   modalidad  opcional     "Electromecánica"   una primaria no tiene ninguna
--
-- ── Por qué la modalidad NO es una tabla padre ─────────────────────────
--
-- Porque en una técnica los tres primeros años son ciclo básico común y la
-- especialidad empieza en 4°: 1°, 2° y 3° no pertenecen a ninguna modalidad.
-- Un padre obligaría a inventarles una ("Ciclo Básico") o a soportar igual el
-- curso sin padre — y entonces la tabla no simplificó nada: agregó una pantalla
-- de ABM, un paso más en el clonado de fin de año y un caso especial en cada
-- formulario, a cambio de un dato que se escribe una vez por año.
--
-- Va como columna opcional del curso, igual que el tipo de un equipo o la
-- categoría de una incidencia: **lo que la institución escribe es texto libre;
-- lo que el sistema interpreta es un enum.**
--
-- ── El nombre pasa a ser una columna GENERADA ──────────────────────────
--
-- Con el año obligatorio, el nombre ya no es un dato que alguien escribe: es
-- la consecuencia de los otros dos. `4°2` es el año 4 con la división 2, y `2°`
-- es el año 2 sin división. Que lo calcule la base tiene dos ventajas sobre
-- calcularlo en la aplicación: no puede desincronizarse, y los cincuenta y
-- pico de lugares que muestran `curso.nombre` siguen leyendo exactamente lo
-- mismo sin enterarse de nada.
--
-- La modalidad NO entra en el nombre. Es una agrupación, no parte de cómo se
-- llama el curso, y se muestra al lado —"4°2 · Electromecánica"— sólo donde
-- hace falta distinguir dos cursos homónimos de carreras distintas.
--
-- ── Qué pasa con los cursos ya cargados ────────────────────────────────
--
-- Nada se pierde: el año y la división salen de partir el nombre por el grado,
-- que es exactamente de donde el sistema los sacaba con un substring. Un `1°A`
-- queda como año 1, división A, y su nombre se vuelve a armar igual.

-- +goose Up

-- ── curso ─────────────────────────────────────────────────────────────

ALTER TABLE curso ADD COLUMN anio SMALLINT;
ALTER TABLE curso ADD COLUMN division VARCHAR(12);
ALTER TABLE curso ADD COLUMN modalidad VARCHAR(80);

-- El formato viejo garantizaba año en el primer carácter y división desde el
-- tercero, así que las dos partes salen sin ambigüedad.
UPDATE curso
   SET anio     = substring(nombre from 1 for 1)::smallint,
       division = NULLIF(substring(nombre from 3), '');

ALTER TABLE curso ALTER COLUMN anio SET NOT NULL;

-- El tope es de sanidad y no una regla de dominio: existe para que un dedazo
-- no cargue un año 400, no para decidir cuántos años tiene una carrera. Eso lo
-- sabe la institución, que crea los cursos que existen. Por lo mismo llega
-- hasta 15 y no hasta 6: una primaria tiene 7° y una carrera de grado también.
ALTER TABLE curso ADD CONSTRAINT curso_anio_check
    CHECK (anio BETWEEN 1 AND 15);

ALTER TABLE curso ADD CONSTRAINT curso_division_check
    CHECK (division IS NULL OR (division <> '' AND division = btrim(division)));

ALTER TABLE curso ADD CONSTRAINT curso_modalidad_check
    CHECK (modalidad IS NULL OR (modalidad <> '' AND modalidad = btrim(modalidad)));

-- El nombre deja de ser un dato y pasa a calcularse. Hay que sacarlo y volver
-- a crearlo: una columna común no se convierte en generada.
DROP INDEX ux_curso_ciclo_nombre;
ALTER TABLE curso DROP CONSTRAINT curso_nombre_check;
ALTER TABLE curso DROP COLUMN nombre;

ALTER TABLE curso ADD COLUMN nombre VARCHAR(20)
    GENERATED ALWAYS AS (anio::text || '°' || coalesce(division, '')) STORED;

-- Las formas canónicas, con el mismo translate() que materia.nombre_norm: sin
-- unaccent(), que vive en una extensión y no es IMMUTABLE, así que no sirve ni
-- en una columna generada ni en un índice.
--
-- El COALESCE a cadena vacía no es cosmético: convierte "sin división" y "sin
-- modalidad" en un valor más, que compite con los otros en el índice único sin
-- necesidad de NULLS NOT DISTINCT.
ALTER TABLE curso ADD COLUMN division_norm VARCHAR(12)
    GENERATED ALWAYS AS (translate(lower(coalesce(division, '')), 'áéíóúüñ', 'aeiouun')) STORED;

ALTER TABLE curso ADD COLUMN modalidad_norm VARCHAR(80)
    GENERATED ALWAYS AS (translate(lower(coalesce(modalidad, '')), 'áéíóúüñ', 'aeiouun')) STORED;

-- La unicidad va sobre las PARTES y no sobre el nombre: es lo mismo, porque el
-- nombre se deriva de ellas, y así el índice no depende de una columna
-- generada (Postgres no deja que una generada dependa de otra).
--
-- Incluye la modalidad, y eso es lo que permite que dos carreras tengan cada
-- una su "1°A". Es también lo que obliga a mostrar la modalidad al lado del
-- nombre donde se elige un curso.
CREATE UNIQUE INDEX ux_curso_ciclo_nombre
    ON curso (ciclo_lectivo_id, modalidad_norm, anio, division_norm);

COMMENT ON COLUMN curso.anio IS
    'El año del curso. OBLIGATORIO: primaria, secundaria, terciario y '
    'universidad organizan sus cursos por año. El tope es de sanidad, no una '
    'regla de dominio.';

COMMENT ON COLUMN curso.division IS
    'La división, comisión o sección: "A", "2", "1ra". NULL cuando la '
    'institución no divide sus cursos, que es lo habitual en una universidad.';

COMMENT ON COLUMN curso.modalidad IS
    'La agrupación de más arriba, si existe: modalidad, orientación o carrera. '
    'NULL = la institución no tiene ninguna, o este curso no pertenece a una '
    '(el ciclo básico de una técnica).';

COMMENT ON COLUMN curso.nombre IS
    'Generada: año + grado + división ("4°2", "2°", "1°A"). No se escribe — es '
    'la consecuencia de las otras dos columnas, y calcularla acá evita que se '
    'desincronice de ellas.';

-- ── equipo_preferencia ────────────────────────────────────────────────
--
-- La marca acotaba por año y división. Se le suma la modalidad, que en una
-- técnica es el eje más útil de los tres: "esta máquina es de Electromecánica"
-- dice más que "esta es de todo 3er año".
--
-- Los tres quedan INDEPENDIENTES: se va la regla "una división sin año no
-- significa nada", porque cada eje se sostiene solo. "Matemática de
-- Electromecánica" es un alcance válido sin decir de qué año.

ALTER TABLE equipo_preferencia ADD COLUMN modalidad VARCHAR(80);

ALTER TABLE equipo_preferencia ADD CONSTRAINT equipo_preferencia_modalidad_check
    CHECK (modalidad IS NULL OR (modalidad <> '' AND modalidad = btrim(modalidad)));

ALTER TABLE equipo_preferencia ADD COLUMN modalidad_norm VARCHAR(80)
    GENERATED ALWAYS AS (translate(lower(coalesce(modalidad, '')), 'áéíóúüñ', 'aeiouun')) STORED;

ALTER TABLE equipo_preferencia ADD COLUMN division_norm VARCHAR(12)
    GENERATED ALWAYS AS (translate(lower(coalesce(division, '')), 'áéíóúüñ', 'aeiouun')) STORED;

ALTER TABLE equipo_preferencia DROP CONSTRAINT chk_equipo_preferencia_alcance;

-- El mismo tope que el curso, por la misma razón.
ALTER TABLE equipo_preferencia DROP CONSTRAINT equipo_preferencia_anio_check;

ALTER TABLE equipo_preferencia ADD CONSTRAINT equipo_preferencia_anio_check
    CHECK (anio IS NULL OR anio BETWEEN 1 AND 15);

-- NULLS NOT DISTINCT sigue haciendo falta por `anio`, que es el único de los
-- tres ejes que puede ser NULL: las otras dos columnas del índice son generadas
-- y convierten el NULL en cadena vacía.
ALTER TABLE equipo_preferencia DROP CONSTRAINT ux_equipo_preferencia;

ALTER TABLE equipo_preferencia ADD CONSTRAINT ux_equipo_preferencia
    UNIQUE NULLS NOT DISTINCT (equipo_id, materia_norm, modalidad_norm, anio, division_norm);

COMMENT ON TABLE equipo_preferencia IS
    'Marcas de preferencia del inventario (RF-03.21). Sólo ordenan la lista '
    'al reservar: no restringen a nadie ni afectan ninguna reserva existente. '
    'El alcance se acota por modalidad, año y división, los tres opcionales.';

-- +goose Down

-- Ojo: esta vuelta atrás PIERDE INFORMACIÓN y puede fallar.
--
-- Pierde la modalidad de cada curso y de cada marca: el modelo viejo no tiene
-- dónde guardarla.
--
-- Y puede fallar por dos motivos, los dos consecuencia de lo que la 009
-- habilita. El CHECK viejo del nombre exige `^[1-6]°\S`: un curso de 7° o sin
-- división —"7°A", "2°"— no lo cumple. Y el UNIQUE viejo no mira la modalidad,
-- así que dos carreras con su "1°A" chocan entre sí. En los dos casos hay que
-- resolverlo a mano antes de bajar de versión.

ALTER TABLE equipo_preferencia DROP CONSTRAINT ux_equipo_preferencia;
ALTER TABLE equipo_preferencia DROP COLUMN division_norm;
ALTER TABLE equipo_preferencia DROP COLUMN modalidad_norm;
ALTER TABLE equipo_preferencia DROP COLUMN modalidad;

ALTER TABLE equipo_preferencia DROP CONSTRAINT equipo_preferencia_anio_check;

ALTER TABLE equipo_preferencia ADD CONSTRAINT equipo_preferencia_anio_check
    CHECK (anio >= 1 AND anio <= 6);

ALTER TABLE equipo_preferencia ADD CONSTRAINT chk_equipo_preferencia_alcance
    CHECK (division IS NULL OR anio IS NOT NULL);

ALTER TABLE equipo_preferencia ADD CONSTRAINT ux_equipo_preferencia
    UNIQUE NULLS NOT DISTINCT (equipo_id, materia_norm, anio, division);

DROP INDEX ux_curso_ciclo_nombre;

ALTER TABLE curso DROP COLUMN modalidad_norm;
ALTER TABLE curso DROP COLUMN division_norm;
ALTER TABLE curso DROP COLUMN nombre;

ALTER TABLE curso ADD COLUMN nombre VARCHAR(16);

UPDATE curso SET nombre = anio::text || '°' || coalesce(division, '');

ALTER TABLE curso ALTER COLUMN nombre SET NOT NULL;

ALTER TABLE curso ADD CONSTRAINT curso_nombre_check
    CHECK (nombre ~ '^[1-6]°\S' AND nombre = btrim(nombre));

CREATE UNIQUE INDEX ux_curso_ciclo_nombre ON curso (ciclo_lectivo_id, upper(nombre));

ALTER TABLE curso DROP CONSTRAINT curso_modalidad_check;
ALTER TABLE curso DROP CONSTRAINT curso_division_check;
ALTER TABLE curso DROP CONSTRAINT curso_anio_check;
ALTER TABLE curso DROP COLUMN modalidad;
ALTER TABLE curso DROP COLUMN division;
ALTER TABLE curso DROP COLUMN anio;
