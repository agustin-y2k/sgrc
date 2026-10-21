-- +goose Up
-- Conservar el más reciente entre códigos legados simultáneos; los anteriores
-- dejan de autorizar recuperación. No se borran sus constancias.
WITH orden AS (
 SELECT id,row_number() OVER(PARTITION BY usuario_id ORDER BY creado_en DESC,id DESC) AS posicion
 FROM codigo_recuperacion WHERE usado_en IS NULL
)
UPDATE codigo_recuperacion c SET usado_en=now() FROM orden o WHERE c.id=o.id AND o.posicion>1;
CREATE UNIQUE INDEX codigo_recuperacion_unico_sin_usar ON codigo_recuperacion(usuario_id) WHERE usado_en IS NULL;

-- +goose Down
DROP INDEX codigo_recuperacion_unico_sin_usar;
