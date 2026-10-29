-- +goose Up
ALTER TABLE materia ADD COLUMN interna_de_espacio boolean NOT NULL DEFAULT false;
ALTER TABLE materia ADD CONSTRAINT materia_interna_con_espacio CHECK (NOT interna_de_espacio OR espacio_id IS NOT NULL);
CREATE UNIQUE INDEX materia_interna_por_espacio ON materia(espacio_id) WHERE interna_de_espacio;

UPDATE materia m SET interna_de_espacio=true FROM espacio e
WHERE m.espacio_id=e.id AND clave_texto(m.nombre)=clave_texto(e.nombre);
-- Un espacio que perdió su materia vuelve a ser reservable. No se alteran
-- materias adicionales ni se reconstruyen asignaciones o reservas eliminadas.
INSERT INTO materia(id,espacio_id,nombre,archivado,interna_de_espacio)
SELECT gen_random_uuid(),e.id,e.nombre,e.archivado,true FROM espacio e
WHERE NOT EXISTS (SELECT 1 FROM materia m WHERE m.espacio_id=e.id AND m.interna_de_espacio);

-- +goose StatementBegin
CREATE FUNCTION proteger_materia_interna_de_espacio() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN
        -- El borrado del espacio sí puede propagar su eliminación.
        IF OLD.interna_de_espacio AND EXISTS (SELECT 1 FROM espacio WHERE id=OLD.espacio_id) THEN
            RAISE EXCEPTION 'La materia interna se elimina desde Espacios'
                USING ERRCODE='23514', CONSTRAINT='materia_interna_de_espacio';
        END IF;
        RETURN OLD;
    END IF;
    IF TG_OP='UPDATE' THEN
        IF OLD.interna_de_espacio AND (NOT NEW.interna_de_espacio OR NEW.espacio_id IS DISTINCT FROM OLD.espacio_id) THEN
            RAISE EXCEPTION 'La materia interna debe conservar su espacio'
                USING ERRCODE='23514', CONSTRAINT='materia_interna_de_espacio';
        END IF;
    END IF;
    IF NEW.interna_de_espacio AND NOT EXISTS (SELECT 1 FROM espacio WHERE id=NEW.espacio_id AND nombre=NEW.nombre) THEN
        RAISE EXCEPTION 'La materia interna se renombra desde Espacios'
            USING ERRCODE='23514', CONSTRAINT='materia_interna_de_espacio';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- Canonicaliza el nombre de las coincidencias anteriores a la protección.
UPDATE materia m SET nombre=e.nombre FROM espacio e WHERE m.espacio_id=e.id AND m.interna_de_espacio;
CREATE TRIGGER materia_interna_de_espacio BEFORE INSERT OR UPDATE OR DELETE ON materia
FOR EACH ROW EXECUTE FUNCTION proteger_materia_interna_de_espacio();

-- +goose Down
DROP TRIGGER materia_interna_de_espacio ON materia;
DROP FUNCTION proteger_materia_interna_de_espacio();
DROP INDEX materia_interna_por_espacio;
ALTER TABLE materia DROP CONSTRAINT materia_interna_con_espacio;
ALTER TABLE materia DROP COLUMN interna_de_espacio;
