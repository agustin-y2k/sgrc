-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — La misma regla de unicidad para todos los nombres del sistema
-- ═══════════════════════════════════════════════════════════════════════
--
-- La 010 resolvió el nombre de una materia. Al revisar el resto del esquema
-- apareció que la MISMA regla estaba escrita con tres rigores distintos, y que
-- ninguno de los otros dos alcanza:
--
--   | tabla                        | comparaba por      | dejaba entrar          |
--   |------------------------------|--------------------|------------------------|
--   | curso, materia, preferencia  | normalizado        | —                      |
--   | equipo suelto, licencia      | lower() y nada más | tildes, espacios       |
--   | carro, numero_serie          | texto exacto       | todo                   |
--
-- «Carro 1», «carro 1» y «Carro  1» eran tres carros, cada uno con sus
-- equipos. El caso no es hipotético: `NuevoCarro` validaba el nombre RECORTADO
-- y después guardaba el crudo, así que un nombre con espacios al borde entraba
-- con los espacios puestos, y el único por texto exacto lo aceptaba.
--
-- Que la regla valga en unos lugares y no en otros es peor que no tenerla: el
-- Admin aprende que el sistema le impide duplicar, y confía en eso también
-- donde no es cierto.
--
-- ── Una sola función, no cuatro expresiones ────────────────────────────
--
-- `clave_texto()` reemplaza a `clave_materia()` de la 010 y la usan los cinco
-- índices. El motivo es el de siempre: una expresión repetida funciona hasta
-- que alguien toca una copia. Con una función, que se separen deja de ser
-- posible, y el espejo en Go es un solo lugar (`internal/shared/texto`).
--
-- ── Qué se hace con lo que ya está cargado ─────────────────────────────
--
-- Lo mismo que la 010, y por el mismo motivo: primero se canonizan los
-- espacios —un espacio doble es un accidente, no un dato— y después se
-- desambigua lo que siga repitiéndose, RENOMBRANDO y nunca borrando. Un carro
-- duplicado tiene equipos colgando, una licencia tiene avisos, un equipo tiene
-- reservas e incidencias; una migración que corre sola al arrancar el
-- contenedor no puede llevarse eso puesto.
--
-- El sufijo « (2)» deja el duplicado VISIBLE en la pantalla para que un Admin
-- decida a mano cuál sobrevive. Renombrar es reversible; borrar no.

-- +goose Up

