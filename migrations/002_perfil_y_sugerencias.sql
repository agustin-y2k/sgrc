-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Perfil del usuario, pedidos de materia y buzón de sugerencias
-- ═══════════════════════════════════════════════════════════════════════
--
-- Tres cosas que llegan juntas porque las tres cuelgan de la misma pantalla:
-- el perfil que se abre desde el redondel con las iniciales.
--
--   1. La foto, para que el redondel sea una persona y no dos letras.
--   2. Los pedidos para dictar una materia más, que hasta ahora solo se
--      podían hacer una vez, al registrarse (usuario.materia_solicitada).
--   3. El buzón por donde un docente cuenta que algo no anda o sugiere un
--      cambio, sin tener que encontrar a un Admin en el pasillo.
--
-- Es la segunda migración del sistema. La primera es el esquema completo;
-- de acá en adelante cada cambio de esquema es un archivo nuevo, y goose
-- los aplica en orden por el número del nombre.

-- +goose Up

-- ── 1. La foto de perfil ───────────────────────────────────────────────
--
-- En una tabla aparte y no como columna de `usuario`: la foto se lee en una
-- sola pantalla y pesa cientos de veces más que el resto de la fila. Con la
-- columna adentro, cada listado de usuarios —el de aprobación, el de
-- administración, cada JOIN que resuelve el nombre de un docente— arrastra
-- las imágenes de todos aunque nadie las mire, y no hay forma de pedir "la
-- fila sin la foto" sin enumerar las otras quince columnas a mano.
--
-- El ON DELETE CASCADE es lo que hace que borrar una cuenta se lleve su
-- foto: guardar las imágenes en el disco en vez de acá dejaría archivos
-- huérfanos que nadie sabe de quién eran, y encima fuera de la copia de
-- seguridad, que hoy es solo de Postgres.
CREATE TABLE foto_de_perfil (
    usuario_id   UUID PRIMARY KEY REFERENCES usuario(id) ON DELETE CASCADE,
    -- La imagen ya recortada y achicada por el navegador antes de subirla
    -- (256×256). El servidor igual valida tipo y tamaño: el navegador es de
    -- quien sube, no del sistema.
    contenido    BYTEA NOT NULL,
    -- Sirve para el Content-Type al devolverla. Lista cerrada: sin SVG, que
    -- puede traer scripts adentro y se serviría desde nuestro propio origen.
    tipo         VARCHAR(20) NOT NULL CHECK (tipo IN ('image/webp', 'image/jpeg', 'image/png')),
    -- Para el ETag: con esto el navegador no vuelve a bajar la misma foto en
    -- cada pantalla que la muestre.
    actualizada_en TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── 2. Pedidos para dictar una materia ─────────────────────────────────
--
-- Al registrarse, un docente ya podía decir qué materia dicta
-- (usuario.curso_solicitado / materia_solicitada, texto libre porque la
-- materia puede no existir todavía). Esta tabla es lo mismo, pero repetible:
-- durante el año a un docente le dan una materia más, y hasta ahora la única
-- salida era pedirle a un Admin que se lo cargara a mano.
--
-- **La aprobación es una decisión humana y el sistema no la automatiza.**
-- Aceptar un pedido habilita a reservar equipos para esa materia, y quien la
-- dicta hoy puede quedarse sin máquinas porque otro llegó antes a reservarlas
-- (no puede tocarle las reservas: eso ya está prohibido en reservation). Si
-- el pedido es legítimo o no, se sabe hablando con la persona o con los
-- directivos, no mirando una pantalla. Lo que hace el sistema es dejar el
-- pedido escrito, con su motivo, avisarle a quien ya dicta esa materia para
-- que no se entere tarde, y guardar quién resolvió qué.
CREATE TABLE pedido_de_materia (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id   UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,

    -- Una de las dos formas de pedir, nunca las dos (ver el CHECK de abajo):
    --
    --   materia_id  → la materia ya existe y se eligió de la lista.
    --   *_solicitado → todavía no existe y va como texto, igual que en el
    --                  registro. Al aprobar, el Admin la crea.
    materia_id         UUID REFERENCES materia(id) ON DELETE CASCADE,
    curso_solicitado   VARCHAR(100),
    materia_solicitada VARCHAR(100),

    -- Por qué lo pide. Obligatorio: es lo único que el Admin tiene para
    -- decidir antes de ir a preguntar, y escribirlo hace pensar dos veces a
    -- quien pide de más.
    motivo       TEXT NOT NULL CHECK (length(trim(motivo)) > 0),

    estado       VARCHAR(20) NOT NULL DEFAULT 'PENDIENTE'
                 CHECK (estado IN ('PENDIENTE', 'APROBADO', 'RECHAZADO')),
    -- Lo que el Admin contesta al resolver. Le llega al docente, así que un
    -- rechazo puede explicar el porqué en vez de ser un portazo.
    respuesta    TEXT,
    resuelto_por UUID REFERENCES usuario(id) ON DELETE SET NULL,
    resuelto_en  TIMESTAMPTZ,
    creado_en    TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- O se eligió una materia de la lista, o se escribió una que no existe.
    -- Sin esto entra un pedido con las dos cosas —o con ninguna— y no hay
    -- forma de saber qué quiso decir.
    CONSTRAINT chk_pedido_una_forma CHECK (
        (materia_id IS NOT NULL AND materia_solicitada IS NULL)
        OR (materia_id IS NULL AND materia_solicitada IS NOT NULL
            AND length(trim(materia_solicitada)) > 0)
    ),
    -- Resuelto es resuelto: quién y cuándo van juntos.
    CONSTRAINT chk_pedido_resuelto CHECK (
        (estado = 'PENDIENTE' AND resuelto_en IS NULL)
        OR (estado <> 'PENDIENTE' AND resuelto_en IS NOT NULL)
    )
);

-- Un mismo docente no puede tener dos pedidos abiertos por la misma materia:
-- apretar dos veces el botón mandaba dos avisos a todos los Admin por lo
-- mismo. El índice es parcial porque la restricción vale solo mientras el
-- pedido está sin resolver — pedir de nuevo el año que viene es válido.
CREATE UNIQUE INDEX idx_pedido_materia_abierto
    ON pedido_de_materia (usuario_id, materia_id)
    WHERE estado = 'PENDIENTE' AND materia_id IS NOT NULL;

-- Lo que mira la pantalla del Admin: los pendientes, del más viejo al más
-- nuevo, porque el que más esperó es el que más urge.
CREATE INDEX idx_pedido_materia_pendientes
    ON pedido_de_materia (estado, creado_en)
    WHERE estado = 'PENDIENTE';

CREATE INDEX idx_pedido_materia_usuario ON pedido_de_materia (usuario_id, creado_en DESC);

-- ── 3. El buzón de sugerencias y fallas ────────────────────────────────
--
-- Lo que hoy pasa por el pasillo: "che, la pantalla esa no me deja", "estaría
-- bueno que se pueda...". Eso se pierde, y quien no cruza a un Admin
-- seguido no lo dice nunca.
--
-- Ojo con lo que NO es: para avisar que una computadora no anda ya está el
-- reporte de incidencias, que la marca en el inventario y la saca de
-- circulación. Esto es sobre el sistema, no sobre las máquinas.
CREATE TABLE sugerencia (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id   UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    tipo         VARCHAR(20) NOT NULL CHECK (tipo IN ('SUGERENCIA', 'PROBLEMA')),
    texto        TEXT NOT NULL CHECK (length(trim(texto)) > 0),

    -- Desde qué pantalla se escribió, y con qué versión del sistema. Lo
    -- completa la aplicación, no la persona: un "no anda" sin saber dónde
    -- estaba parado obliga a ir a buscarlo para preguntarle, y con un docente
    -- que ya se sintió torpe usando el sistema, esa conversación no vuelve a
    -- ocurrir.
    pantalla     VARCHAR(200),
    version      VARCHAR(20),

    estado       VARCHAR(20) NOT NULL DEFAULT 'ABIERTA'
                 CHECK (estado IN ('ABIERTA', 'RESUELTA')),
    -- La respuesta del Admin, que le llega como aviso a quien escribió.
    -- Cierra el círculo: sin respuesta, dos reportes ignorados alcanzan para
    -- que nadie vuelva a usar el buzón.
    respuesta    TEXT,
    respondida_por UUID REFERENCES usuario(id) ON DELETE SET NULL,
    respondida_en  TIMESTAMPTZ,
    creada_en    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sugerencia_abiertas ON sugerencia (estado, creada_en DESC);
CREATE INDEX idx_sugerencia_usuario ON sugerencia (usuario_id, creada_en DESC);

-- ── 4. Los avisos nuevos ───────────────────────────────────────────────
--
-- notificacion.tipo tiene lista cerrada, así que los tipos nuevos van acá o
-- el INSERT falla. Se rehace el CHECK entero: Postgres no sabe agregarle
-- valores a uno existente.
ALTER TABLE notificacion DROP CONSTRAINT chk_notificacion_tipo;
ALTER TABLE notificacion ADD CONSTRAINT chk_notificacion_tipo CHECK (
    tipo IN (
        'GENERAL',
        'DOCENTE_PENDIENTE',
        'RESERVA_CANCELADA',
        'LICENCIA_POR_VENCER',
        'RESERVA_POR_COMENZAR',
        'RESERVA_NO_RETIRADA',
        'EQUIPO_SIN_DEVOLVER',
        'PEDIDO_DE_LIBERACION',
        -- Un docente pidió dictar una materia: les llega a los Admin, y
        -- también a quien ya la dicta.
        'PEDIDO_DE_MATERIA',
        -- El Admin resolvió ese pedido: le llega a quien lo hizo.
        'PEDIDO_DE_MATERIA_RESUELTO',
        -- Alguien escribió en el buzón (a los Admin) / le contestaron (a
        -- quien escribió).
        'SUGERENCIA',
        'SUGERENCIA_RESPONDIDA'
    )
);

-- +goose Down

ALTER TABLE notificacion DROP CONSTRAINT chk_notificacion_tipo;
-- Las notificaciones de los tipos nuevos tienen que irse antes de volver al
-- CHECK viejo, o la restricción no se puede crear.
DELETE FROM notificacion WHERE tipo IN (
    'PEDIDO_DE_MATERIA', 'PEDIDO_DE_MATERIA_RESUELTO', 'SUGERENCIA', 'SUGERENCIA_RESPONDIDA'
);
ALTER TABLE notificacion ADD CONSTRAINT chk_notificacion_tipo CHECK (
    tipo IN (
        'GENERAL', 'DOCENTE_PENDIENTE', 'RESERVA_CANCELADA', 'LICENCIA_POR_VENCER',
        'RESERVA_POR_COMENZAR', 'RESERVA_NO_RETIRADA', 'EQUIPO_SIN_DEVOLVER',
        'PEDIDO_DE_LIBERACION'
    )
);

DROP TABLE sugerencia;
DROP TABLE pedido_de_materia;
DROP TABLE foto_de_perfil;
