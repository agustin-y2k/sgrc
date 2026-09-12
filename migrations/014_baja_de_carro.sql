-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Un carro se puede dar de baja
-- ═══════════════════════════════════════════════════════════════════════
--
-- Había `POST`, `GET` y `PATCH` de carros, y ninguna forma de sacar uno de
-- circulación. Un carro que se retira —se rompió, se devolvió, se reemplazó—
-- quedaba para siempre en el selector de la pantalla de inventario, sin
-- equipos adentro y sin nada que decir.
--
-- ── Baja LÓGICA, igual que la de un equipo ─────────────────────────────
--
-- No un DELETE, por el mismo motivo que en `equipo` (migración 005): el carro
-- aparece por nombre en el histórico de uso de cada máquina que tuvo adentro
-- (`historico_uso_equipo.carro_nombre_snapshot`), y un carro borrado dejaría
-- ese histórico hablando de algo que el sistema ya no puede explicar. Además,
-- `equipo.carro_id` no tiene ON DELETE: borrar el padre sería un error de clave
-- foránea contra un equipo que sigue vivo.
--
-- ── El nombre se libera ────────────────────────────────────────────────
--
-- Mismo criterio que la 005 para el número de serie y el zócalo: el índice
-- único pasa a ser PARCIAL sobre los carros vivos. Para el usuario, dar de baja
-- es sacar del medio, y que el nombre del carro retirado siga bloqueando el
-- del reemplazo convierte una baja en un problema a resolver con un nombre
-- inventado («Carro 1 (nuevo)»).

-- +goose Up

ALTER TABLE carro ADD COLUMN dado_de_baja BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE carro ADD COLUMN fecha_baja   TIMESTAMPTZ;

-- Las dos columnas van juntas o no van: una fecha sin la marca, o la marca sin
-- fecha, son dos formas de no saber cuándo se retiró.
ALTER TABLE carro ADD CONSTRAINT chk_carro_baja_coherente
    CHECK ((dado_de_baja = false AND fecha_baja IS NULL)
        OR (dado_de_baja = true  AND fecha_baja IS NOT NULL));

DROP INDEX ux_carro_nombre;
CREATE UNIQUE INDEX ux_carro_nombre ON carro (clave_texto(nombre))
 WHERE dado_de_baja = false;

COMMENT ON INDEX ux_carro_nombre IS
    'Un nombre por carro VIVO (RF-00.1). Parcial sobre los no dados de baja: '
    'retirar un carro libera su nombre para el que lo reemplaza, igual que la '
    '005 hizo con el número de serie y el zócalo de un equipo.';

COMMENT ON COLUMN carro.dado_de_baja IS
    'Baja lógica y no DELETE: el nombre del carro vive congelado en el '
    'histórico de uso de cada equipo que tuvo adentro, y borrarlo dejaría ese '
    'histórico hablando de algo que el sistema ya no puede explicar.';

-- +goose Down

DROP INDEX ux_carro_nombre;
CREATE UNIQUE INDEX ux_carro_nombre ON carro (clave_texto(nombre));

ALTER TABLE carro DROP CONSTRAINT chk_carro_baja_coherente;
ALTER TABLE carro DROP COLUMN fecha_baja;
ALTER TABLE carro DROP COLUMN dado_de_baja;
