-- +goose Up
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM prestamo p JOIN reserva r ON r.id=p.reserva_id WHERE p.equipo_id<>r.equipo_id) THEN
        RAISE EXCEPTION 'Hay préstamos y reservas vinculados a equipos distintos; revisar esos registros antes de aplicar la migración 030';
    END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION proteger_equipo_de_reserva_entregada() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.equipo_id IS DISTINCT FROM OLD.equipo_id
       AND EXISTS (SELECT 1 FROM prestamo WHERE reserva_id=OLD.id) THEN
        RAISE EXCEPTION 'La reserva ya tuvo una entrega y debe conservar su equipo'
            USING ERRCODE='23514', CONSTRAINT='reserva_equipo_entregado';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER reserva_equipo_entregado BEFORE UPDATE OF equipo_id ON reserva
FOR EACH ROW EXECUTE FUNCTION proteger_equipo_de_reserva_entregada();

-- +goose StatementBegin
CREATE FUNCTION verificar_equipo_del_prestamo() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE equipo_reservado uuid;
BEGIN
    -- El cierre anual puede eliminar la reserva; la custodia sobrevive.
    IF NEW.reserva_id IS NULL THEN RETURN NEW; END IF;
    SELECT equipo_id INTO equipo_reservado FROM reserva WHERE id=NEW.reserva_id FOR UPDATE;
    IF FOUND AND NEW.equipo_id IS DISTINCT FROM equipo_reservado THEN
        RAISE EXCEPTION 'El equipo del préstamo no coincide con la reserva'
            USING ERRCODE='23514', CONSTRAINT='prestamo_equipo_reservado';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER prestamo_equipo_reservado BEFORE INSERT OR UPDATE OF reserva_id,equipo_id ON prestamo
FOR EACH ROW EXECUTE FUNCTION verificar_equipo_del_prestamo();

-- +goose Down
DROP TRIGGER prestamo_equipo_reservado ON prestamo;
DROP FUNCTION verificar_equipo_del_prestamo();
DROP TRIGGER reserva_equipo_entregado ON reserva;
DROP FUNCTION proteger_equipo_de_reserva_entregada();
