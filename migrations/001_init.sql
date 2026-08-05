-- SGRC — Migración inicial
-- Orden respetando dependencias de FK.

CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS btree_gist; -- EXCLUDE constraint de reserva

-- ══════════════════════════════
-- USUARIO
-- ══════════════════════════════
CREATE TABLE usuario (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nombre                 VARCHAR(100) NOT NULL,
    apellido               VARCHAR(100) NOT NULL,
    email                  VARCHAR(150) UNIQUE NOT NULL,
    password_hash          VARCHAR(255) NOT NULL,
    debe_cambiar_password  BOOLEAN NOT NULL DEFAULT false,
    rol                    VARCHAR(10) NOT NULL CHECK (rol IN ('ADMIN','DOCENTE')),
    estado                 VARCHAR(20) NOT NULL DEFAULT 'PENDIENTE'
                           CHECK (estado IN ('PENDIENTE','APROBADA','RECHAZADA','BAJA')),
    fecha_registro         TIMESTAMP NOT NULL DEFAULT now(),
    fecha_aprobacion       TIMESTAMP,
    aprobado_por           UUID REFERENCES usuario(id) ON DELETE SET NULL
);
CREATE INDEX idx_usuario_estado ON usuario(estado);

-- ══════════════════════════════
-- CICLO LECTIVO / CURSO / MATERIA / DOCENTE_MATERIA
-- ══════════════════════════════
CREATE TABLE ciclo_lectivo (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    anio       INTEGER NOT NULL UNIQUE,
    activo     BOOLEAN NOT NULL DEFAULT true,
    archivado  BOOLEAN NOT NULL DEFAULT false
);
-- Solo puede haber un ciclo activo a la vez (RF-02.1)
CREATE UNIQUE INDEX idx_ciclo_lectivo_activo_unico ON ciclo_lectivo(activo) WHERE activo = true;

CREATE TABLE curso (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ciclo_lectivo_id  UUID NOT NULL REFERENCES ciclo_lectivo(id),
    nombre            VARCHAR(4) NOT NULL CHECK (nombre ~ '^[1-6]°[A-Z]$'),
    activo            BOOLEAN NOT NULL DEFAULT true,
    archivado         BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (ciclo_lectivo_id, nombre)
);
CREATE INDEX idx_curso_ciclo ON curso(ciclo_lectivo_id);

CREATE TABLE materia (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    curso_id   UUID NOT NULL REFERENCES curso(id) ON DELETE CASCADE,
    nombre     VARCHAR(100) NOT NULL,
    activo     BOOLEAN NOT NULL DEFAULT true,
    archivado  BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (curso_id, nombre)
);
CREATE INDEX idx_materia_curso ON materia(curso_id);

CREATE TABLE docente_materia (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id  UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    materia_id  UUID NOT NULL REFERENCES materia(id) ON DELETE CASCADE,
    rol         VARCHAR(10) NOT NULL CHECK (rol IN ('TITULAR','SUPLENTE')),
    UNIQUE (usuario_id, materia_id)
);
CREATE INDEX idx_docente_materia_materia ON docente_materia(materia_id);
CREATE INDEX idx_docente_materia_usuario ON docente_materia(usuario_id);

-- ══════════════════════════════
-- CARRO / PC / INCIDENCIA
-- ══════════════════════════════
CREATE TABLE carro (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nombre       VARCHAR(100) NOT NULL UNIQUE,
    descripcion  TEXT
);

-- identificador: entero, único DENTRO del carro (no globalmente) — "PC 27"
-- puede repetirse en el Carro 1 y en el Carro 2.
-- numero_serie: entero largo, de fábrica, único en TODA la tabla.
CREATE TABLE pc (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    carro_id            UUID NOT NULL REFERENCES carro(id),
    identificador       INTEGER NOT NULL,
    numero_serie        BIGINT UNIQUE NOT NULL,
    freezado            BOOLEAN NOT NULL DEFAULT false,
    cpu                 VARCHAR(100),
    ram                 VARCHAR(20),
    sistema_operativo   VARCHAR(50),
    software_instalado  TEXT,
    estado              VARCHAR(25) NOT NULL DEFAULT 'DISPONIBLE'
                        CHECK (estado IN ('DISPONIBLE','EN_MANTENIMIENTO','FUERA_DE_SERVICIO')),
    dada_de_baja        BOOLEAN NOT NULL DEFAULT false,
    fecha_baja          TIMESTAMP,
    fecha_alta          TIMESTAMP NOT NULL DEFAULT now(),
    UNIQUE (carro_id, identificador)
);
CREATE INDEX idx_pc_carro_estado ON pc(carro_id, estado);

