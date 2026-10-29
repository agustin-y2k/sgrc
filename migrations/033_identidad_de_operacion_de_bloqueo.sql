-- +goose Up
ALTER TABLE reserva ADD COLUMN bloqueo_operacion_id uuid;
-- Los datos históricos no permiten asegurar qué filas se crearon juntas.
UPDATE reserva SET bloqueo_operacion_id=id WHERE tipo='BLOQUEO';
ALTER TABLE reserva ADD CONSTRAINT reserva_identidad_bloqueo CHECK ((tipo='BLOQUEO')=(bloqueo_operacion_id IS NOT NULL));
CREATE INDEX reserva_bloqueo_operacion ON reserva(bloqueo_operacion_id) WHERE bloqueo_operacion_id IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION conservar_operacion_de_bloqueo() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='UPDATE' THEN
        IF OLD.bloqueo_operacion_id IS NOT NULL AND NEW.bloqueo_operacion_id IS DISTINCT FROM OLD.bloqueo_operacion_id THEN
            RAISE EXCEPTION 'La identidad de una operación de bloqueo es inmutable'
                USING ERRCODE='23514', CONSTRAINT='reserva_identidad_bloqueo';
        END IF;
    END IF;
    -- Una inserción individual sin lote explícito es su propia operación.
    IF NEW.tipo='BLOQUEO' AND NEW.bloqueo_operacion_id IS NULL THEN NEW.bloqueo_operacion_id := NEW.id; END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER reserva_identidad_bloqueo BEFORE INSERT OR UPDATE ON reserva
FOR EACH ROW EXECUTE FUNCTION conservar_operacion_de_bloqueo();

-- +goose Down
DROP TRIGGER reserva_identidad_bloqueo ON reserva;
DROP FUNCTION conservar_operacion_de_bloqueo();
DROP INDEX reserva_bloqueo_operacion;
ALTER TABLE reserva DROP CONSTRAINT reserva_identidad_bloqueo;
ALTER TABLE reserva DROP COLUMN bloqueo_operacion_id;
