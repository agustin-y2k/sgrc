-- SGRC — Liberar la reserva que nadie vino a buscar, y reclamar la que no volvió
--
-- Segunda mitad de RF-08. La 013 dejó el registro de entregas; esta trae lo
-- que el sistema hace SOLO, sin que nadie apriete nada:
--
--   * a los 40 minutos de empezada la clase, una PC que nadie retiró deja de
--     bloquear el horario y otro docente puede usarla;
--   * una hora antes, al docente le llega un recordatorio con la regla;
--   * si una máquina no volvió a horario, se le reclama.
--
-- ══════════════════════════════════════════════════════════════════
-- Liberar es un cambio de estado, y nada más
-- ══════════════════════════════════════════════════════════════════
-- La constraint de anti-solapamiento de la 001 tiene
-- `WHERE (estado = 'CONFIRMADA')`. O sea que sacar una reserva de ese estado
-- ya la saca del camino: el horario queda libre sin tocar la constraint, sin
-- borrar nada y sin ninguna consulta especial. Por eso alcanza con un estado
-- nuevo.
--
-- NO_RETIRADA y no CANCELADA a propósito: son dos noticias distintas para
-- quien las lee. "Te la cancelaron" pide saber quién y por qué; "no la
-- retiraste" ya se explica sola, y además el reporte de uso (RF-06) puede
-- dejar de contarla como una clase dada, que es lo que hoy hace.
--
-- ══════════════════════════════════════════════════════════════════
-- Las marcas de aviso guardan CUÁNDO, no un booleano
-- ══════════════════════════════════════════════════════════════════
-- Mismo criterio que las licencias (012): el barrido corre cada pocos
-- minutos, y lo que impide que mande el mismo aviso dos veces es que la fila
-- recuerde que ya salió. Un timestamp además deja saber a qué hora fue, que
-- es lo primero que se pregunta cuando alguien dice "a mí no me llegó".

BEGIN;

-- ══════════════════════════════════════════════════════════════════
-- 1. El estado nuevo
-- ══════════════════════════════════════════════════════════════════

ALTER TABLE reserva DROP CONSTRAINT IF EXISTS reserva_estado_check;
ALTER TABLE reserva
    ADD CONSTRAINT reserva_estado_check
    CHECK (estado IN ('CONFIRMADA','CANCELADA','FINALIZADA','NO_RETIRADA'));

-- A nivel grupo solo se marca si NINGUNA de sus PCs se retiró. Si el docente
-- vino y se llevó tres de cinco, el grupo sigue como estaba: vino a dar la
-- clase, y lo que pasó con las otras dos máquinas se ve fila por fila.
ALTER TABLE reserva_grupo DROP CONSTRAINT IF EXISTS reserva_grupo_estado_check;
ALTER TABLE reserva_grupo
    ADD CONSTRAINT reserva_grupo_estado_check
    CHECK (estado IN ('CONFIRMADA','PARCIALMENTE_CANCELADA','CANCELADA','FINALIZADA','NO_RETIRADA'));

-- ══════════════════════════════════════════════════════════════════
-- 2. Las marcas de aviso
-- ══════════════════════════════════════════════════════════════════

-- El recordatorio de "en una hora tenés reserva" es UNO por grupo, no uno
-- por PC: el docente vive la clase como una sola cosa. Y como cada grupo
-- tiene una única fecha y horario, alcanza con saber si ya salió — no hace
-- falta apuntar "para qué fecha", como sí pasa con las licencias, que
-- reviven en cada renovación.
ALTER TABLE reserva_grupo
    ADD COLUMN IF NOT EXISTS recordatorio_enviado_en TIMESTAMPTZ;

-- El aviso de "una de tus PCs no volvió y puede no estar" va POR RESERVA y
-- no por préstamo, y esa es la diferencia que importa: la misma máquina
-- demorada toda la mañana afecta a la clase de las 10 y también a la de las
-- 12, y a las dos hay que avisarles. Con la marca del lado del préstamo,
-- solo se enteraría la primera.
ALTER TABLE reserva
    ADD COLUMN IF NOT EXISTS avisado_pc_no_disponible_en TIMESTAMPTZ;

-- Al Admin se le avisa una vez por préstamo demorado: es un reclamo sobre
-- una máquina puntual y repetirlo cada barrido lo volvería ruido.
ALTER TABLE prestamo
    ADD COLUMN IF NOT EXISTS avisado_demora_en TIMESTAMPTZ;

-- El corte de fin de jornada sí se repite: si la máquina sigue afuera
-- mañana, vuelve a aparecer en el listado del cierre. Por eso la marca es
-- la FECHA de la jornada avisada y no un timestamp — un día, un aviso.
ALTER TABLE prestamo
    ADD COLUMN IF NOT EXISTS avisado_cierre_para DATE;

-- Los tres accesos del barrido. Parciales porque en los tres casos lo que se
-- busca es la excepción: casi ninguna reserva está sin recordar en este
-- momento, y casi ningún préstamo está sin avisar.
CREATE INDEX IF NOT EXISTS idx_grupo_sin_recordar
    ON reserva_grupo(fecha, hora_inicio)
    WHERE recordatorio_enviado_en IS NULL AND estado = 'CONFIRMADA';

CREATE INDEX IF NOT EXISTS idx_reserva_confirmadas_del_dia
    ON reserva(fecha, hora_inicio)
    WHERE estado = 'CONFIRMADA';

CREATE INDEX IF NOT EXISTS idx_prestamo_demorados_sin_avisar
    ON prestamo(devolucion_estimada)
    WHERE devuelto_en IS NULL AND avisado_demora_en IS NULL AND devolucion_estimada IS NOT NULL;

-- ══════════════════════════════════════════════════════════════════
-- 3. Los tipos de aviso
-- ══════════════════════════════════════════════════════════════════
-- Igual que en la 006 y la 012: el tipo existe para que la pantalla ofrezca
-- el botón que corresponde sin leer el texto del mensaje.
--
-- RESERVA_POR_COMENZAR cubre dos avisos que llevan al mismo lado —el
-- recordatorio y el "una de tus PCs puede no estar"— porque para el docente
-- la acción es la misma: mirar su reserva.
ALTER TABLE notificacion DROP CONSTRAINT IF EXISTS chk_notificacion_tipo;
ALTER TABLE notificacion
    ADD CONSTRAINT chk_notificacion_tipo
    CHECK (tipo IN (
        'GENERAL',
        'DOCENTE_PENDIENTE',
        'RESERVA_CANCELADA',
        'LICENCIA_POR_VENCER',
        'RESERVA_POR_COMENZAR',
        'RESERVA_NO_RETIRADA',
        'PC_SIN_DEVOLVER'
    ));

COMMENT ON COLUMN reserva.estado IS
    'NO_RETIRADA: nadie vino a buscar la máquina dentro del plazo de gracia, '
    'así que dejó de bloquear el horario. No es una cancelación (ver 014).';

COMMENT ON COLUMN prestamo.avisado_cierre_para IS
    'Jornada para la que ya salió el aviso de cierre. Es una fecha y no un '
    'booleano porque el corte se repite cada día que la máquina siga afuera.';

COMMIT;
