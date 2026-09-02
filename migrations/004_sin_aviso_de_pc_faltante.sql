-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Se retira el aviso de "una PC tuya no volvió al laboratorio"
-- ═══════════════════════════════════════════════════════════════════════
--
-- El aviso salía una hora antes de la clase y le decía al docente que una de
-- las computadoras que había reservado seguía fuera del laboratorio.
--
-- Le pedía resolver algo que no puede resolver. El docente no entrega ni
-- recibe equipos: llega al mostrador y se le da lo que hay. Quien resuelve la
-- falta es el `ADMIN`, en el momento de la entrega y sin necesidad de que
-- nadie le avise —lo ve en su propia pantalla—, cambiando la máquina que no
-- está por otra libre. El aviso solo lograba que el docente llegara preocupado
-- a una clase que iba a tener sus computadoras igual.
--
-- Con él se van su categoría de correo y el tipo de notificación
-- RESERVA_POR_COMENZAR, que ya no usa nadie: el recordatorio de "en un rato
-- tenés clase" había dejado de escribir en la campana en la 003.
--
-- ── Y los correos personales pasan a arrancar APAGADOS ─────────────────
--
-- Hasta acá, cinco categorías personales venían encendidas de fábrica con el
-- argumento de que traen noticias de algo que hizo otro. En la práctica eso
-- llena la casilla de gente que nunca lo pidió, y el aviso está igual en la
-- campana, que es la fuente de verdad y no se puede apagar. Un correo que
-- nadie pidió se archiva sin leer, y entrena a no mirar los que sí importan.
--
-- Ahora **de fábrica solo salen los cuatro correos fijos** —los dos de la
-- cuenta y los dos de soporte— **más cuentas pendientes**, que es la única
-- excepción y no por quien lo recibe sino por quien lo sufre: un docente que
-- no puede entrar hasta que un Admin lo mire.
--
-- Lo que cada persona ya eligió a mano se respeta: esta migración NO toca
-- `preferencia_email` salvo para borrar la categoría que deja de existir.
-- Quien tildó algo lo sigue teniendo tildado; lo que cambia es el valor para
-- quien nunca abrió el panel, que vive en el código.

-- +goose Up

-- La marca del aviso. No tiene índice propio que borrar: se leía dentro de la
-- consulta general de reservas a vigilar, no en una consulta suya.
ALTER TABLE reserva DROP COLUMN IF EXISTS avisado_equipo_no_disponible_en;

-- Las notificaciones viejas se conservan, como en la 003: son el historial de
-- alguien y siguen diciendo algo cierto sobre lo que pasó ese día.
UPDATE notificacion SET tipo = 'GENERAL' WHERE tipo = 'RESERVA_POR_COMENZAR';

ALTER TABLE notificacion DROP CONSTRAINT chk_notificacion_tipo;
ALTER TABLE notificacion ADD CONSTRAINT chk_notificacion_tipo CHECK (tipo IN (
    'GENERAL',
    'DOCENTE_PENDIENTE',
    'RESERVA_CANCELADA',
    'LICENCIA_POR_VENCER',
    'EQUIPO_SIN_DEVOLVER',
    'PEDIDO_DE_LIBERACION',
    'PEDIDO_DE_MATERIA',
    'PEDIDO_DE_MATERIA_RESUELTO',
    'SUGERENCIA',
    'SUGERENCIA_RESPONDIDA'
));

DELETE FROM preferencia_email WHERE categoria = 'EQUIPO_NO_DISPONIBLE';

ALTER TABLE preferencia_email DROP CONSTRAINT chk_preferencia_email_categoria;
ALTER TABLE preferencia_email ADD CONSTRAINT chk_preferencia_email_categoria CHECK (categoria IN (
    'RESERVA_CANCELADA',
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

ALTER TABLE reserva ADD COLUMN IF NOT EXISTS avisado_equipo_no_disponible_en TIMESTAMPTZ;

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
