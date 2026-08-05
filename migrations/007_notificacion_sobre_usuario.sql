-- SGRC — De quién habla una notificación
--
-- El aviso "X se registró y está pendiente de aprobación" queda sin sentido
-- apenas alguien aprueba o rechaza a X: el Admin ya hizo lo que el aviso le
-- pedía. Pero seguía ahí, sin leer, hasta que además se acordara de marcarlo
-- a mano — y con varios Admin, cada uno tenía que hacerlo por su cuenta.
--
-- Para poder cerrarlo solo hace falta saber a qué cuenta se refiere. Hasta
-- ahora la notificación solo podía apuntar a una reserva (`reserva_id`).

BEGIN;

-- `usuario_id` es a QUIÉN le llega el aviso; `sobre_usuario_id` es DE QUIÉN
-- habla. En el aviso de una cuenta pendiente, el primero es cada Admin y el
-- segundo es la persona que se registró.
--
-- ON DELETE CASCADE y no SET NULL: si la cuenta de la que habla el aviso se
-- elimina definitivamente (RF-01.9), el aviso pierde todo sentido — no es
-- como `reserva_id`, donde el texto sigue diciendo algo aunque la reserva ya
-- no exista.
ALTER TABLE notificacion
    ADD COLUMN IF NOT EXISTS sobre_usuario_id UUID
        REFERENCES usuario(id) ON DELETE CASCADE;

-- Las filas viejas quedan en NULL: no se puede inferir de quién hablaban sin
-- adivinar leyendo el texto. Son avisos que ya se leyeron o que el Admin va
-- a cerrar a mano, y esto no rompe nada.

-- El índice sirve al único acceso nuevo: "las notificaciones pendientes que
-- hablan de esta persona", que es lo que se marca como leído al aprobarla.
CREATE INDEX IF NOT EXISTS idx_notif_sobre_usuario
    ON notificacion(sobre_usuario_id, tipo)
    WHERE sobre_usuario_id IS NOT NULL;

COMMIT;
