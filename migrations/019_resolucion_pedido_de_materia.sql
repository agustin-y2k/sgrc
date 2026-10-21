-- +goose Up
-- La solicitud original conserva su forma (materia elegida o texto libre).
-- Los pedidos anteriores quedan sin vínculo: no se puede inferir qué materia
-- creó una aprobación antigua a partir de un nombre potencialmente repetido.
ALTER TABLE pedido_de_materia
    ADD COLUMN materia_creada_id UUID REFERENCES materia(id) ON DELETE SET NULL,
    ADD CONSTRAINT chk_pedido_materia_creada CHECK (
        materia_creada_id IS NULL OR (estado = 'APROBADO' AND materia_id IS NULL)
    );

CREATE INDEX idx_pedido_materia_creada ON pedido_de_materia (materia_creada_id)
    WHERE materia_creada_id IS NOT NULL;

-- +goose Down
ALTER TABLE pedido_de_materia DROP COLUMN materia_creada_id;
