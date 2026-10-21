-- +goose Up
CREATE TABLE evento_pendiente (
 id bigserial PRIMARY KEY,
 tipo text NOT NULL,
 clase text NOT NULL,
 payload text NOT NULL,
 creado_en timestamptz NOT NULL DEFAULT now(),
 procesado_en timestamptz,
 intentos integer NOT NULL DEFAULT 0,
 reintentar_en timestamptz NOT NULL DEFAULT now(),
 ultimo_error text
);
CREATE INDEX evento_pendiente_por_entregar ON evento_pendiente(id) WHERE procesado_en IS NULL;
COMMENT ON COLUMN evento_pendiente.payload IS 'JSON cifrado con AES-GCM; incluye datos privados de destinatarios y códigos de recuperación. Se vacía al confirmar la entrega.';
-- +goose Down
DROP TABLE evento_pendiente;
