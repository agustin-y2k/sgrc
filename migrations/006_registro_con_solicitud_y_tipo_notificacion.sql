-- SGRC — Qué pide el docente al registrarse, y de qué se trata cada aviso
--
-- Dos cambios que vienen del uso real del sistema:
--
--  1. Al aprobar a un docente, el Admin no tenía forma de saber qué materia
--     y curso le corresponden: tenía que preguntárselo por fuera del
--     sistema. Ahora eso viaja con el registro.
--
--  2. Las notificaciones eran solo texto, así que la pantalla no podía
--     ofrecer "ir a aprobar" en el aviso de un docente pendiente sin
--     adivinar por el contenido del mensaje. Un `tipo` explícito le da a la
--     interfaz algo estable de lo que colgarse: cambiar la redacción de un
--     mensaje deja de romper un botón.

BEGIN;

-- ══════════════════════════════════════════════════════════════════
-- 1. Lo que el docente declara al registrarse
-- ══════════════════════════════════════════════════════════════════
-- Texto libre y nullable a propósito. Al momento de registrarse la persona
-- todavía no está autenticada, así que no puede elegir de una lista: el
-- curso o la materia que va a dictar pueden no existir todavía en el
-- sistema (y de hecho el Admin quizás los tenga que crear al aprobarla).
-- Es una declaración de intención para que el Admin sepa a qué asignarla,
-- no una referencia a nada.
ALTER TABLE usuario
    ADD COLUMN IF NOT EXISTS curso_solicitado    VARCHAR(100),
    ADD COLUMN IF NOT EXISTS materia_solicitada  VARCHAR(100);

-- ══════════════════════════════════════════════════════════════════
-- 2. De qué se trata cada notificación
-- ══════════════════════════════════════════════════════════════════
-- Las filas que ya existen quedan como GENERAL: no se puede inferir su tipo
-- del texto sin adivinar, y un aviso viejo sin botón es correcto — el botón
-- es una comodidad, no información que se pierda.
ALTER TABLE notificacion
    ADD COLUMN IF NOT EXISTS tipo VARCHAR(30) NOT NULL DEFAULT 'GENERAL';

ALTER TABLE notificacion
    DROP CONSTRAINT IF EXISTS chk_notificacion_tipo;

ALTER TABLE notificacion
    ADD CONSTRAINT chk_notificacion_tipo
    CHECK (tipo IN ('GENERAL', 'DOCENTE_PENDIENTE', 'RESERVA_CANCELADA'));

COMMIT;
