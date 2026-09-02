-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — El barrido deja de avisar de lo que no puede saber (RF-07.6)
-- ═══════════════════════════════════════════════════════════════════════
--
-- El barrido de reservas y entregas concluía cosas a partir de lo que NO
-- estaba registrado: si a los cuarenta minutos ninguna máquina figuraba
-- entregada, daba por hecho que nadie las había ido a buscar. Eso solo es
-- cierto mientras haya un Admin operando el mostrador.
--
-- El día que el Admin falta y lo cubre un directivo o un preceptor que
-- prefiere anotar en papel, el sistema no ve "nadie vino": ve su propio
-- silencio, y lo interpreta como si los docentes no hubieran aparecido. Les
-- liberaba las reservas, les avisaba que no habían retirado nada y ensuciaba
-- el reporte de uso — todo por una ausencia que no era de ellos.
--
-- A partir de esta versión, las pasadas que deducen algo de un registro
-- faltante solo corren si algún Admin declaró estar de guardia en ese momento
-- (RF-07.1/07.4, las tablas `horario_admin` y `horario_admin_excepcion`,
-- que ya existían y hasta ahora eran puramente informativas). Sin nadie
-- atendiendo, el sistema se queda quieto: no libera, no avisa y no escribe.
--
-- Con eso, dos avisos dejan de tener sentido y se retiran enteros:
--
--   * el de "todavía no retiraste" (RF-08.20), que era el aviso previo a una
--     liberación que ahora solo ocurre cuando hay alguien que la respalde;
--   * el reclamo de devolución demorada (RF-08.12), que le reprochaba a un
--     docente no haber devuelto una máquina que quizás devolvió y nadie
--     registró. Lo que sigue afuera se ve en la pantalla de entregas, que no
--     depende de ningún correo.
--
-- Se van también dos avisos que no dependían del mostrador sino de haber
-- crecido de más: la copia al docente que ya dicta una materia que otro pidió
-- (no se le pedía nada) y el aviso de cierre al docente de la próxima reserva
-- (llegaba de madrugada, y el de "tu PC puede no estar" ya le llega una hora
-- antes de su clase, que es cuando todavía puede cambiarla).

-- +goose Up

-- ── Las marcas de los dos avisos que ya no existen ─────────────────────
--
-- Se van con su índice parcial: los dos servían a consultas que el barrido ya
-- no hace, y un índice sin lector sigue costando en cada INSERT y cada UPDATE
-- de dos de las tablas más escritas del sistema.

DROP INDEX IF EXISTS idx_grupo_sin_aviso_retiro;
ALTER TABLE reserva_grupo DROP COLUMN IF EXISTS aviso_sin_retirar_en;

DROP INDEX IF EXISTS idx_prestamo_demorados_sin_avisar;
ALTER TABLE prestamo DROP COLUMN IF EXISTS avisado_demora_en;

-- ── El tipo de notificación que se retira ──────────────────────────────
--
-- Las notificaciones viejas NO se borran: son el historial de alguien y
-- todavía dicen algo cierto sobre lo que pasó ese día. Pasan a GENERAL, que
-- es exactamente lo que son ahora — un aviso que se lee y nada más, sin
-- pantalla a la que llevar.
UPDATE notificacion SET tipo = 'GENERAL' WHERE tipo = 'RESERVA_NO_RETIRADA';

ALTER TABLE notificacion DROP CONSTRAINT chk_notificacion_tipo;
ALTER TABLE notificacion ADD CONSTRAINT chk_notificacion_tipo CHECK (tipo IN (
    'GENERAL',
    'DOCENTE_PENDIENTE',
    'RESERVA_CANCELADA',
    'LICENCIA_POR_VENCER',
    'RESERVA_POR_COMENZAR',
    'EQUIPO_SIN_DEVOLVER',
    'PEDIDO_DE_LIBERACION',
    'PEDIDO_DE_MATERIA',
    'PEDIDO_DE_MATERIA_RESUELTO',
    'SUGERENCIA',
    'SUGERENCIA_RESPONDIDA'
));

