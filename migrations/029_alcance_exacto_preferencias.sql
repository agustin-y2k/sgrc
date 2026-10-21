-- +goose Up
ALTER TABLE equipo_preferencia ADD COLUMN curso_exacto boolean NOT NULL DEFAULT false;
ALTER TABLE equipo_preferencia ADD CONSTRAINT chk_preferencia_curso_exacto CHECK (NOT curso_exacto OR anio IS NOT NULL);
ALTER TABLE equipo_preferencia DROP CONSTRAINT ux_equipo_preferencia;
ALTER TABLE equipo_preferencia ADD CONSTRAINT ux_equipo_preferencia
    UNIQUE NULLS NOT DISTINCT (equipo_id, materia_norm, modalidad_norm, anio, division_norm, curso_exacto);
COMMENT ON COLUMN equipo_preferencia.curso_exacto IS
    'En un curso concreto, modalidad o división ausente significa sin ese atributo. En reglas generales significa todas. Conserva el significado de las marcas anteriores.';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM equipo_preferencia WHERE curso_exacto) THEN
        RAISE EXCEPTION 'Hay prioridades de curso concreto: revisar sus alcances antes de revertir la migración';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE equipo_preferencia DROP CONSTRAINT ux_equipo_preferencia;
ALTER TABLE equipo_preferencia DROP COLUMN curso_exacto;
ALTER TABLE equipo_preferencia ADD CONSTRAINT ux_equipo_preferencia
    UNIQUE NULLS NOT DISTINCT (equipo_id, materia_norm, modalidad_norm, anio, division_norm);