CREATE TABLE incidencia (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pc_id            UUID NOT NULL REFERENCES pc(id),
    reportado_por    UUID REFERENCES usuario(id) ON DELETE SET NULL,
    descripcion      TEXT NOT NULL,
    gravedad         VARCHAR(10) NOT NULL CHECK (gravedad IN ('LEVE','MODERADA','GRAVE')),
    fecha            TIMESTAMP NOT NULL DEFAULT now(),
    enviado_dge      BOOLEAN NOT NULL DEFAULT false,
    fecha_envio_dge  TIMESTAMP,
    estado           VARCHAR(20) NOT NULL DEFAULT 'ABIERTA'
                     CHECK (estado IN ('ABIERTA','EN_REPARACION','ENVIADA_DGE','RESUELTA'))
);
CREATE INDEX idx_incidencia_pc ON incidencia(pc_id);

-- ══════════════════════════════
-- RECURRENCIA / RESERVAS
-- ══════════════════════════════
CREATE TABLE regla_recurrencia (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    materia_id    UUID NOT NULL REFERENCES materia(id),
    creado_por    UUID NOT NULL REFERENCES usuario(id),
    dia_semana    VARCHAR(10) NOT NULL,
    hora_inicio   TIME NOT NULL,
    hora_fin      TIME NOT NULL CHECK (hora_fin > hora_inicio),
    fecha_inicio  DATE NOT NULL,
    fecha_fin     DATE NOT NULL CHECK (fecha_fin >= fecha_inicio)
);

CREATE TABLE regla_recurrencia_pc (
    regla_recurrencia_id  UUID NOT NULL REFERENCES regla_recurrencia(id),
    pc_id                 UUID NOT NULL REFERENCES pc(id),
    PRIMARY KEY (regla_recurrencia_id, pc_id)
);

CREATE TABLE reserva_grupo (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    materia_id             UUID NOT NULL REFERENCES materia(id),
    creado_por             UUID REFERENCES usuario(id) ON DELETE SET NULL,
    nombre_docente_snapshot VARCHAR(200) NOT NULL,
    fecha                  DATE NOT NULL,
    hora_inicio            TIME NOT NULL,
    hora_fin               TIME NOT NULL CHECK (hora_fin > hora_inicio),
    estado                 VARCHAR(25) NOT NULL DEFAULT 'CONFIRMADA'
                           CHECK (estado IN ('CONFIRMADA','PARCIALMENTE_CANCELADA','CANCELADA','FINALIZADA')),
    regla_recurrencia_id   UUID REFERENCES regla_recurrencia(id),
    creada_en              TIMESTAMP NOT NULL DEFAULT now()
);
CREATE INDEX idx_reserva_grupo_materia ON reserva_grupo(materia_id);
CREATE INDEX idx_reserva_grupo_creado_por ON reserva_grupo(creado_por);
CREATE INDEX idx_reserva_grupo_regla ON reserva_grupo(regla_recurrencia_id) WHERE regla_recurrencia_id IS NOT NULL;

CREATE TABLE reserva (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reserva_grupo_id        UUID REFERENCES reserva_grupo(id) ON DELETE CASCADE,
    pc_id                   UUID NOT NULL REFERENCES pc(id),
    materia_id              UUID REFERENCES materia(id),
    nombre_docente_snapshot VARCHAR(200),
    fecha                   DATE NOT NULL,
    hora_inicio             TIME NOT NULL,
    hora_fin                TIME NOT NULL CHECK (hora_fin > hora_inicio),
    estado                  VARCHAR(15) NOT NULL DEFAULT 'CONFIRMADA'
                            CHECK (estado IN ('CONFIRMADA','CANCELADA','FINALIZADA')),
    tipo                    VARCHAR(20) NOT NULL DEFAULT 'NORMAL'
                            CHECK (tipo IN ('NORMAL','EVALUACION_ESTATAL')),
    creado_por              UUID REFERENCES usuario(id) ON DELETE SET NULL,
    creada_en               TIMESTAMP NOT NULL DEFAULT now(),
    cancelado_por           UUID REFERENCES usuario(id) ON DELETE SET NULL,
    motivo_cancelacion      TEXT,
    cancelada_en            TIMESTAMP,
    CONSTRAINT chk_reserva_tipo_coherente CHECK (
        (tipo = 'NORMAL' AND reserva_grupo_id IS NOT NULL AND materia_id IS NOT NULL)
        OR
        (tipo = 'EVALUACION_ESTATAL' AND reserva_grupo_id IS NULL AND materia_id IS NULL)
    )
);

