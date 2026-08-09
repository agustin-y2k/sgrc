-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Esquema inicial
-- ═══════════════════════════════════════════════════════════════════════
--
-- Sistema de reserva de equipos informáticos para una institución
-- educativa. Este archivo es el esquema completo: se aplica solo, sobre una
-- base vacía, y deja el sistema listo para arrancar.
--
-- Postgres lo ejecuta solo la primera vez que se crea el volumen, vía
-- docker-entrypoint-initdb.d (ver docker-compose.yml). Para aplicarlo a mano
-- sobre una base ya creada: `make migrate ARCHIVO=migrations/001_esquema_inicial.sql`.
--
-- ── Cómo leerlo ────────────────────────────────────────────────────────
--
-- Las tablas van en orden de dependencia, así que se puede leer de corrido.
-- Los comentarios explican POR QUÉ una decisión es como es, no qué hace el
-- SQL — eso ya lo dice el SQL. Donde una regla parece rara, el comentario
-- cuenta qué pasaba sin ella.
--
-- El modelo completo, con diagrama entidad-relación, está en
-- `docs/07-modelo-datos.md`. Los requisitos que cada regla implementa
-- (RF-XX) están en `docs/01-requisitos.md`.
--
-- ── Dos principios que se repiten ──────────────────────────────────────
--
-- **Las reglas que protegen datos viven en la base, no solo en el código.**
-- El anti-solapamiento de reservas es una constraint EXCLUDE, no una
-- validación en Go: una validación se puede ganar por carrera, una
-- constraint no. Lo mismo con los índices únicos parciales y los CHECK de
-- coherencia. El código valida antes para dar un mensaje entendible; la
-- base valida siempre para que el dato no pueda existir.
--
-- **Lo que la institución escribe es texto libre; lo que el sistema
-- interpreta es un enum.** El tipo de un equipo, la categoría de una falla
-- o el motivo de un bloqueo son texto libre, porque cada institución rompe,
-- presta y se organiza distinto, y una lista cerrada obligaría a una
-- migración para anotar el primer caso no previsto. En cambio los estados
-- —de una reserva, de un equipo, de una cuenta— son enums con CHECK: sobre
-- ellos el sistema decide, así que un valor inesperado sería un error.

-- ═══════════════════════════════════════════════════════════════════════
-- Extensiones
-- ═══════════════════════════════════════════════════════════════════════

-- gen_random_uuid() para las claves primarias.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- btree_gist habilita la constraint EXCLUDE de `reserva`, que necesita
-- combinar igualdad (mismo equipo) con solapamiento de rangos (misma franja)
-- en un solo índice.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- ═══════════════════════════════════════════════════════════════════════
-- Usuarios y acceso (RF-01, RF-02)
-- ═══════════════════════════════════════════════════════════════════════

-- Un docente se registra solo y queda PENDIENTE hasta que un ADMIN lo
-- aprueba: tener un correo no prueba que la institución conozca a esa
-- persona.
CREATE TABLE usuario (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nombre                 VARCHAR(100) NOT NULL,
    apellido               VARCHAR(100) NOT NULL,
    email                  VARCHAR(150) NOT NULL UNIQUE,

    -- password_hash es NULL en quien entra únicamente con Google, y
    -- google_sub es NULL en quien entra con contraseña. El CHECK de abajo
    -- exige al menos una de las dos: una cuenta sin ninguna credencial no
    -- puede iniciar sesión de ninguna forma, así que no debería existir.
    password_hash          VARCHAR(255),
    google_sub             VARCHAR(255),
    debe_cambiar_password  BOOLEAN NOT NULL DEFAULT false,

    rol                    VARCHAR(10) NOT NULL CHECK (rol IN ('ADMIN','DOCENTE')),
    estado                 VARCHAR(20) NOT NULL DEFAULT 'PENDIENTE'
                           CHECK (estado IN ('PENDIENTE','APROBADA','RECHAZADA','BAJA')),

    fecha_registro         TIMESTAMPTZ NOT NULL DEFAULT now(),
    fecha_aprobacion       TIMESTAMPTZ,
    aprobado_por           UUID REFERENCES usuario(id) ON DELETE SET NULL,

    -- Qué dijo que iba a dictar al registrarse. Texto libre y no una
    -- referencia: al registrarse la persona no está autenticada y no puede
    -- elegir de una lista. Es lo que el ADMIN mira para saber a qué materia
    -- asignarla, o para darse cuenta de que esa materia todavía no existe.
    curso_solicitado       VARCHAR(100),
    materia_solicitada     VARCHAR(100),

    -- Se incrementa para invalidar las sesiones abiertas de esta persona.
    -- Los tokens son stateless: sin este contador, cambiar una contraseña o
    -- dar de baja una cuenta no cerraba las sesiones ya emitidas.
    version_sesion         INTEGER NOT NULL DEFAULT 0,

    CONSTRAINT chk_usuario_credencial CHECK (password_hash IS NOT NULL OR google_sub IS NOT NULL)
);

