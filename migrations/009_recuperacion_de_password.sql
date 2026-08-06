-- SGRC — Recuperación de contraseña por autoservicio (RF-01.10)
--
-- La persona pide un código, le llega a su email, y con ese código elige
-- una contraseña nueva sin que ningún Admin intervenga ni vea nada.
--
-- Solo aplica a las cuentas con contraseña propia: a las que entran con
-- Google las verifica Google (ver 008). El reset asistido por un Admin
-- (RF-01.6) queda como rescate para quien no puede recibir el mail.

BEGIN;

-- Tabla aparte y no un par de columnas en `usuario` por dos razones: el
-- ciclo de vida no tiene nada que ver (una fila acá vive quince minutos, la
-- de usuario vive años), y los intentos fallidos se escriben en cada prueba
-- de código — sobre `usuario` eso haría que cada intento tocara la fila que
-- usa todo el resto del sistema.
CREATE TABLE IF NOT EXISTS codigo_recuperacion (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id  UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,

    -- Hasheado con el mismo argon2 que las contraseñas. Son seis dígitos:
    -- si la base se filtrara (un backup, un dump de soporte), un código en
    -- claro sería una cuenta abierta hasta que expire.
    codigo_hash VARCHAR(255) NOT NULL,

    creado_en   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expira_en   TIMESTAMPTZ NOT NULL,

    -- Marca los dos finales posibles: el código se consumió, o se quemó al
    -- agotar los intentos. En ambos casos dejó de existir para el sistema.
    usado_en    TIMESTAMPTZ,

    -- Intentos fallidos contra ESTE código. Seis dígitos son un millón de
    -- combinaciones: sin tope, y con el rate limit por IP esquivable
    -- cambiando de red, probarlas todas es cuestión de tiempo.
    intentos    INTEGER NOT NULL DEFAULT 0
);

-- Índice parcial: la única consulta en caliente es "¿tiene esta persona un
-- código sin usar?". Las filas ya consumidas —que con el tiempo son todas—
-- no hace falta indexarlas. No se borran: quedan como registro de que esa
-- persona pidió un código.
CREATE INDEX IF NOT EXISTS idx_codigo_recuperacion_vigente
    ON codigo_recuperacion (usuario_id, creado_en DESC)
    WHERE usado_en IS NULL;

COMMIT;
