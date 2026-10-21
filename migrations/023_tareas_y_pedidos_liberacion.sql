-- +goose Up
ALTER TABLE notificacion ADD COLUMN pedido_id uuid, ADD COLUMN hilo_id uuid;
CREATE INDEX notificacion_tarea_pendiente ON notificacion(tipo,pedido_id,hilo_id) WHERE estado='NO_LEIDA';
-- Los avisos antiguos no identifican la tarea: conservarlos como historia y
-- reconstruir los pendientes actuales por pedido/hilo, sin enviar correos.
UPDATE notificacion SET estado='LEIDA', leida_en=now()
WHERE tipo IN ('PEDIDO_DE_MATERIA','SUGERENCIA') AND estado='NO_LEIDA';
INSERT INTO notificacion(usuario_id,sobre_usuario_id,pedido_id,mensaje,tipo)
SELECT a.id,p.usuario_id,p.id,'Pedido pendiente: ' || COALESCE(m.nombre,p.materia_solicitada),'PEDIDO_DE_MATERIA'
FROM pedido_de_materia p LEFT JOIN materia m ON m.id=p.materia_id
CROSS JOIN usuario a WHERE p.estado='PENDIENTE' AND a.rol='ADMIN' AND a.estado='APROBADA';
INSERT INTO notificacion(usuario_id,sobre_usuario_id,hilo_id,mensaje,tipo)
SELECT a.id,s.usuario_id,s.id,'Conversación pendiente: ' || s.asunto,'SUGERENCIA'
FROM sugerencia s CROSS JOIN usuario a
WHERE s.estado='ABIERTA' AND a.rol='ADMIN' AND a.estado='APROBADA'
AND NOT COALESCE((SELECT de_admin FROM sugerencia_mensaje WHERE sugerencia_id=s.id ORDER BY escrito_en DESC,id DESC LIMIT 1),false);
CREATE TABLE pedido_liberacion (
 reserva_id uuid NOT NULL REFERENCES reserva(id) ON DELETE CASCADE,
 solicitante_id uuid NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
 fecha date NOT NULL,
 creado_en timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(reserva_id,solicitante_id,fecha)
);
CREATE INDEX pedido_liberacion_por_solicitante ON pedido_liberacion(solicitante_id);
-- +goose Down
DROP TABLE pedido_liberacion;
DROP INDEX notificacion_tarea_pendiente;
ALTER TABLE notificacion DROP COLUMN pedido_id,DROP COLUMN hilo_id;
