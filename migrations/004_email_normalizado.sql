-- SGRC — El email identifica una cuenta sin importar mayúsculas
--
-- `usuario.email` era UNIQUE a secas, y Postgres compara VARCHAR byte a
-- byte. "Juan.Perez@escuela.edu.ar" y "juan.perez@escuela.edu.ar" eran, para
-- la base, dos cuentas distintas del mismo buzón:
--
--   * quien se registraba con una y después tipeaba la otra al entrar
--     recibía "credenciales inválidas", sin ninguna pista de por qué;
--   * el mismo docente podía quedar registrado dos veces, y un Admin
--     aprobaba una de las dos sin forma de saber que existía la otra.
--
-- La app ya normaliza a minúsculas al crear y al buscar (ver
-- domain.NormalizarEmail). Esta migración pone la misma regla en la base,
-- que es lo que la hace cumplirse aunque alguien inserte una fila a mano.

BEGIN;

-- ══════════════════════════════════════════════════════════════════
-- 1. Cortar temprano si ya hay duplicados
-- ══════════════════════════════════════════════════════════════════
-- Sobre una base nueva esto no encuentra nada. Sobre una base con datos,
-- decidir cuál de las dos cuentas sobrevive NO es algo que una migración
-- pueda resolver sola: cada una puede tener reservas, materias asignadas e
-- historial propio. Así que se corta acá, con los emails en el mensaje, en
-- vez de fallar más abajo con un error de índice que no dice qué pasó.
DO $$
DECLARE
    duplicados text;
BEGIN
    SELECT string_agg(e, ', ')
      INTO duplicados
      FROM (
          SELECT lower(email) AS e
          FROM usuario
          GROUP BY lower(email)
          HAVING count(*) > 1
      ) AS d;

    IF duplicados IS NOT NULL THEN
        RAISE EXCEPTION
            'Hay cuentas que solo se diferencian por mayúsculas: %. Resolvé cuál queda antes de aplicar esta migración: revisá cada par con  SELECT id, email, rol, estado, fecha_registro FROM usuario WHERE lower(email) IN (...)  y dá de baja + eliminá la que sobra (RF-01.9) o cambiale el email.',
            duplicados;
    END IF;
END $$;

-- ══════════════════════════════════════════════════════════════════
-- 2. Llevar lo existente a la forma canónica
-- ══════════════════════════════════════════════════════════════════
UPDATE usuario SET email = lower(btrim(email)) WHERE email <> lower(btrim(email));

-- ══════════════════════════════════════════════════════════════════
-- 3. Que la regla la sostenga la base
-- ══════════════════════════════════════════════════════════════════
-- Índice funcional sobre lower(email): es lo que impide que vuelvan a
-- entrar dos capitalizaciones del mismo buzón, y además es el índice que usa
-- el WHERE lower(email) = lower($1) de BuscarPorEmail.
--
-- El UNIQUE original sobre la columna se conserva: ahora que todo está en
-- minúsculas es redundante con este índice, pero sacarlo no gana nada y
-- dejarlo mantiene la restricción visible en el esquema.
CREATE UNIQUE INDEX IF NOT EXISTS idx_usuario_email_lower ON usuario (lower(email));

COMMIT;