-- Los DOS translate() no son uno solo por comodidad: el primero saca tildes y
-- el segundo convierte a un espacio común TODO lo que cuenta como espacio.
--
-- Ese segundo paso existe por algo que encontró el test de paridad contra Go y
-- no una lectura del código: el `\s` de Postgres NO matchea el espacio duro
-- (U+00A0), y Go sí lo consideraba espacio. O sea que la base dejaba pasar un
-- texto que Go ya había colapsado, y las dos definiciones —que sostienen los
-- mismos cinco índices— opinaban distinto sobre el caso más difícil de ver de
-- todos: el espacio duro se imprime igual que uno común.
--
-- Convertidos todos a un espacio común, btrim y el colapso trabajan sobre un
-- solo carácter y el resultado es idéntico del lado de Go
-- (internal/shared/texto.esEspacio tiene esta misma lista).
-- +goose StatementBegin
CREATE FUNCTION clave_texto(t text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    RETURN regexp_replace(
               btrim(
                   translate(
                       translate(lower(t), 'áéíóúüñ', 'aeiouun'),
                       chr(160) || chr(9) || chr(10) || chr(11) || chr(12) || chr(13),
                       '      '   -- seis espacios, uno por carácter de arriba
                   )
               ),
               ' +', ' ', 'g');
-- +goose StatementEnd

-- canonizar_texto es el texto tal como se GUARDA: mismo tratamiento de espacios
-- que clave_texto, pero conservando la caja y las tildes. Es el espejo de
-- texto.Canonizar en Go.
--
-- Son dos funciones y no una porque son dos preguntas distintas: «¿cómo se
-- escribe esto?» y «¿esto y aquello son la misma cosa?». Guardar la clave sería
-- mostrarle al Admin sus carros en minúsculas y sin tildes.
-- +goose StatementBegin
CREATE FUNCTION canonizar_texto(t text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    RETURN regexp_replace(
               btrim(
                   translate(t,
                       chr(160) || chr(9) || chr(10) || chr(11) || chr(12) || chr(13),
                       '      '
                   )
               ),
               ' +', ' ', 'g');
-- +goose StatementEnd

COMMENT ON FUNCTION clave_texto(text) IS
    'Cómo se comparan dos textos para decidir si nombran la MISMA cosa: sin '
    'tildes, sin mayúsculas y con los espacios colapsados. La sostiene esta '
    'función y no una expresión repetida en cada índice, para que no puedan '
    'separarse. Su espejo en Go es internal/shared/texto.Clave.';

-- ── materia: pasa de clave_materia a la función general ────────────────
--
-- Mismo criterio, otro nombre: no hay dos reglas, hay una. El índice se rehace
-- porque una función no se puede renombrar sin invalidar lo que la indexa.

DROP INDEX ux_materia_curso_nombre;
DROP FUNCTION clave_materia(text);

-- La 010 ya canonizó estos nombres, pero con la expresión que NO ve el espacio
-- duro. Se repasan con la buena antes de rehacer el índice.
UPDATE materia
   SET nombre = canonizar_texto(nombre)
 WHERE nombre <> canonizar_texto(nombre);
CREATE UNIQUE INDEX ux_materia_curso_nombre ON materia (curso_id, clave_texto(nombre));

COMMENT ON INDEX ux_materia_curso_nombre IS
    'Una materia por curso (RF-02.3), comparando con clave_texto().';

-- ── carro ──────────────────────────────────────────────────────────────

-- El único viejo se va PRIMERO, antes de tocar los nombres, y ése es el orden
-- que importa: canonizar puede convertir «Carro  1» en el mismo texto exacto
-- que «Carro 1», que es justo lo que ese único prohíbe. Al revés, la migración
-- fallaría contra el índice VIEJO, y el mensaje nombraría al que está por
-- desaparecer en vez de al que decide la regla nueva.
ALTER TABLE carro DROP CONSTRAINT carro_nombre_key;

UPDATE carro
   SET nombre = canonizar_texto(nombre)
 WHERE nombre <> canonizar_texto(nombre);

UPDATE carro c
   SET nombre = left(d.nombre, 94) || ' (' || d.n || ')'
  FROM (
        SELECT id, nombre,
               row_number() OVER (
                   PARTITION BY clave_texto(nombre)
                   -- El que tiene más equipos adentro conserva el nombre: es el
                   -- que el Admin reconoce, y el que aparece en la pantalla de
                   -- reservas.
                   ORDER BY (SELECT count(*) FROM equipo e WHERE e.carro_id = cc.id) DESC,
                            nombre, id
               ) AS n
          FROM carro cc
       ) AS d
 WHERE c.id = d.id AND d.n > 1;

CREATE UNIQUE INDEX ux_carro_nombre ON carro (clave_texto(nombre));

-- ── equipo suelto: el nombre sólo distingue a los que no están en un carro ──
--
-- Los de carro se identifican por su zócalo (`identificador`), no por nombre,
-- así que el índice sigue siendo parcial sobre los sueltos y vivos — igual que
-- antes, sólo cambia la expresión de lower() a clave_texto().

DROP INDEX ux_equipo_suelto_nombre;

UPDATE equipo
   SET nombre = canonizar_texto(nombre)
 WHERE nombre IS NOT NULL AND nombre <> canonizar_texto(nombre);

UPDATE equipo e
   SET nombre = left(d.nombre, 94) || ' (' || d.n || ')'
  FROM (
        SELECT id, nombre,
               row_number() OVER (
                   PARTITION BY clave_texto(nombre)
                   -- El que tiene historial se queda con el nombre: es el que
                   -- aparece en préstamos y reservas ya hechos.
                   ORDER BY (SELECT count(*) FROM prestamo p WHERE p.equipo_id = ee.id) DESC,
                            nombre, id
               ) AS n
          FROM equipo ee
         WHERE carro_id IS NULL AND dado_de_baja = false AND nombre IS NOT NULL
       ) AS d
 WHERE e.id = d.id AND d.n > 1;

CREATE UNIQUE INDEX ux_equipo_suelto_nombre
    ON equipo (clave_texto(nombre))
 WHERE carro_id IS NULL AND dado_de_baja = false;

-- ── número de serie ────────────────────────────────────────────────────
--
-- Se tipea mirando una etiqueta pegada a la máquina, así que sale con la
-- cantidad de espacios que salga. Go ya lo guardaba en mayúsculas; lo que
-- faltaba era que la base comparara igual.
--
-- Acá NO se desambigua con un sufijo: una serie inventada es peor que una
-- repetida, porque deja de coincidir con la etiqueta física. Si dos equipos
-- vivos comparten serie después de canonizar, la migración FALLA y lo dice —
-- son dos máquinas mal cargadas y eso lo resuelve una persona, no un UPDATE.

DROP INDEX ux_equipo_numero_serie;

UPDATE equipo
   SET numero_serie = upper(canonizar_texto(numero_serie))
 WHERE numero_serie IS NOT NULL AND numero_serie <> upper(canonizar_texto(numero_serie));
CREATE UNIQUE INDEX ux_equipo_numero_serie
    ON equipo (clave_texto(numero_serie))
 WHERE dado_de_baja = false;

-- ── licencia de software ───────────────────────────────────────────────

DROP INDEX ux_licencia_equipo_nombre;

UPDATE licencia_software
   SET nombre = canonizar_texto(nombre)
 WHERE nombre <> canonizar_texto(nombre);

UPDATE licencia_software l
   SET nombre = left(d.nombre, 94) || ' (' || d.n || ')'
  FROM (
        SELECT id, nombre,
               row_number() OVER (
                   PARTITION BY equipo_id, clave_texto(nombre)
                   -- La que tiene vencimiento cargado es la que se está
                   -- siguiendo; la otra es una copia sin usar.
                   ORDER BY (fecha_vencimiento IS NULL), nombre, id
               ) AS n
          FROM licencia_software
       ) AS d
 WHERE l.id = d.id AND d.n > 1;

CREATE UNIQUE INDEX ux_licencia_equipo_nombre
    ON licencia_software (equipo_id, clave_texto(nombre));

-- ── Lo que NOT NULL no impide: la cadena vacía ─────────────────────────
--
-- `NOT NULL` deja pasar ''. Hasta acá lo impedía sólo la validación de Go, o
-- sea que la regla valía para la API y no para la base — y con dos caminos de
-- carga masiva nuevos (RF-02.12) el margen dejó de ser teórico.
--
-- Se agregan sólo sobre lo que ESCRIBE una persona y se MUESTRA como la
-- identidad de algo. Los snapshots históricos quedan afuera a propósito: son
-- copias congeladas de un texto que ya se validó cuando estaba vivo, y un CHECK
-- ahí podría hacer fallar un archivado por un dato viejo que ya no se puede
-- corregir.

ALTER TABLE carro       ADD CONSTRAINT chk_carro_nombre_no_vacio       CHECK (btrim(nombre) <> '');
ALTER TABLE materia     ADD CONSTRAINT chk_materia_nombre_no_vacio     CHECK (btrim(nombre) <> '');
ALTER TABLE incidencia  ADD CONSTRAINT chk_incidencia_desc_no_vacia    CHECK (btrim(descripcion) <> '');
ALTER TABLE usuario     ADD CONSTRAINT chk_usuario_nombre_no_vacio     CHECK (btrim(nombre) <> '' AND btrim(apellido) <> '');
ALTER TABLE usuario     ADD CONSTRAINT chk_usuario_email_no_vacio      CHECK (btrim(email) <> '');
ALTER TABLE notificacion ADD CONSTRAINT chk_notificacion_msg_no_vacio  CHECK (btrim(mensaje) <> '');

-- ── El único redundante de usuario ─────────────────────────────────────
--
-- `usuario_email_key UNIQUE (email)` es estrictamente más débil que
-- `idx_usuario_email_lower`, que ya existe y sí es único: dos emails iguales
-- también son iguales en minúsculas. Mantener los dos cuesta una escritura de
-- índice por alta y le hace creer a quien lea el esquema que la unicidad es
-- sensible a mayúsculas, que es justo lo contrario de lo que hace el login.

ALTER TABLE usuario DROP CONSTRAINT usuario_email_key;

-- +goose Down

ALTER TABLE usuario ADD CONSTRAINT usuario_email_key UNIQUE (email);

ALTER TABLE notificacion DROP CONSTRAINT chk_notificacion_msg_no_vacio;
ALTER TABLE usuario      DROP CONSTRAINT chk_usuario_email_no_vacio;
ALTER TABLE usuario      DROP CONSTRAINT chk_usuario_nombre_no_vacio;
ALTER TABLE incidencia   DROP CONSTRAINT chk_incidencia_desc_no_vacia;
ALTER TABLE materia      DROP CONSTRAINT chk_materia_nombre_no_vacio;
ALTER TABLE carro        DROP CONSTRAINT chk_carro_nombre_no_vacio;

DROP INDEX ux_licencia_equipo_nombre;
CREATE UNIQUE INDEX ux_licencia_equipo_nombre ON licencia_software (equipo_id, lower(nombre));

DROP INDEX ux_equipo_numero_serie;
CREATE UNIQUE INDEX ux_equipo_numero_serie ON equipo (numero_serie) WHERE dado_de_baja = false;

DROP INDEX ux_equipo_suelto_nombre;
CREATE UNIQUE INDEX ux_equipo_suelto_nombre ON equipo (lower(nombre))
 WHERE carro_id IS NULL AND dado_de_baja = false;

DROP INDEX ux_carro_nombre;
ALTER TABLE carro ADD CONSTRAINT carro_nombre_key UNIQUE (nombre);

-- materia vuelve a su función propia, que es como la dejó la 010.
-- +goose StatementBegin
CREATE FUNCTION clave_materia(nombre text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    RETURN regexp_replace(
               btrim(translate(lower(nombre), 'áéíóúüñ', 'aeiouun')),
               '\s+', ' ', 'g');
-- +goose StatementEnd

DROP INDEX ux_materia_curso_nombre;
CREATE UNIQUE INDEX ux_materia_curso_nombre ON materia (curso_id, clave_materia(nombre));
DROP FUNCTION clave_texto(text);
DROP FUNCTION canonizar_texto(text);

-- Los « (2)» no se deshacen, por lo mismo que en la 010: el Down recupera el
-- ESQUEMA anterior, no los nombres que la subida tuvo que desambiguar.