-- El UNIQUE de la columna distingue mayúsculas, así que "Ana@x.com" y
-- "ana@x.com" entrarían como dos cuentas. Este índice lo impide de verdad.
CREATE UNIQUE INDEX idx_usuario_email_lower ON usuario (lower(email));

-- Parcial: una cuenta sin Google tiene google_sub en NULL, y varios NULL no
-- chocan entre sí en un índice único, pero el parcial deja explícito que la
-- unicidad aplica solo a quienes sí lo tienen.
CREATE UNIQUE INDEX idx_usuario_google_sub ON usuario (google_sub) WHERE google_sub IS NOT NULL;

-- La pantalla de aprobación filtra por estado.
CREATE INDEX idx_usuario_estado ON usuario (estado);

-- Recuperación de contraseña por código de un solo uso (RF-01.10).
CREATE TABLE codigo_recuperacion (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id   UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    -- Se guarda hasheado por el mismo motivo que la contraseña: quien pueda
    -- leer la tabla no tiene que poder usar el código.
    codigo_hash  VARCHAR(255) NOT NULL,
    creado_en    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expira_en    TIMESTAMPTZ NOT NULL,
    usado_en     TIMESTAMPTZ,
    -- Cuenta los intentos fallidos contra ESTE código. Un código de pocos
    -- dígitos sin tope se adivina a fuerza de probar.
    intentos     INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_codigo_recuperacion_vigente
    ON codigo_recuperacion (usuario_id, creado_en DESC) WHERE usado_en IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- Ciclo lectivo, cursos y materias (RF-02)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE ciclo_lectivo (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    anio       INTEGER NOT NULL UNIQUE,
    activo     BOOLEAN NOT NULL DEFAULT true,
    archivado  BOOLEAN NOT NULL DEFAULT false
);

-- Un solo ciclo activo a la vez, garantizado por la base. El índice es
-- parcial sobre `activo = true`: los archivados no compiten entre sí.
CREATE UNIQUE INDEX idx_ciclo_lectivo_activo_unico ON ciclo_lectivo (activo) WHERE activo = true;

CREATE TABLE curso (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ciclo_lectivo_id  UUID NOT NULL REFERENCES ciclo_lectivo(id),
    -- Formato "año + división": 1°A hasta 6°Z. El CHECK evita que la misma
    -- división entre como "3A", "3° A" y "3°a" y termine partida en tres.
    -- Una institución con otra nomenclatura cambia esta expresión y nada
    -- más: el resto del sistema trata el nombre como una etiqueta.
    nombre            VARCHAR(4) NOT NULL CHECK (nombre ~ '^[1-6]°[A-Z]$'),
    activo            BOOLEAN NOT NULL DEFAULT true,
    archivado         BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (ciclo_lectivo_id, nombre)
);

CREATE INDEX idx_curso_ciclo ON curso (ciclo_lectivo_id);

CREATE TABLE materia (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    curso_id   UUID NOT NULL REFERENCES curso(id) ON DELETE CASCADE,
    nombre     VARCHAR(100) NOT NULL,
    activo     BOOLEAN NOT NULL DEFAULT true,
    archivado  BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (curso_id, nombre)
);

CREATE INDEX idx_materia_curso ON materia (curso_id);

-- Quién dicta qué. Un docente puede tener varias materias y una materia
-- varios docentes (titular y suplente).
CREATE TABLE docente_materia (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id  UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    materia_id  UUID NOT NULL REFERENCES materia(id) ON DELETE CASCADE,
    rol         VARCHAR(10) NOT NULL CHECK (rol IN ('TITULAR','SUPLENTE')),
    UNIQUE (usuario_id, materia_id)
);

CREATE INDEX idx_docente_materia_usuario ON docente_materia (usuario_id);
CREATE INDEX idx_docente_materia_materia ON docente_materia (materia_id);

-- ═══════════════════════════════════════════════════════════════════════
-- Inventario (RF-03)
-- ═══════════════════════════════════════════════════════════════════════

-- Un carro es un mueble con ruedas y zócalos numerados donde las notebooks
-- se guardan y se cargan. Cuántos zócalos tiene depende del carro: el modelo
-- no presupone ninguna cantidad.
CREATE TABLE carro (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nombre       VARCHAR(100) NOT NULL UNIQUE,
    descripcion  TEXT
);

-- Todo lo que la institución presta, en UNA sola tabla: las computadoras de
-- un carro y también proyectores, cargadores o notebooks sueltas.
--
-- Compartir entidad no es un atajo de implementación, es la decisión que
-- hace que todo lo demás funcione. "Qué hay fuera del laboratorio" tiene que
-- ser una sola lista: con dos clases de cosa, el préstamo necesitaría dos
-- referencias, el mostrador dos consultas y el barrido de vencimientos dos
-- recorridos. Compartiendo tabla, un proyector queda prestable, reclamable y
-- —si se marca reservable— reservable, sin una línea nueva en ninguno de
-- esos flujos.
CREATE TABLE equipo (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Lo que está en un carro tiene carro_id + identificador (el número de
    -- su zócalo). Lo que no está en ninguno tiene nombre. El CHECK de abajo
    -- exige exactamente una de las dos formas: un equipo que no se puede
    -- nombrar de ninguna manera no sirve para nada.
    carro_id           UUID REFERENCES carro(id),
    identificador      INTEGER,
    nombre             VARCHAR(100),

    -- Texto libre, no un enum: la lista de cosas que presta una institución
    -- no es la de otra, y agregar "impresora 3D" no puede pedir tocar el
    -- sistema. El formulario sugiere los tipos ya cargados para que no
    -- convivan "PROYECTOR" y "Proyector".
    tipo               VARCHAR(50) NOT NULL DEFAULT 'PC',

    numero_serie       VARCHAR(50) UNIQUE,
    freezado           BOOLEAN NOT NULL DEFAULT false,
    cpu                VARCHAR(100),
    ram                VARCHAR(20),
    sistema_operativo  VARCHAR(50),
    software_instalado TEXT,

    estado             VARCHAR(25) NOT NULL DEFAULT 'DISPONIBLE'
                       CHECK (estado IN ('DISPONIBLE','EN_MANTENIMIENTO','FUERA_DE_SERVICIO')),

    -- Marca si aparece en la lista de equipos libres al reservar. Un
    -- proyector sí; un cargador se presta en el momento y sería ruido cada
    -- vez que un docente arma una reserva.
    reservable         BOOLEAN NOT NULL DEFAULT true,

    -- Baja lógica: el equipo deja de listarse y de poder reservarse, pero su
    -- historial de incidencias, préstamos y reservas pasadas se conserva.
    dado_de_baja       BOOLEAN NOT NULL DEFAULT false,
    fecha_baja         TIMESTAMPTZ,
    fecha_alta         TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_equipo_identificable CHECK (
        (carro_id IS NOT NULL AND identificador IS NOT NULL)
        OR
        (carro_id IS NULL AND nombre IS NOT NULL)
    ),

    -- El número de serie es el de fábrica y casi siempre lleva letras. Se
    -- guarda en mayúsculas y sin espacios al borde para que la misma máquina
    -- no pueda entrar dos veces con distinta caja.
    CONSTRAINT equipo_numero_serie_no_vacio CHECK (numero_serie <> '' AND numero_serie = upper(btrim(numero_serie))),
    CONSTRAINT equipo_tipo_check           CHECK (tipo <> '' AND tipo = btrim(tipo)),
    CONSTRAINT equipo_nombre_check         CHECK (nombre IS NULL OR (nombre <> '' AND nombre = btrim(nombre))),

    -- El identificador es el número del zócalo, así que se repite entre
    -- carros y es único dentro de uno: "PC 7" existe en cada carro.
    UNIQUE (carro_id, identificador)
);

COMMENT ON TABLE equipo IS
    'Todo lo que la institución presta: las computadoras de un carro y también '
    'proyectores, cargadores o notebooks sueltas. Lo que está en un carro se '
    'nombra por su identificador ("PC 3"); lo que no, por su nombre.';

COMMENT ON COLUMN equipo.nombre IS
    'Cómo se lo llama cuando no tiene número de carro. NULL en las '
    'computadoras de un carro, que se nombran por su identificador.';

COMMENT ON COLUMN equipo.numero_serie IS
    'Número de serie de fábrica, alfanumérico. Único en toda la institución. '
    'Se guarda en mayúsculas y sin espacios al borde.';

COMMENT ON COLUMN equipo.reservable IS
    'Si aparece en la lista de equipos libres al reservar. Un proyector sí, '
    'un cargador no: se presta en el momento y nadie planifica con él.';

CREATE INDEX idx_equipo_carro_estado ON equipo (carro_id, estado);

-- El listado de lo que no está en ningún carro, en su propio orden.
CREATE INDEX idx_equipo_sueltos ON equipo (tipo, nombre) WHERE carro_id IS NULL;

-- Entre los equipos sueltos el nombre es lo único que los distingue, así que
-- dos "Cargador" serían indistinguibles justo donde hay que elegir cuál se
-- está prestando. Excluye los dados de baja: el nombre de algo que salió del
-- inventario puede reutilizarse.
CREATE UNIQUE INDEX ux_equipo_suelto_nombre
    ON equipo (lower(nombre)) WHERE carro_id IS NULL AND dado_de_baja = false;

-- ── Incidencias (RF-03.5) ──────────────────────────────────────────────

CREATE TABLE incidencia (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    equipo_id      UUID NOT NULL REFERENCES equipo(id),

    -- ON DELETE SET NULL: el historial de la falla vale por sí mismo aunque
    -- se pierda quién la reportó, si esa cuenta se elimina más adelante.
    reportado_por  UUID REFERENCES usuario(id) ON DELETE SET NULL,

    descripcion    TEXT NOT NULL,

    -- Qué tipo de falla es, en texto libre: batería, pantalla, bisagra.
    -- Es lo que convierte el historial en estadística — la descripción dice
    -- qué le pasa a ESTA máquina, la categoría dice cuántas tienen lo mismo,
    -- que es el dato con el que se pide un presupuesto o un lote de
    -- repuestos.
    --
    -- NULL es un estado legítimo y no un dato faltante: una máquina que no
    -- enciende y que nadie pudo diagnosticar tiene una falla real y ninguna
    -- categoría. Obligar a completarla llevaría a que alguien escriba "otro",
    -- que parece un diagnóstico y no lo es.
    categoria      VARCHAR(50),

    gravedad       VARCHAR(10) NOT NULL CHECK (gravedad IN ('LEVE','MODERADA','GRAVE')),
    fecha          TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Envío a un soporte técnico externo: el organismo, proveedor o taller al
    -- que cada institución manda los equipos que no puede reparar.
    enviado_a_soporte      BOOLEAN NOT NULL DEFAULT false,
    fecha_envio_a_soporte  TIMESTAMPTZ,

    estado         VARCHAR(20) NOT NULL DEFAULT 'ABIERTA'
                   CHECK (estado IN ('ABIERTA','EN_REPARACION','ENVIADA_A_SOPORTE','RESUELTA')),

    CONSTRAINT incidencia_categoria_check CHECK (categoria IS NULL OR (categoria <> '' AND categoria = btrim(categoria)))
);

COMMENT ON COLUMN incidencia.categoria IS
    'Qué tipo de falla es, en texto libre normalizado (ej: batería, pantalla). '
    'NULL cuando todavía no se pudo diagnosticar, que es un estado real y no '
    'un dato faltante. Los reportes agrupan por lower(categoria).';

COMMENT ON COLUMN incidencia.enviado_a_soporte IS
    'Si el equipo se mandó a reparar afuera. A dónde depende de la '
    'institución: un organismo educativo, un proveedor, un taller.';

CREATE INDEX idx_incidencia_equipo ON incidencia (equipo_id);

-- Agrupar por tipo de falla sin distinguir mayúsculas. Parcial porque las no
-- clasificadas se cuentan por separado y no entran en ese agrupamiento.
CREATE INDEX idx_incidencia_categoria ON incidencia (lower(categoria)) WHERE categoria IS NOT NULL;

-- "La última incidencia de este equipo", que es lo que pide el listado de
-- equipos fuera de circulación por cada fila que muestra.
CREATE INDEX idx_incidencia_equipo_fecha ON incidencia (equipo_id, fecha DESC);

-- ── Licencias de software con vencimiento (RF-03.11 a RF-03.14) ────────

CREATE TABLE licencia_software (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    equipo_id              UUID NOT NULL REFERENCES equipo(id) ON DELETE CASCADE,
    nombre                 VARCHAR(100) NOT NULL,

    dias_duracion          INTEGER NOT NULL CHECK (dias_duracion > 0 AND dias_duracion <= 3650),
    -- Cuántos días antes avisar. 0 = avisar solo el día del vencimiento.
    dias_aviso             INTEGER NOT NULL DEFAULT 1 CHECK (dias_aviso >= 0 AND dias_aviso <= 365),

    fecha_vencimiento      DATE,
    ultima_renovacion      DATE,
    vencimiento_fijado_por UUID REFERENCES usuario(id) ON DELETE SET NULL,
    vencimiento_fijado_en  TIMESTAMPTZ,

    -- Marcas del barrido: guardan PARA QUÉ DÍA salió cada aviso, no un
    -- booleano. Es lo que permite que el aviso se repita en la renovación
    -- siguiente sin volver a mandarlo mil veces por la misma.
    avisado_previo_para       DATE,
    avisado_vencimiento_para  DATE,

    creada_en              TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT licencia_software_nombre_check CHECK (nombre <> '' AND nombre = btrim(nombre))
);

COMMENT ON COLUMN licencia_software.fecha_vencimiento IS
    'Día en que vence. NULL = a verificar (todavía no se miró la máquina), '
    'no "no vence nunca": el job ignora estas filas.';

-- El mismo software no puede estar cargado dos veces en la misma máquina,
-- sin distinguir mayúsculas.
CREATE UNIQUE INDEX ux_licencia_equipo_nombre ON licencia_software (equipo_id, lower(nombre));

-- El barrido busca las que vencen pronto; las que están a verificar no
-- entran, así que el índice es parcial.
CREATE INDEX idx_licencia_vencimiento ON licencia_software (fecha_vencimiento) WHERE fecha_vencimiento IS NOT NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- Reservas (RF-04)
-- ═══════════════════════════════════════════════════════════════════════

-- Una serie recurrente: "todos los martes de 8 a 10 hasta que termine el
-- año". Genera reserva_grupo materializados, uno por fecha.
CREATE TABLE regla_recurrencia (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    materia_id    UUID NOT NULL REFERENCES materia(id),
    creado_por    UUID REFERENCES usuario(id) ON DELETE SET NULL,
    dia_semana    VARCHAR(10) NOT NULL,
    hora_inicio   TIME NOT NULL,
    hora_fin      TIME NOT NULL,
    fecha_inicio  DATE NOT NULL,
    fecha_fin     DATE NOT NULL,

    CHECK (hora_fin > hora_inicio),
    CHECK (fecha_fin >= fecha_inicio),
    -- La semana lectiva es de lunes a viernes. Una institución que dicte los
    -- sábados amplía este CHECK y el equivalente de horario_admin.
    CONSTRAINT chk_regla_recurrencia_dia_lectivo
        CHECK (dia_semana IN ('LUNES','MARTES','MIERCOLES','JUEVES','VIERNES'))
);

-- Lo que el docente percibe como "una reserva": una materia, una fecha, un
-- horario. Adentro hay una fila `reserva` por cada equipo elegido.
--
-- Separar las dos cosas no es normalización por deporte: el docente tilda
-- varias máquinas en una sola operación, pero las cancelaciones en cascada
-- actúan sobre UNA máquina puntual —la que se rompió, la que se bloqueó— y
-- el grupo tiene que poder quedar parcialmente cancelado.
CREATE TABLE reserva_grupo (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    materia_id               UUID NOT NULL REFERENCES materia(id),
    creado_por               UUID REFERENCES usuario(id) ON DELETE SET NULL,

    -- Copia del nombre al momento de reservar. Es lo que hace que el
    -- histórico de un año cerrado siga siendo legible aunque esa cuenta se
    -- elimine después.
    nombre_docente_snapshot  VARCHAR(200) NOT NULL,

    fecha                    DATE NOT NULL,
    hora_inicio              TIME NOT NULL,
    hora_fin                 TIME NOT NULL,

    estado                   VARCHAR(25) NOT NULL DEFAULT 'CONFIRMADA'
                             CHECK (estado IN ('CONFIRMADA','PARCIALMENTE_CANCELADA','CANCELADA','FINALIZADA','NO_RETIRADA')),

    regla_recurrencia_id     UUID REFERENCES regla_recurrencia(id) ON DELETE SET NULL,
    creada_en                TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Cuándo salió el recordatorio de "tu clase empieza pronto". Instante y
    -- no booleano: lo primero que se pregunta cuando alguien dice "no me
    -- llegó" es a qué hora fue.
    recordatorio_enviado_en  TIMESTAMPTZ,

    CHECK (hora_fin > hora_inicio)
);

CREATE INDEX idx_reserva_grupo_materia    ON reserva_grupo (materia_id);
CREATE INDEX idx_reserva_grupo_creado_por ON reserva_grupo (creado_por);
CREATE INDEX idx_reserva_grupo_regla      ON reserva_grupo (regla_recurrencia_id) WHERE regla_recurrencia_id IS NOT NULL;

-- El barrido de recordatorios: las que todavía no se avisaron.
CREATE INDEX idx_grupo_sin_recordar
    ON reserva_grupo (fecha, hora_inicio) WHERE recordatorio_enviado_en IS NULL AND estado = 'CONFIRMADA';

-- La ocupación de UN equipo en UNA franja. Es la unidad que protege la
-- constraint de anti-solapamiento.
CREATE TABLE reserva (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- NULL en los BLOQUEO, que no pertenecen a ningún grupo de docente.
    reserva_grupo_id         UUID REFERENCES reserva_grupo(id) ON DELETE CASCADE,
    equipo_id                UUID NOT NULL REFERENCES equipo(id),
    materia_id               UUID REFERENCES materia(id),
    nombre_docente_snapshot  VARCHAR(200),

    fecha                    DATE NOT NULL,
    hora_inicio              TIME NOT NULL,
    hora_fin                 TIME NOT NULL,

    estado                   VARCHAR(15) NOT NULL DEFAULT 'CONFIRMADA'
                             CHECK (estado IN ('CONFIRMADA','CANCELADA','FINALIZADA','NO_RETIRADA')),

    -- NORMAL: la reserva de un docente para su clase.
    -- BLOQUEO: un ADMIN se tomó el equipo para otra cosa —una evaluación,
    -- una jornada docente, una obra en el aula— y canceló lo que hubiera
    -- encima. El sistema no puede prever la lista de motivos, así que el
    -- motivo es texto libre y obligatorio.
    tipo                     VARCHAR(20) NOT NULL DEFAULT 'NORMAL'
                             CHECK (tipo IN ('NORMAL','BLOQUEO')),
    motivo_bloqueo           TEXT,

    creado_por               UUID REFERENCES usuario(id) ON DELETE SET NULL,
    creada_en                TIMESTAMPTZ NOT NULL DEFAULT now(),

    cancelado_por            UUID REFERENCES usuario(id) ON DELETE SET NULL,
    motivo_cancelacion       TEXT,
    cancelada_en             TIMESTAMPTZ,

    -- Cuándo se avisó que el equipo de esta reserva no volvió y no está
    -- disponible. Instante y no booleano, mismo criterio que el recordatorio.
    avisado_equipo_no_disponible_en TIMESTAMPTZ,

    CHECK (hora_fin > hora_inicio),

    -- Las dos formas válidas de existir, y ninguna otra. Un BLOQUEO sin
    -- motivo deja de ser representable, que es lo que hace que la regla valga
    -- también para lo que escriba otra aplicación contra esta base.
    CONSTRAINT chk_reserva_tipo_coherente CHECK (
        (tipo = 'NORMAL'
         AND reserva_grupo_id IS NOT NULL
         AND materia_id IS NOT NULL
         AND motivo_bloqueo IS NULL)
        OR
        (tipo = 'BLOQUEO'
         AND reserva_grupo_id IS NULL
         AND materia_id IS NULL
         AND motivo_bloqueo IS NOT NULL
         AND btrim(motivo_bloqueo) <> '')
    )
);

COMMENT ON COLUMN reserva.estado IS
    'NO_RETIRADA: nadie vino a buscar la máquina dentro del plazo de gracia, '
    'así que dejó de bloquear el horario. No es una cancelación.';

COMMENT ON COLUMN reserva.tipo IS
    'NORMAL: la reserva de un docente para su clase. BLOQUEO: un Admin se '
    'tomó el equipo para otra cosa y canceló lo que hubiera encima.';

COMMENT ON COLUMN reserva.motivo_bloqueo IS
    'Por qué se tomó el equipo, en texto libre: una evaluación, una jornada '
    'docente, una obra en el aula. Obligatorio en los BLOQUEO porque cancelan '
    'clases ajenas; NULL en las reservas normales, que ya dicen para qué son '
    'por su materia.';

-- ── La garantía que sostiene todo el sistema ───────────────────────────
--
-- Dos reservas confirmadas no pueden pisarse sobre el mismo equipo. Es una
-- constraint y NO una validación en Go a propósito: una validación se puede
-- ganar por carrera —dos docentes apretando "confirmar" en el mismo
-- milisegundo— y esto no. Vale además contra cualquier cosa que escriba
-- directo en la base.
--
-- El WHERE deja fuera las canceladas, finalizadas y no retiradas: una franja
-- que se liberó tiene que poder volver a reservarse.
ALTER TABLE reserva ADD CONSTRAINT no_solapamiento
    EXCLUDE USING gist (
        equipo_id WITH =,
        tsrange(fecha + hora_inicio, fecha + hora_fin) WITH &&
    ) WHERE (estado = 'CONFIRMADA');

CREATE INDEX idx_reserva_equipo_fecha ON reserva (equipo_id, fecha);
CREATE INDEX idx_reserva_creado_por   ON reserva (creado_por);
CREATE INDEX idx_reserva_grupo        ON reserva (reserva_grupo_id) WHERE reserva_grupo_id IS NOT NULL;
CREATE INDEX idx_reserva_materia      ON reserva (materia_id) WHERE materia_id IS NOT NULL;

-- El mostrador arma su pantalla con las confirmadas de hoy.
CREATE INDEX idx_reserva_confirmadas_del_dia ON reserva (fecha, hora_inicio) WHERE estado = 'CONFIRMADA';

-- ═══════════════════════════════════════════════════════════════════════
-- Entregas y devoluciones (RF-08)
-- ═══════════════════════════════════════════════════════════════════════

-- La custodia física de un equipo: quién lo tiene AHORA. No es una reserva,
-- y la diferencia es la razón de ser de esta tabla. La reserva es el derecho
-- a usar un equipo en una franja; el préstamo es dónde está la máquina. Los
-- dos existen por separado: hay reservas que nadie retiró, préstamos sin
-- reserva detrás, y préstamos que sobreviven a su reserva.
CREATE TABLE prestamo (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    equipo_id               UUID NOT NULL REFERENCES equipo(id),

    -- NULL = préstamo espontáneo, sin reserva detrás. Es un caso normal.
    -- ON DELETE SET NULL: al archivar un ciclo las reservas se borran y el
    -- préstamo tiene que sobrevivir — es el registro de que la máquina salió.
    reserva_id              UUID REFERENCES reserva(id) ON DELETE SET NULL,

    -- Quién RESPONDE por el equipo. Contra una reserva es siempre el docente
    -- que reservó: es a quien se le reclama si no vuelve. El nombre va
    -- siempre; el usuario solo si esa persona tiene cuenta.
    entregado_a_usuario_id  UUID REFERENCES usuario(id) ON DELETE SET NULL,
    entregado_a_nombre      VARCHAR(200) NOT NULL,

    -- Quién vino físicamente a buscarlo, si no fue quien responde. Opcional:
    -- a una institución le sirve para reconstruir quién pasó por el
    -- mostrador y a otra le sobra. No es una referencia a usuario porque casi
    -- nunca es alguien con cuenta.
    retirado_por            VARCHAR(200),

    motivo                  TEXT,

    -- NULL = no se pactó hora. "Vengo en un rato" es una respuesta honesta, y
    -- una hora inventada solo generaría reclamos falsos.
    devolucion_estimada     TIMESTAMPTZ,

    entregado_por           UUID REFERENCES usuario(id) ON DELETE SET NULL,
    entregado_en            TIMESTAMPTZ NOT NULL DEFAULT now(),

    devuelto_en             TIMESTAMPTZ,
    recibido_por            UUID REFERENCES usuario(id) ON DELETE SET NULL,
    observaciones           TEXT,

    -- Marcas del barrido de vigilancia.
    avisado_demora_en       TIMESTAMPTZ,
    avisado_cierre_para     DATE,

    CONSTRAINT chk_prestamo_devolucion_coherente CHECK (devuelto_en IS NULL OR devuelto_en >= entregado_en),
    CONSTRAINT prestamo_entregado_a_nombre_check CHECK (entregado_a_nombre <> '' AND entregado_a_nombre = btrim(entregado_a_nombre)),
    CONSTRAINT prestamo_retirado_por_check       CHECK (retirado_por IS NULL OR (retirado_por <> '' AND retirado_por = btrim(retirado_por)))
);

COMMENT ON TABLE prestamo IS
    'Custodia física de un equipo: quién lo tiene ahora. Distinta de reserva, '
    'que es el derecho a usarlo en una franja.';

COMMENT ON COLUMN prestamo.devuelto_en IS
    'NULL = el equipo todavía está afuera. Define qué préstamos están abiertos.';

COMMENT ON COLUMN prestamo.retirado_por IS
    'Quién vino físicamente a buscar el equipo, si no fue quien responde por '
    'él. NULL = lo retiró la misma persona de entregado_a_nombre. No cambia de '
    'quién es la responsabilidad: eso lo dice entregado_a_nombre.';

COMMENT ON COLUMN prestamo.avisado_cierre_para IS
    'Jornada para la que ya salió el aviso de cierre. Es una fecha y no un '
    'booleano porque el corte se repite cada día que el equipo siga afuera.';

-- Un equipo no puede estar entregado dos veces a la vez. Como el
-- anti-solapamiento de reservas, es la base la que lo garantiza: dos Admin
-- en el mostrador pueden apretar "entregar" al mismo tiempo.
CREATE UNIQUE INDEX ux_prestamo_abierto ON prestamo (equipo_id) WHERE devuelto_en IS NULL;

CREATE INDEX idx_prestamo_abiertos ON prestamo (entregado_en) WHERE devuelto_en IS NULL;
CREATE INDEX idx_prestamo_equipo   ON prestamo (equipo_id, entregado_en DESC);
CREATE INDEX idx_prestamo_reserva  ON prestamo (reserva_id) WHERE reserva_id IS NOT NULL;

-- El barrido de demorados: los que ya deberían haber vuelto y todavía no se
-- reclamaron. Los tres filtros juntos porque los tres van en el mismo WHERE.
CREATE INDEX idx_prestamo_demorados_sin_avisar
    ON prestamo (devolucion_estimada)
    WHERE devuelto_en IS NULL AND avisado_demora_en IS NULL AND devolucion_estimada IS NOT NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- Notificaciones (RF-05)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE notificacion (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id        UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    reserva_id        UUID REFERENCES reserva(id) ON DELETE SET NULL,

    -- Sobre quién es el aviso, cuando no es sobre quien lo recibe: un ADMIN
    -- recibe "hay una cuenta esperando aprobación" y esta columna dice de
    -- quién. Permite borrar el aviso si esa cuenta se resuelve o se elimina.
    sobre_usuario_id  UUID REFERENCES usuario(id) ON DELETE CASCADE,

    mensaje           TEXT NOT NULL,
    estado            VARCHAR(10) NOT NULL DEFAULT 'NO_LEIDA' CHECK (estado IN ('NO_LEIDA','LEIDA')),
    creada_en         TIMESTAMPTZ NOT NULL DEFAULT now(),
    leida_en          TIMESTAMPTZ,

    -- El tipo SÍ es un enum: sobre él decide el sistema (qué ícono mostrar,
    -- qué avisos deduplicar), no la institución.
    tipo              VARCHAR(30) NOT NULL DEFAULT 'GENERAL',

    CONSTRAINT chk_notificacion_tipo CHECK (tipo IN (
        'GENERAL',
        'DOCENTE_PENDIENTE',
        'RESERVA_CANCELADA',
        'LICENCIA_POR_VENCER',
        'RESERVA_POR_COMENZAR',
        'RESERVA_NO_RETIRADA',
        'EQUIPO_SIN_DEVOLVER'
    ))
);

CREATE INDEX idx_notif_usuario_estado ON notificacion (usuario_id, estado);
CREATE INDEX idx_notif_sobre_usuario  ON notificacion (sobre_usuario_id, tipo) WHERE sobre_usuario_id IS NOT NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- Disponibilidad de los Admin (RF-07) — puramente informativo
-- ═══════════════════════════════════════════════════════════════════════

-- El horario semanal habitual de cada ADMIN. Es un patrón recurrente, no una
-- serie de filas por semana: editarlo aplica hacia adelante sin necesidad de
-- una acción aparte.
CREATE TABLE horario_admin (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id   UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    dia_semana   VARCHAR(10) NOT NULL,
    hora_inicio  TIME NOT NULL,
    hora_fin     TIME NOT NULL,

    CHECK (hora_fin > hora_inicio),
    CONSTRAINT chk_horario_admin_dia_lectivo
        CHECK (dia_semana IN ('LUNES','MARTES','MIERCOLES','JUEVES','VIERNES'))
);

CREATE INDEX idx_horario_admin_usuario ON horario_admin (usuario_id);

-- Un día puntual distinto del patrón: ausencia total o un horario cambiado.
CREATE TABLE horario_admin_excepcion (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id   UUID NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    fecha        DATE NOT NULL,
    tipo         VARCHAR(20) NOT NULL CHECK (tipo IN ('NO_DISPONIBLE','HORARIO_MODIFICADO')),
    hora_inicio  TIME,
    hora_fin     TIME,
    motivo       TEXT,

    -- Una ausencia no lleva horario y un horario modificado sí. Sin esto,
    -- una fila podía decir las dos cosas a la vez.
    CONSTRAINT chk_excepcion_horario_coherente CHECK (
        (tipo = 'NO_DISPONIBLE'      AND hora_inicio IS NULL AND hora_fin IS NULL)
        OR
        (tipo = 'HORARIO_MODIFICADO' AND hora_inicio IS NOT NULL AND hora_fin IS NOT NULL AND hora_fin > hora_inicio)
    ),

    UNIQUE (usuario_id, fecha)
);

CREATE INDEX idx_horario_excepcion_usuario_fecha ON horario_admin_excepcion (usuario_id, fecha);

-- ═══════════════════════════════════════════════════════════════════════
-- Histórico de ciclos archivados (RF-06.4)
-- ═══════════════════════════════════════════════════════════════════════
--
-- Al cerrar un año lectivo se eliminan físicamente sus reservas, pero antes
-- se calcula este resumen. Es la única fuente de verdad para años cerrados,
-- porque el detalle ya no existe.
--
-- Guarda los nombres CONGELADOS: un equipo que después se mude de carro, o un
-- docente cuya cuenta se elimine, siguen apareciendo correctamente en el
-- reporte del año que ya pasó.

CREATE TABLE historico_uso_equipo (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    anio                    INTEGER NOT NULL,
    equipo_id               UUID NOT NULL REFERENCES equipo(id),
    etiqueta_snapshot       VARCHAR(100) NOT NULL,
    identificador_snapshot  INTEGER,
    carro_nombre_snapshot   VARCHAR(100),
    minutos_reservados      INTEGER NOT NULL,
    cantidad_reservas       INTEGER NOT NULL,
    UNIQUE (anio, equipo_id)
);

CREATE INDEX idx_historico_equipo_anio ON historico_uso_equipo (anio);

CREATE TABLE historico_uso_docente (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    anio                     INTEGER NOT NULL,
    -- ON DELETE SET NULL: el resumen del año sobrevive aunque la cuenta se
    -- elimine, porque el nombre está congelado en la fila.
    usuario_id               UUID REFERENCES usuario(id) ON DELETE SET NULL,
    nombre_docente_snapshot  VARCHAR(200) NOT NULL,
    cantidad_reservas        INTEGER NOT NULL,
    minutos_totales          INTEGER NOT NULL,
    UNIQUE (anio, usuario_id)
);

CREATE INDEX idx_historico_docente_anio ON historico_uso_docente (anio);

-- ═══════════════════════════════════════════════════════════════════════
-- Auditoría
-- ═══════════════════════════════════════════════════════════════════════
--
-- Toda acción sensible queda registrada con quién, cuándo y desde qué
-- dirección. Se escribe y no se toca: existe para poder reconstruir qué pasó
-- cuando alguien reclama algo de hace meses.
--
-- `accion` es texto libre y no un enum a propósito: los valores guardados son
-- el registro de cómo se llamaba una operación EN SU MOMENTO. Si el sistema
-- renombra algo, las filas viejas conservan el nombre viejo — reescribir un
-- registro de auditoría es precisamente lo que un registro de auditoría no
-- debe permitir.
CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id  UUID NOT NULL,
    accion      VARCHAR(100) NOT NULL,
    entidad     VARCHAR(50) NOT NULL,
    entidad_id  UUID,
    detalle     JSONB,
    ip_origen   INET,
    creado_en   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sin FK a usuario a propósito: si una cuenta se elimina, lo que hizo tiene
-- que seguir registrado.
CREATE INDEX idx_audit_usuario ON audit_log (usuario_id, creado_en DESC);
