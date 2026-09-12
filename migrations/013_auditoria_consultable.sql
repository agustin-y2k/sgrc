-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Que el registro de auditoría se pueda leer
-- ═══════════════════════════════════════════════════════════════════════
--
-- `audit_log` se escribe en cada acción sensible desde que existe el sistema, y
-- hasta acá NO había forma de consultarlo: ni un endpoint, ni una pantalla. La
-- única manera de mirarlo era entrar a la base con psql.
--
-- Eso convierte el requisito en una promesa a medias. El registro no está para
-- guardar: está para contestar «¿quién borró esto?» el día que alguien lo
-- pregunta, y una respuesta que exige acceso de administrador de base de datos
-- no está disponible para el Admin que tiene la pregunta.
--
-- ── Los índices ────────────────────────────────────────────────────────
--
-- La tabla tenía uno solo, `(usuario_id, creado_en DESC)`, que sirve para «¿qué
-- hizo esta persona?». Las otras dos preguntas —que son las que uno hace de
-- verdad— no tenían ninguno:
--
--   - «¿qué pasó últimamente?» → `(creado_en DESC)`. Es además el orden en el
--     que se lee siempre, así que también evita ordenar el resultado.
--   - «¿quién tocó ESTA cosa?» → `(entidad, entidad_id, creado_en DESC)`. Es la
--     pregunta que se hace cuando un equipo aparece con algo cambiado o un
--     curso desapareció.
--
-- El de `accion` va aparte y no combinado con `entidad`: se filtra por una o
-- por la otra, y quien filtra por las dos ya queda acotado por la primera.

-- +goose Up

CREATE INDEX idx_audit_creado_en ON audit_log (creado_en DESC);

CREATE INDEX idx_audit_entidad ON audit_log (entidad, entidad_id, creado_en DESC);

CREATE INDEX idx_audit_accion ON audit_log (accion, creado_en DESC);

COMMENT ON TABLE audit_log IS
    'Quién hizo qué acción sensible, sobre qué entidad y cuándo (RF-09.x, '
    'docs/09-seguridad-rbac.md §5). Se lee desde /api/auditoria, sólo Admin. '
    'Nunca se actualiza ni se borra: reescribir un registro de auditoría es '
    'precisamente lo que un registro de auditoría no debe permitir.';

-- +goose Down

DROP INDEX idx_audit_accion;
DROP INDEX idx_audit_entidad;
DROP INDEX idx_audit_creado_en;
