-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Índices para las claves foráneas que se pagan al borrar
-- ═══════════════════════════════════════════════════════════════════════
--
-- Postgres indexa sola la columna REFERENCIADA de una clave foránea (es la PK),
-- pero NO la que referencia. Sin ese índice, cada borrado del padre obliga a un
-- recorrido completo del hijo para cumplir el ON DELETE: una vez por fila
-- borrada.
--
-- Con la escuela recién cargada no se nota. Se nota en las dos operaciones que
-- borran de a muchas, que son justo las que corren desatendidas:
--
--   1. **Cerrar el año** (RF-02.4) borra TODAS las reservas del ciclo. Cada una
--      dispara un recorrido de `notificacion` para poner en NULL las que la
--      nombran — y `notificacion` es la tabla que más rápido crece del sistema,
--      porque nada la limpia nunca.
--
--      Medido sobre esta base, con 16.000 avisos cargados:
--
--        |  reservas borradas | sin índice | con índice |
--        |--------------------|------------|------------|
--        |                400 |     4,5 s  |    0,88 s  |
--        |              1.600 |    18,7 s  |    1,03 s  |
--
--      Lo que importa no es el 5× de la primera fila sino la forma de las dos
--      columnas: cuadruplicar las reservas cuadruplica el tiempo SIN índice y
--      deja el otro casi igual. El trabajo inevitable —poner en NULL los avisos
--      que de verdad apuntan a algo— no depende de cuántas reservas se borren;
--      el recorrido de la tabla, sí, una vez por cada una.
--
--   2. **Eliminar una cuenta** (RF-01.9) toca DIEZ columnas que apuntan a
--      `usuario`, entre ellas tres de `prestamo` y una de `reserva`. Son diez
--      recorridos de las tablas más grandes para borrar una sola fila.
--
-- ── Qué NO se indexa, y por qué ────────────────────────────────────────
--
-- Un índice no es gratis: se mantiene en cada INSERT y cada UPDATE de la tabla.
-- Dos de las quince foráneas sin índice se dejan como están:
--
--   - `historico_uso_equipo.equipo_id` — un equipo NUNCA se borra de verdad.
--     La baja es lógica (`equipo.dado_de_baja`), así que esta foránea no se
--     ejerce jamás en un DELETE. Y la tabla se consulta sólo por `anio`, que ya
--     tiene índice. El índice sería costo puro.
--
--   - `historico_uso_docente.usuario_id` — acá el padre SÍ se borra, pero la
--     tabla tiene una fila por docente por año: cientos, no millones. Un
--     recorrido de eso, en una operación que pasa una vez cada tanto, no se
--     mide. También se consulta sólo por `anio`.
--
-- El criterio, entonces, no es «toda foránea lleva índice» sino «lleva índice
-- la foránea cuyo padre se borra y cuyo hijo crece».

-- +goose Up

-- ── El que paga el cierre del año ──────────────────────────────────────

CREATE INDEX idx_notificacion_reserva ON notificacion (reserva_id)
 WHERE reserva_id IS NOT NULL;

COMMENT ON INDEX idx_notificacion_reserva IS
    'Para el ON DELETE SET NULL al borrar las reservas del ciclo que se cierra '
    '(RF-02.4). Parcial: la mayoría de los avisos no habla de ninguna reserva.';

-- ── Los que pagan la eliminación de una cuenta (RF-01.9) ───────────────
--
-- Las tres de `prestamo` van juntas porque el borrado las recorre a las tres:
-- indexar dos de tres deja la operación atada a la que falta.

CREATE INDEX idx_prestamo_entregado_a ON prestamo (entregado_a_usuario_id)
 WHERE entregado_a_usuario_id IS NOT NULL;
CREATE INDEX idx_prestamo_entregado_por ON prestamo (entregado_por)
 WHERE entregado_por IS NOT NULL;
CREATE INDEX idx_prestamo_recibido_por ON prestamo (recibido_por)
 WHERE recibido_por IS NOT NULL;

CREATE INDEX idx_reserva_cancelado_por ON reserva (cancelado_por)
 WHERE cancelado_por IS NOT NULL;

CREATE INDEX idx_incidencia_reportado_por ON incidencia (reportado_por)
 WHERE reportado_por IS NOT NULL;

CREATE INDEX idx_licencia_vencimiento_fijado_por ON licencia_software (vencimiento_fijado_por)
 WHERE vencimiento_fijado_por IS NOT NULL;

CREATE INDEX idx_sugerencia_mensaje_autor ON sugerencia_mensaje (autor_id)
 WHERE autor_id IS NOT NULL;

CREATE INDEX idx_regla_recurrencia_creado_por ON regla_recurrencia (creado_por)
 WHERE creado_por IS NOT NULL;

CREATE INDEX idx_pedido_materia_resuelto_por ON pedido_de_materia (resuelto_por)
 WHERE resuelto_por IS NOT NULL;

-- Autorreferencia: borrar una cuenta obliga a recorrer `usuario` buscando a
-- quién aprobó. La tabla es chica, pero el índice también.
CREATE INDEX idx_usuario_aprobado_por ON usuario (aprobado_por)
 WHERE aprobado_por IS NOT NULL;

-- ── Los que paga borrar una materia o un curso ─────────────────────────
--
-- Eliminar un curso arrastra sus materias en cascada (RF-02.11), y desde que la
-- estructura se carga de a muchas (RF-02.12) corregir una planilla mal armada
-- puede ser borrar veintiséis cursos con sus doscientas materias de una vez.

CREATE INDEX idx_pedido_materia_materia ON pedido_de_materia (materia_id)
 WHERE materia_id IS NOT NULL;

CREATE INDEX idx_regla_recurrencia_materia ON regla_recurrencia (materia_id);

-- ── Todos los índices de arriba son PARCIALES salvo uno ────────────────
--
-- `WHERE columna IS NOT NULL` sale gratis y a veces gana mucho. El chequeo de
-- una foránea busca siempre por un valor concreto, nunca por NULL, así que
-- excluir las filas nulas no le saca nada — y Postgres sabe deducir que
-- `col = $1` implica `col IS NOT NULL`, que es lo que le permite usar el índice
-- parcial igual (verificado con EXPLAIN sobre una sentencia preparada, no
-- asumido).
--
-- Cuánto achica depende de la columna: en `notificacion.reserva_id` y en
-- `reserva.cancelado_por` es la enorme mayoría de las filas —casi ningún aviso
-- habla de una reserva, casi ninguna reserva se cancela—; en
-- `prestamo.entregado_por` casi ninguna fila es nula y el índice queda igual de
-- grande que uno entero. En ese caso no gana nada y tampoco pierde, y la forma
-- uniforme evita tener que justificar cada uno por separado el día que cambien
-- las proporciones.
--
-- `regla_recurrencia.materia_id` es el único NOT NULL de la lista, así que va
-- entero.

-- +goose Down

DROP INDEX idx_regla_recurrencia_materia;
DROP INDEX idx_pedido_materia_materia;
DROP INDEX idx_usuario_aprobado_por;
DROP INDEX idx_pedido_materia_resuelto_por;
DROP INDEX idx_regla_recurrencia_creado_por;
DROP INDEX idx_sugerencia_mensaje_autor;
DROP INDEX idx_licencia_vencimiento_fijado_por;
DROP INDEX idx_incidencia_reportado_por;
DROP INDEX idx_reserva_cancelado_por;
DROP INDEX idx_prestamo_recibido_por;
DROP INDEX idx_prestamo_entregado_por;
DROP INDEX idx_prestamo_entregado_a;
DROP INDEX idx_notificacion_reserva;
