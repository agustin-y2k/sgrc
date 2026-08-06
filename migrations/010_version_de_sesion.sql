-- SGRC — Revocación de sesiones al cambiar la contraseña (RF-01.11)
--
-- El token lleva adentro la versión de sesión con la que se emitió, y el
-- middleware la compara contra la de la fila en el mismo request en el que
-- ya verifica el estado de la cuenta. Cambiar la contraseña incrementa el
-- contador, y todo token anterior deja de valer en el request siguiente.
--
-- Sin esto, quien cambia su contraseña porque sospecha que entraron a su
-- cuenta deja al otro adentro hasta que se le venza el token (JWT_ACCESS_TTL,
-- una hora por defecto).
--
-- Es un entero y no una marca de tiempo ("invalidar lo emitido antes de X")
-- a propósito: `iat` en un JWT tiene resolución de SEGUNDOS, así que
-- comparado contra un now() con microsegundos el token que el propio cambio
-- acaba de emitir se rechaza a sí mismo. Redondear al segundo lo arregla
-- pero abre una ventana en la que las sesiones abiertas en ese mismo
-- segundo sobreviven. Con un contador no hay nada que redondear.

BEGIN;

-- DEFAULT 0 coincide con el claim ausente en un token, así que aplicar esta
-- migración no desloguea a nadie: los tokens vigentes siguen valiendo hasta
-- expirar, y recién el primer cambio de contraseña de cada cuenta invalida
-- los suyos.
ALTER TABLE usuario
    ADD COLUMN IF NOT EXISTS version_sesion INTEGER NOT NULL DEFAULT 0;

COMMIT;
