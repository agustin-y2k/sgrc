-- +goose Up
-- Las preferencias deciden qué reservas se pueden desplazar. Sus cambios
-- requieren la misma constancia atómica y atribución que el resto del inventario.
CREATE TRIGGER auditoria_sensible AFTER INSERT OR UPDATE OR DELETE
ON equipo_preferencia FOR EACH ROW EXECUTE FUNCTION auditar_cambio_sensible();

-- +goose Down
-- Se conserva audit_log, incluidas las constancias de preferencias anteriores.
DROP TRIGGER auditoria_sensible ON equipo_preferencia;
