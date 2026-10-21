-- +goose Up
ALTER TABLE reserva ADD COLUMN version BIGINT NOT NULL DEFAULT 0 CHECK (version >= 0);
ALTER TABLE reserva_grupo ADD COLUMN version BIGINT NOT NULL DEFAULT 0 CHECK (version >= 0);

-- +goose Down
ALTER TABLE reserva_grupo DROP COLUMN version;
ALTER TABLE reserva DROP COLUMN version;
