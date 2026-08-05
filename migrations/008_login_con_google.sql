-- SGRC — Ingreso con cuenta de Google
--
-- Hasta acá toda cuenta tenía contraseña, y el esquema lo daba por hecho:
-- password_hash era NOT NULL. Una cuenta de Google no tiene contraseña —
-- quien la verifica es Google, no nosotros— así que la columna pasa a ser
-- opcional y aparece google_sub al lado.
--
-- Las dos son opcionales por separado, pero no las dos a la vez: una fila
-- sin ninguna de las dos sería una cuenta por la que no se puede entrar de
-- ninguna forma, y nada más adelante lo detectaría. De ahí el CHECK.
--
-- Que sean independientes (y no un enum "proveedor = LOCAL | GOOGLE") es a
-- propósito: un docente que ya se registró con contraseña y después entra
-- con Google queda con las dos, sin perder la que tenía. Un enum obligaría
-- a elegir una y a inventar un tercer valor "AMBAS" que no significa nada
-- distinto de "tiene las dos columnas llenas".

BEGIN;

-- ══════════════════════════════════════════════════════════════════
-- 1. La contraseña deja de ser obligatoria
-- ══════════════════════════════════════════════════════════════════
ALTER TABLE usuario ALTER COLUMN password_hash DROP NOT NULL;

-- ══════════════════════════════════════════════════════════════════
-- 2. Identificador de la cuenta de Google
-- ══════════════════════════════════════════════════════════════════
-- Es el claim `sub` del ID token: el identificador estable de la cuenta.
-- Se guarda eso y no el email porque el email de una cuenta de Google
-- puede cambiar y el sub no. El email sigue siendo la identidad dentro del
-- sistema (es por donde el Admin reconoce a la persona), pero el vínculo
-- con Google cuelga del sub.
ALTER TABLE usuario ADD COLUMN IF NOT EXISTS google_sub VARCHAR(255);

-- UNIQUE parcial y no UNIQUE a secas: en Postgres un UNIQUE común deja
-- pasar cualquier cantidad de NULL, así que las dos formas funcionan, pero
-- el índice parcial no indexa las filas de las cuentas con contraseña —
-- que hoy son todas.
CREATE UNIQUE INDEX IF NOT EXISTS idx_usuario_google_sub
    ON usuario (google_sub) WHERE google_sub IS NOT NULL;

-- ══════════════════════════════════════════════════════════════════
-- 3. Toda cuenta tiene que tener al menos una forma de entrar
-- ══════════════════════════════════════════════════════════════════
ALTER TABLE usuario DROP CONSTRAINT IF EXISTS chk_usuario_credencial;

ALTER TABLE usuario
    ADD CONSTRAINT chk_usuario_credencial
    CHECK (password_hash IS NOT NULL OR google_sub IS NOT NULL);

COMMIT;