-- Anti-solapamiento por PC (RF-04.3)
--
-- IMPORTANTE: se usa `fecha + hora_inicio` (aritmética DATE + TIME =
-- timestamp, una operación pura) en vez de concatenar texto y castear
-- ("fecha::text || ' ' || hora_inicio::text)::timestamp"). Postgres exige
-- que las expresiones de un índice (y un EXCLUDE es un índice GiST por
-- debajo) sean IMMUTABLE — el cast de texto a timestamp depende de la
-- configuración regional del servidor (DateStyle) y Postgres lo trata
-- como STABLE, no IMMUTABLE, así que la versión con texto rechaza la
-- constraint con "functions in index expression must be marked IMMUTABLE".
-- La suma date+time no tiene ese problema: es aritmética directa.
ALTER TABLE reserva ADD CONSTRAINT no_solapamiento
    EXCLUDE USING gist (
        pc_id WITH =,
        tsrange(fecha + hora_inicio, fecha + hora_fin) WITH &&
    )
    WHERE (estado = 'CONFIRMADA');

CREATE INDEX idx_reserva_pc_fecha ON reserva(pc_id, fecha);
CREATE INDEX idx_reserva_grupo ON reserva(reserva_grupo_id) WHERE reserva_grupo_id IS NOT NULL;
CREATE INDEX idx_reserva_materia ON reserva(materia_id) WHERE materia_id IS NOT NULL;
CREATE INDEX idx_reserva_creado_por ON reserva(creado_por);

-- ══════════════════════════════
-- NOTIFICACIONES
-- ══════════════════════════════
CREATE TABLE notificacion (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id  UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    reserva_id  UUID REFERENCES reserva(id) ON DELETE SET NULL,
    mensaje     TEXT NOT NULL,
    estado      VARCHAR(10) NOT NULL DEFAULT 'NO_LEIDA' CHECK (estado IN ('NO_LEIDA','LEIDA')),
    creada_en   TIMESTAMP NOT NULL DEFAULT now(),
    leida_en    TIMESTAMP
);
CREATE INDEX idx_notif_usuario_estado ON notificacion(usuario_id, estado);

-- ══════════════════════════════
-- DISPONIBILIDAD DE ADMINS (RF-07, informativo)
-- ══════════════════════════════
CREATE TABLE horario_admin (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id   UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    dia_semana   VARCHAR(10) NOT NULL,
    hora_inicio  TIME NOT NULL,
    hora_fin     TIME NOT NULL CHECK (hora_fin > hora_inicio)
);
CREATE INDEX idx_horario_admin_usuario ON horario_admin(usuario_id);

CREATE TABLE horario_admin_excepcion (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id   UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    fecha        DATE NOT NULL,
    tipo         VARCHAR(20) NOT NULL CHECK (tipo IN ('NO_DISPONIBLE','HORARIO_MODIFICADO')),
    hora_inicio  TIME,
    hora_fin     TIME,
    motivo       TEXT,
    UNIQUE (usuario_id, fecha),
    CONSTRAINT chk_excepcion_horario_coherente CHECK (
        (tipo = 'NO_DISPONIBLE' AND hora_inicio IS NULL AND hora_fin IS NULL)
        OR
        (tipo = 'HORARIO_MODIFICADO' AND hora_inicio IS NOT NULL AND hora_fin IS NOT NULL AND hora_fin > hora_inicio)
    )
);
CREATE INDEX idx_horario_excepcion_usuario_fecha ON horario_admin_excepcion(usuario_id, fecha);

-- ══════════════════════════════
-- HISTÓRICO (snapshot permanente, calculado al archivar un ciclo)
-- ══════════════════════════════
CREATE TABLE historico_uso_pc (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    anio                    INTEGER NOT NULL,
    pc_id                   UUID NOT NULL REFERENCES pc(id),
    identificador_snapshot  INTEGER NOT NULL,
    carro_nombre_snapshot   VARCHAR(100) NOT NULL,
    minutos_reservados      INTEGER NOT NULL,
    cantidad_reservas       INTEGER NOT NULL,
    UNIQUE (anio, pc_id)
);
CREATE INDEX idx_historico_pc_anio ON historico_uso_pc(anio);

CREATE TABLE historico_uso_docente (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    anio                     INTEGER NOT NULL,
    usuario_id               UUID REFERENCES usuario(id),
    nombre_docente_snapshot  VARCHAR(200) NOT NULL,
    cantidad_reservas        INTEGER NOT NULL,
    minutos_totales          INTEGER NOT NULL,
    UNIQUE (anio, usuario_id)
);
CREATE INDEX idx_historico_docente_anio ON historico_uso_docente(anio);

-- ══════════════════════════════
-- AUDITORÍA
-- ══════════════════════════════
CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id  UUID NOT NULL,
    accion      VARCHAR(100) NOT NULL,
    entidad     VARCHAR(50) NOT NULL,
    entidad_id  UUID,
    detalle     JSONB,
    ip_origen   INET,
    creado_en   TIMESTAMP NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_usuario ON audit_log(usuario_id, creado_en DESC);
