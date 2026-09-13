-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Dirección, Biblioteca y Preceptoría no son cursos
-- ═══════════════════════════════════════════════════════════════════════
--
-- En una escuela trabaja gente que no da clase frente a un curso: el
-- bibliotecario, el preceptor, alguien de Dirección. Hasta acá el sistema no
-- tenía dónde ponerlos — todo lo que existe cuelga de un `curso`, y un curso es
-- un AÑO, obligatorio (migración 009).
--
-- ── Por qué NO se relajó el curso ──────────────────────────────────────
--
-- La salida corta era hacer `curso.anio` opcional y cargar "Biblioteca" como un
-- curso sin año. Se descartó: `anio` es obligatorio a propósito —lo tienen
-- todos los ámbitos, de la primaria a la universidad— y aflojarlo para meter
-- algo que no es un curso deja la puerta abierta a crear cursos sin año por
-- error, que es exactamente lo que esa regla evita. El concepto nuevo se
-- nombra, no se disfraza.
--
-- ── Qué SÍ comparten ───────────────────────────────────────────────────
--
-- Materias. En la Biblioteca puede haber «Apoyo escolar» o «Taller de
-- lectura», y se reserva para esas igual que para Matemática: una reserva
-- sigue teniendo una materia detrás, y nada del módulo de reservas cambia.
--
-- Por eso `materia` pasa a colgar de UNO de los dos, con un CHECK que lo
-- garantiza. Y por eso existe la vista de abajo.
--
-- ── La vista, que es la parte importante ───────────────────────────────
--
-- Hay QUINCE consultas en el código con la forma
-- `JOIN curso c ON c.id = m.curso_id`, todas INNER. Con `curso_id` opcional,
-- cada una de las quince descarta EN SILENCIO las materias de un espacio: no
-- fallan, no avisan, simplemente devuelven de menos. Una materia de la
-- Biblioteca desaparecería de las reservas, de los reportes y de las
-- validaciones sin un solo error en ningún lado.
--
-- `contenedor_de_materia` le da a cada materia su contenedor —sea curso o
-- espacio— con los mismos nombres de columna que esas consultas ya usaban
-- (`nombre`, `modalidad`, `ciclo_lectivo_id`). Así el cambio en cada una es
-- reemplazar el JOIN, y no queda ningún lugar donde alguien pueda olvidarse de
-- la otra mitad.

-- +goose Up

CREATE TABLE espacio (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ciclo_lectivo_id UUID NOT NULL REFERENCES ciclo_lectivo(id),
    -- Nombre libre: es lo único que lo identifica. No hay año, división ni
    -- modalidad que componer — «Biblioteca» es «Biblioteca».
    nombre           VARCHAR(80) NOT NULL,
    archivado        BOOLEAN NOT NULL DEFAULT FALSE
);

ALTER TABLE espacio ADD CONSTRAINT chk_espacio_nombre_no_vacio
    CHECK (btrim(nombre) <> '');

-- Misma regla de unicidad que el resto del sistema (migración 011): dos
-- nombres son el mismo si coinciden sin tildes, sin mayúsculas y sin espacios
-- de más. «Biblioteca» y «biblioteca » no son dos.
CREATE UNIQUE INDEX ux_espacio_ciclo_nombre
    ON espacio (ciclo_lectivo_id, clave_texto(nombre));

CREATE INDEX idx_espacio_ciclo ON espacio (ciclo_lectivo_id);

-- ── La materia cuelga de uno de los dos ────────────────────────────────

ALTER TABLE materia ADD COLUMN espacio_id UUID
    REFERENCES espacio(id) ON DELETE CASCADE;

ALTER TABLE materia ALTER COLUMN curso_id DROP NOT NULL;

-- Exactamente uno. Ni los dos —una materia no se dicta en un curso Y en la
-- biblioteca al mismo tiempo— ni ninguno, que dejaría una materia huérfana sin
-- ciclo lectivo del que colgar.
ALTER TABLE materia ADD CONSTRAINT chk_materia_un_solo_contenedor
    CHECK (num_nonnulls(curso_id, espacio_id) = 1);

CREATE INDEX idx_materia_espacio ON materia (espacio_id);

-- La unicidad del nombre, del lado del espacio. El índice de curso
-- (ux_materia_curso_nombre) ya no cubre estas filas: con curso_id en NULL, un
-- índice único no las compara entre sí.
CREATE UNIQUE INDEX ux_materia_espacio_nombre
    ON materia (espacio_id, clave_texto(nombre))
    WHERE espacio_id IS NOT NULL;

-- ── El contenedor de cada materia, sea cual sea ────────────────────────

CREATE VIEW contenedor_de_materia AS
    SELECT m.id                 AS materia_id,
           -- Se llama `id` y no `contenedor_id` con toda intención: las
           -- consultas que antes hacían `JOIN curso c` seleccionan `c.id`, y
           -- así el reemplazo es una línea por consulta y no una revisión de
           -- cada columna. Lo que identifica a la fila de esta vista es
           -- `materia_id`.
           c.id                 AS id,
           'CURSO'::text        AS tipo,
           c.nombre             AS nombre,
           c.modalidad          AS modalidad,
           -- `anio` va en la vista porque varias consultas ordenan por él. Un
           -- espacio no tiene: queda NULL, y los ORDER BY que ya decían
           -- `anio NULLS LAST` lo mandan al final sin tocarlos.
           c.anio               AS anio,
           -- Las formas canónicas las usa el ordenamiento de equipos por
           -- preferencia de materia (RF-03.21), que compara el alcance de una
           -- marca con el curso de la reserva. Un espacio no tiene modalidad ni
           -- división: van en '' —el mismo valor que un curso sin ninguna— así
           -- que una marca acotada por año o división no lo alcanza, que es
           -- justo lo correcto: esa marca habla de un curso.
           c.modalidad_norm     AS modalidad_norm,
           c.division_norm      AS division_norm,
           c.ciclo_lectivo_id   AS ciclo_lectivo_id,
           c.archivado          AS archivado
      FROM materia m
      JOIN curso c ON c.id = m.curso_id
    UNION ALL
    SELECT m.id,
           e.id,
           'ESPACIO'::text,
           e.nombre,
           NULL::varchar,
           NULL::smallint,
           ''::varchar,
           ''::varchar,
           e.ciclo_lectivo_id,
           e.archivado
      FROM materia m
      JOIN espacio e ON e.id = m.espacio_id;

COMMENT ON VIEW contenedor_de_materia IS
    'De dónde cuelga cada materia: un curso o un espacio. Existe para que las '
    'consultas que antes hacían JOIN curso no pierdan en silencio las materias '
    'de un espacio — con curso_id opcional, un INNER JOIN las descarta sin '
    'avisar. Las columnas se llaman igual que las de curso para que el cambio '
    'en cada consulta sea reemplazar el JOIN y nada más.';

-- +goose Down

DROP VIEW contenedor_de_materia;
DROP INDEX ux_materia_espacio_nombre;
DROP INDEX idx_materia_espacio;
ALTER TABLE materia DROP CONSTRAINT chk_materia_un_solo_contenedor;

-- Antes de volver a exigir curso_id hay que sacar lo que no lo tiene: son las
-- materias de un espacio, que en el esquema viejo no tienen dónde vivir.
DELETE FROM materia WHERE espacio_id IS NOT NULL;

ALTER TABLE materia ALTER COLUMN curso_id SET NOT NULL;
ALTER TABLE materia DROP COLUMN espacio_id;

DROP TABLE espacio;
