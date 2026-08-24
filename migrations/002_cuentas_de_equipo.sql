-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Las cuentas de usuario de cada equipo (RF-03.22)
-- ═══════════════════════════════════════════════════════════════════════
--
-- Una notebook no se abre sola: hay que saber con qué cuenta entrar. En una
-- escuela conviven cuentas locales, cuentas de Microsoft y cuentas de Linux,
-- de usuario común y de administrador, algunas con contraseña y otras libres.
-- Sin esto anotado en algún lado, esa información vive en la memoria de una
-- persona y en un papel.
--
-- Cargar cuentas es OPCIONAL: un equipo sin ninguna es un equipo del que no
-- anotamos nada, no un equipo mal cargado.
--
-- Es una tabla aparte y no columnas en `equipo` porque una misma máquina puede
-- tener varias: la cuenta del alumno y la de administración conviven en la
-- misma notebook.
--
-- ── Tres decisiones que conviene no deshacer ───────────────────────────
--
-- 1. `visibilidad` es de la CUENTA y no se deduce del privilegio. Puede haber
--    una cuenta de administrador que todo el mundo usa, y una cuenta común
--    que solo el equipo de administración debe poder abrir. Deducirla del
--    privilegio se equivocaría en los dos sentidos.
--
--    Lo que oculta es la CONTRASEÑA, no la cuenta: el usuario y su privilegio
--    se listan siempre. Saber que una notebook tiene una cuenta de
--    administrador es útil aunque no puedas usarla, y esconderla entera haría
--    que el inventario mienta por omisión.
--
-- 1.b `clase` es TEXTO LIBRE y no una lista cerrada, por el mismo criterio que
--    el tipo de equipo: "local" y "Microsoft" no agotan nada. Una escuela que
--    corre RedHat tiene cuentas de Linux, otra usa Google Workspace, y con un
--    enum cada una de esas realidades pide una migración y un despliegue para
--    poder anotarse. El formulario sugiere las clases ya cargadas para que no
--    convivan "Microsoft" y "MICROSOFT".
--
-- 2. `tiene_password` y `password_cifrada` son dos cosas distintas, porque hay
--    TRES estados y no dos: la cuenta libre que no pide nada, la que pide una
--    contraseña que tenemos anotada, y la que pide una que no sabemos. Sin el
--    tercero, "no tiene contraseña" y "no sabemos la contraseña" se muestran
--    igual — y esa confusión termina con alguien parado frente a una máquina
--    que no abre.
--
-- 3. La contraseña se guarda CIFRADA (AES-GCM, clave en CUENTAS_SECRET del
--    .env). No se puede hashear como la de un docente: al hash no se le
--    pregunta cuál era la contraseña, y acá justamente hay que poder leerla.
--    Cifrarla es lo que hace que el volcado de `make backup` no sea una lista
--    de contraseñas en claro. El control de quién la ve vive en la
--    aplicación (`visibilidad`); el cifrado protege lo otro: la copia del
--    archivo.
--
-- Sin CUENTAS_SECRET configurada el sistema arranca igual y las cuentas se
-- pueden cargar; lo único que no se puede es guardar contraseñas. Es el mismo
-- criterio que SMTP y que el ingreso con Google: una función de menos, no un
-- arranque roto.

-- +goose Up

CREATE TABLE equipo_cuenta (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    equipo_id           UUID NOT NULL REFERENCES equipo(id) ON DELETE CASCADE,

    -- El nombre con el que se inicia sesión en la máquina.
    usuario             VARCHAR(100) NOT NULL,
    -- Columna generada para que "Alumno" y "alumno" no entren dos veces en el
    -- mismo equipo. Se guarda `usuario` con su caja original porque es lo que
    -- hay que tipear; el match usa esta.
    usuario_normalizado VARCHAR(100) GENERATED ALWAYS AS (lower(btrim(usuario))) STORED,

    -- Texto libre: Local, Microsoft, Linux, Google… lo que esa institución
    -- tenga. Ver la nota 1.b de arriba.
    clase               VARCHAR(30) NOT NULL,
    privilegio          VARCHAR(20) NOT NULL
                        CHECK (privilegio IN ('COMUN','ADMINISTRADOR')),

    tiene_password      BOOLEAN NOT NULL,
    -- NULL cuando la cuenta no pide contraseña, y también cuando pide una que
    -- no tenemos anotada. Lo que distingue esos dos casos es tiene_password.
    password_cifrada    TEXT,

    visibilidad         VARCHAR(20) NOT NULL DEFAULT 'SOLO_ADMIN'
                        CHECK (visibilidad IN ('PUBLICA','SOLO_ADMIN')),

    notas               TEXT,

    creada_en           TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizada_en      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_equipo_cuenta_usuario_no_vacio
        CHECK (usuario <> '' AND usuario = btrim(usuario)),

    CONSTRAINT chk_equipo_cuenta_clase_no_vacia
        CHECK (clase <> '' AND clase = btrim(clase)),

    -- Una cuenta que no pide contraseña no puede traer una guardada: sería un
    -- dato que contradice al otro, y la pantalla tendría que elegir a cuál
    -- creerle.
    CONSTRAINT chk_equipo_cuenta_password_coherente
        CHECK (tiene_password OR password_cifrada IS NULL),

    -- Dos cuentas con el mismo nombre en la misma máquina no existen.
    CONSTRAINT ux_equipo_cuenta_usuario UNIQUE (equipo_id, usuario_normalizado)
);

-- No hace falta un índice por equipo_id: el UNIQUE de arriba ya crea uno
-- compuesto que empieza por esa columna, y listar las cuentas de un equipo es
-- exactamente esa búsqueda.

-- +goose Down

DROP TABLE IF EXISTS equipo_cuenta;
