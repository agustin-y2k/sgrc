-- SGRC — Quien viene a retirar y quien responde no son la misma persona
--
-- Cuando una entrega sale contra una reserva, el préstamo se anotaba a
-- nombre de quien vino a buscar los equipos. Si el docente mandaba a otra
-- persona —lo habitual en una escuela: manda un alumno—, ese nombre
-- REEMPLAZABA al del docente en el registro.
--
-- Eso pierde el dato que importa. Quien reservó es el responsable de los
-- equipos: es quien tiene que devolverlos y a quien hay que reclamarle si no
-- vuelven, sin importar por manos de quién salieron del laboratorio. Con el
-- nombre reemplazado, el registro decía que las tenía alguien que a lo mejor
-- ni siquiera tiene cuenta en el sistema, y los avisos de devolución
-- quedaban sin destinatario.
--
-- ══════════════════════════════════════════════════════════════════
-- Una columna aparte y no un reemplazo
-- ══════════════════════════════════════════════════════════════════
-- `entregado_a_nombre` vuelve a significar siempre lo mismo: quién responde
-- por el equipo. Contra una reserva es el docente y no se elige; en una
-- entrega espontánea es quien la pide, que ahí sí responde por ella.
--
-- `retirado_por` es el dato nuevo y es OPCIONAL a propósito. Anotar al
-- alumno le sirve a una institución que quiere poder reconstruir quién pasó
-- por el mostrador, y a otra le resulta una fricción que nadie va a
-- completar. Obligarlo llevaría a que se escriba cualquier cosa para poder
-- seguir, que es peor que el vacío: parece un dato y no lo es. Vacío
-- significa exactamente "las retiró quien responde por ellas".
--
-- No es una FK a `usuario`: casi nunca es alguien con cuenta —un alumno, una
-- preceptora—, y exigirlo obligaría a crear cuentas para poder anotar un
-- nombre. Mismo criterio que `entregado_a_nombre` desde la 013.
--
-- El CHECK impide las dos formas de escribir "nada" que no son NULL, para
-- que exista UN valor que signifique "no se anotó" y no tres.

BEGIN;

ALTER TABLE prestamo
    ADD COLUMN IF NOT EXISTS retirado_por VARCHAR(200)
        CHECK (retirado_por IS NULL OR (retirado_por <> '' AND retirado_por = btrim(retirado_por)));

COMMENT ON COLUMN prestamo.retirado_por IS
    'Quién vino físicamente a buscar el equipo, si no fue quien responde por '
    'él. NULL = lo retiró la misma persona de entregado_a_nombre. No cambia '
    'de quién es la responsabilidad: eso lo dice entregado_a_nombre.';

COMMIT;