-- ── Las cuatro categorías de correo que se retiran ─────────────────────
--
-- Acá sí se borran las filas, y no es lo mismo que arriba: una preferencia no
-- es un hecho que pasó, es una decisión sobre un correo que ya no se manda.
-- Conservarla solo dejaría al panel ofreciendo tildar algo inexistente.
DELETE FROM preferencia_email WHERE categoria IN (
    'RESERVA_SIN_RETIRAR',
    'DEVOLUCION_PENDIENTE',
    'DEVOLUCION_DEMORADA',
    'PEDIDO_SOBRE_MI_MATERIA'
);

ALTER TABLE preferencia_email DROP CONSTRAINT chk_preferencia_email_categoria;
ALTER TABLE preferencia_email ADD CONSTRAINT chk_preferencia_email_categoria CHECK (categoria IN (
    'RESERVA_CANCELADA',
    'EQUIPO_NO_DISPONIBLE',
    'PEDIDO_DE_LIBERACION',
    'PEDIDO_DE_MATERIA_RESUELTO',
    'SUGERENCIA_RESPONDIDA',
    'RECORDATORIO_DE_RESERVA',
    'CUENTA_PENDIENTE',
    'LICENCIA_POR_VENCER',
    'SUGERENCIA',
    'PEDIDO_DE_MATERIA',
    'CIERRE_SIN_DEVOLVER'
));

-- +goose Down

ALTER TABLE reserva_grupo ADD COLUMN IF NOT EXISTS aviso_sin_retirar_en TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_grupo_sin_aviso_retiro
    ON reserva_grupo (fecha, hora_inicio) WHERE aviso_sin_retirar_en IS NULL AND estado = 'CONFIRMADA';

ALTER TABLE prestamo ADD COLUMN IF NOT EXISTS avisado_demora_en TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_prestamo_demorados_sin_avisar
    ON prestamo (devolucion_estimada)
    WHERE devuelto_en IS NULL AND avisado_demora_en IS NULL AND devolucion_estimada IS NOT NULL;

-- Los CHECK vuelven a admitir lo de antes. Lo que NO vuelve es el contenido:
-- las notificaciones que pasaron a GENERAL se quedan así y las preferencias
-- borradas no están. Es una vuelta atrás del esquema, no del tiempo — quien
-- baje esta migración recupera un sistema que puede volver a escribir esos
-- valores, no los que ya se convirtieron.
ALTER TABLE notificacion DROP CONSTRAINT chk_notificacion_tipo;
ALTER TABLE notificacion ADD CONSTRAINT chk_notificacion_tipo CHECK (tipo IN (
    'GENERAL',
    'DOCENTE_PENDIENTE',
    'RESERVA_CANCELADA',
    'LICENCIA_POR_VENCER',
    'RESERVA_POR_COMENZAR',
    'RESERVA_NO_RETIRADA',
    'EQUIPO_SIN_DEVOLVER',
    'PEDIDO_DE_LIBERACION',
    'PEDIDO_DE_MATERIA',
    'PEDIDO_DE_MATERIA_RESUELTO',
    'SUGERENCIA',
    'SUGERENCIA_RESPONDIDA'
));

ALTER TABLE preferencia_email DROP CONSTRAINT chk_preferencia_email_categoria;
ALTER TABLE preferencia_email ADD CONSTRAINT chk_preferencia_email_categoria CHECK (categoria IN (
    'RESERVA_CANCELADA',
    'EQUIPO_NO_DISPONIBLE',
    'PEDIDO_DE_LIBERACION',
    'PEDIDO_DE_MATERIA_RESUELTO',
    'PEDIDO_SOBRE_MI_MATERIA',
    'SUGERENCIA_RESPONDIDA',
    'RECORDATORIO_DE_RESERVA',
    'RESERVA_SIN_RETIRAR',
    'DEVOLUCION_PENDIENTE',
    'CUENTA_PENDIENTE',
    'LICENCIA_POR_VENCER',
    'SUGERENCIA',
    'PEDIDO_DE_MATERIA',
    'DEVOLUCION_DEMORADA',
    'CIERRE_SIN_DEVOLVER'
));
