-- SGRC — Instantes con zona horaria
--
-- Las doce columnas de abajo representan **instantes**: el momento en que
-- algo pasó. Estaban declaradas TIMESTAMP (sin zona), y como el proceso
-- lee la hora en la zona de la escuela (APP_TIMEZONE), lo que terminaba
-- guardado era la hora de pared local. Al leerla de vuelta, pgx la
-- interpretaba como UTC y la API la serializaba con sufijo Z:
--
--   instante real   03:31 -03:00  (= 06:31 UTC)
--   Postgres        2026-08-01 03:31:37
--   API             2026-08-01T03:31:37Z   ← miente: no es UTC
--   navegador       00:31                  ← tres horas antes
--
-- Nadie lo había notado porque ninguna pantalla mostraba una marca de
-- tiempo. La primera que lo hace es el listado de notificaciones.
--
-- TIMESTAMPTZ guarda el instante de verdad: Postgres normaliza a UTC al
-- escribir y devuelve un valor con zona al leer, así que la conversión
-- deja de depender de que nadie se olvide de hacerla. `now()` sigue
-- funcionando como default, y ahora coincide con lo que escribe la app.
--
-- IMPORTANTE: `USING columna AT TIME ZONE 'America/Argentina/Buenos_Aires'`
-- es lo que hace correcta la conversión de los datos que ya existen. Sin
-- esa cláusula, Postgres asumiría que los valores guardados están en la
-- zona de la sesión y, si esa zona no es la de la escuela, correría cada
-- fila. Se nombra la zona explícitamente en vez de confiar en la
-- configuración del servidor.
--
-- Lo que NO se toca: reserva.fecha (DATE) y hora_inicio/hora_fin (TIME).
-- Esos sí son hora de pared de la institución a propósito — "de 8 a 9"
-- significa lo mismo en enero que en julio, sin importar la zona. Ver
-- docs/07-modelo-datos.md.

BEGIN;

-- usuario
ALTER TABLE usuario
    ALTER COLUMN fecha_registro TYPE TIMESTAMPTZ
        USING fecha_registro AT TIME ZONE 'America/Argentina/Buenos_Aires',
    ALTER COLUMN fecha_aprobacion TYPE TIMESTAMPTZ
        USING fecha_aprobacion AT TIME ZONE 'America/Argentina/Buenos_Aires';

-- pc
ALTER TABLE pc
    ALTER COLUMN fecha_baja TYPE TIMESTAMPTZ
        USING fecha_baja AT TIME ZONE 'America/Argentina/Buenos_Aires',
    ALTER COLUMN fecha_alta TYPE TIMESTAMPTZ
        USING fecha_alta AT TIME ZONE 'America/Argentina/Buenos_Aires';

-- incidencia
ALTER TABLE incidencia
    ALTER COLUMN fecha TYPE TIMESTAMPTZ
        USING fecha AT TIME ZONE 'America/Argentina/Buenos_Aires',
    ALTER COLUMN fecha_envio_dge TYPE TIMESTAMPTZ
        USING fecha_envio_dge AT TIME ZONE 'America/Argentina/Buenos_Aires';

-- reserva_grupo
ALTER TABLE reserva_grupo
    ALTER COLUMN creada_en TYPE TIMESTAMPTZ
        USING creada_en AT TIME ZONE 'America/Argentina/Buenos_Aires';

-- reserva
ALTER TABLE reserva
    ALTER COLUMN creada_en TYPE TIMESTAMPTZ
        USING creada_en AT TIME ZONE 'America/Argentina/Buenos_Aires',
    ALTER COLUMN cancelada_en TYPE TIMESTAMPTZ
        USING cancelada_en AT TIME ZONE 'America/Argentina/Buenos_Aires';

-- notificacion
ALTER TABLE notificacion
    ALTER COLUMN creada_en TYPE TIMESTAMPTZ
        USING creada_en AT TIME ZONE 'America/Argentina/Buenos_Aires',
    ALTER COLUMN leida_en TYPE TIMESTAMPTZ
        USING leida_en AT TIME ZONE 'America/Argentina/Buenos_Aires';

-- audit_log
ALTER TABLE audit_log
    ALTER COLUMN creado_en TYPE TIMESTAMPTZ
        USING creado_en AT TIME ZONE 'America/Argentina/Buenos_Aires';

COMMIT;
