-- +goose Up
-- La reasignación de varios equipos valida el resultado completo del lote.
-- Las operaciones habituales siguen verificando inmediatamente cada escritura.
ALTER TABLE reserva DROP CONSTRAINT no_solapamiento;
ALTER TABLE reserva ADD CONSTRAINT no_solapamiento EXCLUDE USING gist (
    equipo_id WITH =,
    tsrange(fecha + hora_inicio, fin_de_pared(fecha, hora_inicio, hora_fin)) WITH &&
) WHERE (estado = 'CONFIRMADA') DEFERRABLE INITIALLY IMMEDIATE;

-- +goose Down
ALTER TABLE reserva DROP CONSTRAINT no_solapamiento;
ALTER TABLE reserva ADD CONSTRAINT no_solapamiento EXCLUDE USING gist (
    equipo_id WITH =,
    tsrange(fecha + hora_inicio, fin_de_pared(fecha, hora_inicio, hora_fin)) WITH &&
) WHERE (estado = 'CONFIRMADA');
