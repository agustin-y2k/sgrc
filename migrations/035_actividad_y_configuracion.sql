-- +goose Up
-- NULL significa que ese cierre fue sellado antes de disponer de este resumen.
ALTER TABLE snapshot_ciclo ADD COLUMN actividad jsonb;

-- Conservar la procedencia aun cuando el cierre elimine las reservas originales.
ALTER TABLE prestamo ADD COLUMN origen text NOT NULL DEFAULT 'NO_DETERMINADO'
 CHECK (origen IN ('CON_RESERVA','SIN_RESERVA','NO_DETERMINADO'));
UPDATE prestamo p SET origen='CON_RESERVA'
WHERE p.reserva_id IS NOT NULL OR EXISTS(SELECT 1 FROM prestamo_contexto_academico ca WHERE ca.prestamo_id=p.id);
-- Un préstamo antiguo sin vínculo puede haberlo perdido en un cierre anterior.
-- No se adivina su procedencia a partir de nombres, fechas o destinos.

-- +goose StatementBegin
CREATE FUNCTION conservar_origen_prestamo() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' AND NEW.reserva_id IS NOT NULL THEN
  NEW.origen:='CON_RESERVA';
 ELSIF TG_OP='UPDATE' AND NEW.origen IS DISTINCT FROM OLD.origen THEN
  RAISE EXCEPTION 'El origen de una entrega registrada no se modifica' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER prestamo_origen_conservado BEFORE INSERT OR UPDATE ON prestamo
 FOR EACH ROW EXECUTE FUNCTION conservar_origen_prestamo();

CREATE TABLE configuracion_operacion (
 id boolean PRIMARY KEY DEFAULT true CHECK(id),
 retiro_gracia_minutos integer NOT NULL CHECK(retiro_gracia_minutos BETWEEN 1 AND 1440),
 recordatorio_antelacion_minutos integer NOT NULL CHECK(recordatorio_antelacion_minutos BETWEEN 1 AND 1440),
 cierre_gracia_minutos integer NOT NULL CHECK(cierre_gracia_minutos BETWEEN 0 AND 720),
 cierre_hora_sin_jornada integer NOT NULL CHECK(cierre_hora_sin_jornada BETWEEN 0 AND 23),
 licencias_hora_aviso integer NOT NULL CHECK(licencias_hora_aviso BETWEEN 0 AND 23),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 actualizada_en timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM configuracion_operacion) OR EXISTS(SELECT 1 FROM snapshot_ciclo WHERE actividad IS NOT NULL) THEN
  RAISE EXCEPTION 'Respaldar y revisar la configuración y los nuevos cierres antes de revertir la migración 035';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE configuracion_operacion;
DROP TRIGGER prestamo_origen_conservado ON prestamo;
DROP FUNCTION conservar_origen_prestamo();
ALTER TABLE prestamo DROP COLUMN origen;
ALTER TABLE snapshot_ciclo DROP COLUMN actividad;
