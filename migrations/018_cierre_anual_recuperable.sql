-- +goose Up

-- El UUID histórico no tiene FK: sobrevive a ON DELETE SET NULL. En cuentas
-- eliminadas antes de esta migración ya no se conoce la identidad original;
-- se conserva cada grupo por separado, sin fusionar personas por su nombre.
ALTER TABLE reserva ADD COLUMN docente_id_historico UUID;
UPDATE reserva SET docente_id_historico=COALESCE(creado_por,reserva_grupo_id,id)
WHERE tipo='NORMAL';
ALTER TABLE reserva ADD CONSTRAINT chk_reserva_identidad_historica
CHECK (tipo <> 'NORMAL' OR docente_id_historico IS NOT NULL);

ALTER TABLE historico_uso_docente ADD COLUMN docente_id_historico UUID;
UPDATE historico_uso_docente SET docente_id_historico=COALESCE(usuario_id,id);
ALTER TABLE historico_uso_docente ALTER COLUMN docente_id_historico SET NOT NULL;
ALTER TABLE historico_uso_docente DROP CONSTRAINT historico_uso_docente_anio_usuario_id_key;
CREATE UNIQUE INDEX ux_historico_docente_identidad ON historico_uso_docente(anio,docente_id_historico);

CREATE TABLE snapshot_ciclo (
 ciclo_id UUID PRIMARY KEY REFERENCES ciclo_lectivo(id),
 anio INTEGER NOT NULL UNIQUE,
 sellado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
 legado BOOLEAN NOT NULL DEFAULT false
);
-- Los históricos anteriores se preservan. No se reconstruyen desde una
-- fuente que pudo haberse borrado parcial o completamente.
INSERT INTO snapshot_ciclo(ciclo_id,anio,legado)
SELECT id,anio,true FROM ciclo_lectivo WHERE archivado;

CREATE TABLE cierre_ciclo (
 ciclo_id UUID PRIMARY KEY REFERENCES ciclo_lectivo(id),
 anio_destino INTEGER,
 nuevo_ciclo_id UUID,
 cursos_clonados INTEGER NOT NULL CHECK(cursos_clonados>=0),
 materias_clonadas INTEGER NOT NULL CHECK(materias_clonadas>=0),
 completado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK ((anio_destino IS NULL)=(nuevo_ciclo_id IS NULL))
);

-- +goose StatementBegin
CREATE FUNCTION proteger_reserva_del_cierre() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE ciclo UUID; cerrado BOOLEAN;
BEGIN
 IF TG_TABLE_NAME='reserva' THEN
  IF TG_OP='UPDATE' THEN
   NEW.docente_id_historico:=OLD.docente_id_historico;
   -- Desvincular una cuenta eliminada conserva identidad y estadísticas.
   IF (NEW.creado_por IS NOT DISTINCT FROM OLD.creado_por OR NEW.creado_por IS NULL) AND
      (NEW.cancelado_por IS NOT DISTINCT FROM OLD.cancelado_por OR NEW.cancelado_por IS NULL) AND
      (to_jsonb(NEW)-ARRAY['creado_por','cancelado_por'])=(to_jsonb(OLD)-ARRAY['creado_por','cancelado_por']) THEN
    RETURN NEW;
   END IF;
  ELSIF NEW.tipo='NORMAL' THEN
   NEW.docente_id_historico:=COALESCE(NEW.creado_por,NEW.reserva_grupo_id,NEW.id);
  END IF;
  IF NEW.tipo='BLOQUEO' THEN
   SELECT id,archivado INTO ciclo,cerrado FROM ciclo_lectivo
    WHERE anio=EXTRACT(YEAR FROM NEW.fecha) FOR SHARE;
  ELSE
   SELECT cl.id,cl.archivado INTO ciclo,cerrado FROM contenedor_de_materia c
    JOIN ciclo_lectivo cl ON cl.id=c.ciclo_lectivo_id
    WHERE c.materia_id=NEW.materia_id FOR SHARE OF cl;
  END IF;
 ELSE
  IF TG_OP='UPDATE' AND NEW.creado_por IS NULL AND
     (to_jsonb(NEW)-'creado_por')=(to_jsonb(OLD)-'creado_por') THEN
   RETURN NEW;
  END IF;
  SELECT cl.id,cl.archivado INTO ciclo,cerrado FROM contenedor_de_materia c
   JOIN ciclo_lectivo cl ON cl.id=c.ciclo_lectivo_id
   WHERE c.materia_id=NEW.materia_id FOR SHARE OF cl;
 END IF;
 IF cerrado OR EXISTS(SELECT 1 FROM snapshot_ciclo WHERE ciclo_id=ciclo) THEN
  RAISE EXCEPTION 'el ciclo de la reserva está cerrado'
   USING ERRCODE='23514',CONSTRAINT='reserva_ciclo_cerrado';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER reserva_cierre BEFORE INSERT OR UPDATE ON reserva
FOR EACH ROW EXECUTE FUNCTION proteger_reserva_del_cierre();
CREATE TRIGGER reserva_grupo_cierre BEFORE INSERT OR UPDATE ON reserva_grupo
FOR EACH ROW EXECUTE FUNCTION proteger_reserva_del_cierre();

-- +goose Down
DROP TRIGGER reserva_grupo_cierre ON reserva_grupo;
DROP TRIGGER reserva_cierre ON reserva;
DROP FUNCTION proteger_reserva_del_cierre();
DROP TABLE cierre_ciclo;
DROP TABLE snapshot_ciclo;
-- Conservar separadas las identidades históricas al volver al esquema previo;
-- las cuentas activas siguen teniendo como máximo una fila por año.
DROP INDEX ux_historico_docente_identidad;
ALTER TABLE historico_uso_docente ADD CONSTRAINT historico_uso_docente_anio_usuario_id_key UNIQUE(anio,usuario_id);
ALTER TABLE historico_uso_docente DROP COLUMN docente_id_historico;
ALTER TABLE reserva DROP CONSTRAINT chk_reserva_identidad_historica;
ALTER TABLE reserva DROP COLUMN docente_id_historico;
