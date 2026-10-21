-- +goose Up
CREATE TABLE baja_usuario (
    usuario_id UUID PRIMARY KEY REFERENCES usuario(id) ON DELETE CASCADE,
    preparada_en TIMESTAMPTZ NOT NULL,
    completada_en TIMESTAMPTZ
);

CREATE TABLE baja_usuario_materia (
    usuario_id UUID NOT NULL REFERENCES baja_usuario(usuario_id) ON DELETE CASCADE,
    materia_id UUID NOT NULL REFERENCES materia(id) ON DELETE CASCADE,
    completada_en TIMESTAMPTZ,
    PRIMARY KEY (usuario_id, materia_id)
);
CREATE INDEX idx_baja_usuario_materia_materia ON baja_usuario_materia(materia_id);

-- Las bajas legadas que conservan asignaciones aparecen como pendientes.
-- preparada_en representa el momento en que se registró la recuperación.
INSERT INTO baja_usuario(usuario_id, preparada_en)
SELECT u.id, CURRENT_TIMESTAMP FROM usuario u
WHERE u.estado = 'BAJA'
  AND EXISTS (SELECT 1 FROM docente_materia dm WHERE dm.usuario_id = u.id);
INSERT INTO baja_usuario_materia(usuario_id, materia_id)
SELECT DISTINCT dm.usuario_id, dm.materia_id FROM docente_materia dm
JOIN baja_usuario b ON b.usuario_id = dm.usuario_id;
-- No se infiere la finalización ni se reconstruyen asignaciones ya perdidas.

-- +goose Down
-- No descartar el registro de una limpieza todavía pendiente.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM baja_usuario WHERE completada_en IS NULL) THEN
        RAISE EXCEPTION 'hay bajas pendientes; completarlas antes de bajar 020';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE baja_usuario_materia;
DROP TABLE baja_usuario;
