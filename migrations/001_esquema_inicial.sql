-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Esquema inicial
-- ═══════════════════════════════════════════════════════════════════════
--
-- Sistema de reserva de equipos informáticos para una institución
-- educativa. Este archivo es el esquema completo: se aplica solo, sobre una
-- base vacía, y deja el sistema listo para arrancar.
--
-- Lo aplica el binario al arrancar, con goose, contra una base vacía o ya
-- creada: goose lleva la cuenta de qué migraciones corrieron en la tabla
-- `goose_db_version` y no repite ninguna. Antes esto lo hacía Postgres vía
-- docker-entrypoint-initdb.d, que corre UNA sola vez —cuando el volumen se
-- crea— y por eso una base ya existente se quedaba en el esquema viejo sin
-- que nada avisara. Ver `docs/11-operacion.md`.
--
-- Las dos anotaciones de goose que aparecen más abajo —la de subida y la de
-- bajada— no son decorativas: parten el archivo en dos. Lo que va después de
-- la primera se aplica; lo que va después de la segunda es cómo se deshace.
-- Cuidado al comentar sobre ellas: goose lee TODAS las líneas que llevan su
-- marca, así que escribirla de nuevo dentro de un comentario rompe el
-- archivo con un error de anotación duplicada.
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

-- +goose Up

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
-- Bloques que cruzan la medianoche
-- ═══════════════════════════════════════════════════════════════════════

-- Cuándo termina de verdad un bloque, dado el día que lo nombra.
--
-- Las escuelas nocturnas dictan de 22:00 a 01:00, y eso antes era
-- inexpresable: la base exigía hora_fin > hora_inicio. La regla es la que
-- cualquiera lee sin que se la expliquen:
--
--   hora_fin > hora_inicio  → termina el mismo día
--   hora_fin < hora_inicio  → termina al día siguiente
--   hora_fin = hora_inicio  → inválido (lo rechazan los CHECK)
--
-- Existe como función y no repetida en cada consulta porque la usan la
-- constraint de anti-solapamiento, los barridos y todos los listados: una
-- copia que se olvide de sumar el día no rompe nada visible, simplemente
-- deja de ver las clases nocturnas.
--
-- IMMUTABLE no es decorativo: sin eso Postgres no la acepta dentro del
-- índice de la constraint EXCLUDE. Lo es de verdad — misma entrada, misma
-- salida, sin leer nada de afuera.
--
-- El gemelo de esto en Go es domain.FinDePared. Las dos tienen que decir lo
-- mismo: si divergen, la aplicación acepta reservas que la base rechaza o,
-- peor, deja pasar solapamientos que creía haber chequeado.
CREATE OR REPLACE FUNCTION fin_de_pared(fecha DATE, hora_inicio TIME, hora_fin TIME)
RETURNS TIMESTAMP
LANGUAGE SQL
IMMUTABLE
STRICT
PARALLEL SAFE
AS $$
    SELECT fecha + hora_fin
         + CASE WHEN hora_fin < hora_inicio THEN INTERVAL '1 day' ELSE INTERVAL '0' END
$$;

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
    -- Si se ofrece como titular o como suplente. A diferencia de los dos de
    -- arriba SÍ lleva CHECK: es la misma lista cerrada de docente_materia.rol
    -- y nombra algo que existe siempre, mientras que un curso o una materia
    -- pueden no existir todavía. Sigue siendo una declaración, no un vínculo:
    -- el rol que rige es el que el ADMIN carga al asignar (RF-02.6).
    rol_solicitado         VARCHAR(10) CHECK (rol_solicitado IN ('TITULAR','SUPLENTE')),

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

    -- La forma canónica del nombre —sin mayúsculas ni acentos— contra la que
    -- se cruzan las marcas de preferencia de equipo_preferencia (RF-03.21).
    --
    -- Se resuelve con translate() y NO con unaccent(): esa función vive en una
    -- extensión, depende de un diccionario y por eso no es IMMUTABLE, así que
    -- no se puede usar ni en una columna generada ni en un índice. Las cinco
    -- vocales acentuadas más ü y ñ cubren el castellano.
    nombre_norm VARCHAR(100)
                GENERATED ALWAYS AS (translate(lower(nombre), 'áéíóúüñ', 'aeiouun')) STORED,

    activo     BOOLEAN NOT NULL DEFAULT true,
    archivado  BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (curso_id, nombre)
);

CREATE INDEX idx_materia_curso ON materia (curso_id);
CREATE INDEX idx_materia_nombre_norm ON materia (nombre_norm);

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

-- ── Qué materia prefiere cada equipo (RF-03.21) ────────────────────────
--
-- El ADMIN marca en el inventario que una máquina es preferente para una
-- materia, y opcionalmente sólo para un año o para un año y división. Al
-- reservar, esa materia la ve primero y las demás la ven al final.
--
-- La marca SÓLO ORDENA. No es un permiso, no oculta nada y no bloquea a
-- nadie: cualquiera puede reservar cualquier equipo libre. Esa decisión es
-- la que hace que marcar equipos con reservas ya hechas no tenga ningún
-- conflicto posible —no hay nada que cancelar ni que revisar— y que borrar
-- una marca sea gratis.
--
-- El vínculo con la materia es por NOMBRE y no una FK: `materia` es por curso
-- y ArchivarYClonar la recrea con un UUID nuevo cada año (RF-02.5); `curso`
-- también. Una FK a cualquiera de las dos borraría todas las marcas el 31/12,
-- que es justo cuando el ADMIN espera que sigan puestas. Por lo mismo, el año
-- y la división se guardan como número y letra sueltos.
--
-- El nombre no se fragmenta por tipeo porque el ADMIN es el único que lo
-- escribe: quien se registra sólo lo SUGIERE en texto libre
-- (usuario.materia_solicitada), y es el ADMIN quien crea la materia. Aun así
-- el match va por nombre normalizado, para que "Matemática" y "matematica"
-- sean la misma.
CREATE TABLE equipo_preferencia (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    equipo_id       UUID NOT NULL REFERENCES equipo(id) ON DELETE CASCADE,

    -- El nombre tal como lo eligió el ADMIN de la lista de materias que ya
    -- existen. Se guarda con su capitalización porque es lo que se muestra
    -- ("preferente para Dibujo Técnico"); el match usa la columna generada.
    materia_nombre  VARCHAR(100) NOT NULL,
    materia_norm    VARCHAR(100)
                    GENERATED ALWAYS AS (translate(lower(materia_nombre), 'áéíóúüñ', 'aeiouun')) STORED,

    -- El alcance, de menos a más específico:
    --   (NULL, NULL) → toda materia con ese nombre, en cualquier curso
    --   (3,    NULL) → sólo las de tercer año
    --   (3,    'B')  → sólo 3°B
    -- Los rangos son los mismos del CHECK de curso.nombre ('^[1-6]°[A-Z]$'):
    -- una institución con otra nomenclatura cambia los dos juntos.
    anio            SMALLINT CHECK (anio BETWEEN 1 AND 6),
    division        CHAR(1)  CHECK (division ~ '^[A-Z]$'),

    -- Cuál manda cuando una misma máquina es preferente de varias materias:
    -- 1 es la más fuerte. Es un número y no un enum para que la escuela
    -- defina sus propios rangos sin tocar código.
    prioridad       SMALLINT NOT NULL DEFAULT 1 CHECK (prioridad BETWEEN 1 AND 9),

    creada_en       TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Una división sin año no significa nada: no existen "todas las B". El
    -- alcance se abre de a un nivel.
    CONSTRAINT chk_equipo_preferencia_alcance
        CHECK (division IS NULL OR anio IS NOT NULL),
    CONSTRAINT chk_equipo_preferencia_materia
        CHECK (materia_nombre <> '' AND materia_nombre = btrim(materia_nombre)),

    -- NULLS NOT DISTINCT no es un detalle: con el UNIQUE normal de SQL dos
    -- filas (equipo, materia, NULL, NULL) se consideran distintas entre sí,
    -- así que la misma marca se podría cargar infinitas veces.
    CONSTRAINT ux_equipo_preferencia
        UNIQUE NULLS NOT DISTINCT (equipo_id, materia_norm, anio, division)
);

COMMENT ON TABLE equipo_preferencia IS
    'Marcas de preferencia del inventario (RF-03.21). Sólo ordenan la lista '
    'al reservar: no restringen a nadie ni afectan ninguna reserva existente.';

-- El ordenamiento entra por el nombre de la materia: para una reserva se
-- conocen materia, año y división, y hay que encontrar qué equipos las
-- prefieren. El panel del inventario entra por el otro lado.
CREATE INDEX idx_equipo_preferencia_materia ON equipo_preferencia (materia_norm);
CREATE INDEX idx_equipo_preferencia_equipo  ON equipo_preferencia (equipo_id);

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

    -- El fin puede ser MENOR que el inicio: eso significa que el bloque termina
    -- al día siguiente (22:00–01:00). Lo que no puede es ser igual, que sería
    -- un bloque de cero horas o de veinticuatro y en la práctica es un tipeo.
    CHECK (hora_fin <> hora_inicio),
    CHECK (fecha_fin >= fecha_inicio),
    -- Los siete días. Qué días opera la institución NO se decide acá: se
    -- declara en jornada_institucion y se valida en la aplicación. Este
    -- CHECK solo fija el vocabulario del enum, para que valga también
    -- contra cualquier cosa que escriba directo en la base.
    CONSTRAINT chk_regla_recurrencia_dia_valido
        CHECK (dia_semana IN ('LUNES','MARTES','MIERCOLES','JUEVES','VIERNES','SABADO','DOMINGO'))
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

    -- Cuándo salió el aviso de "todavía no las retiraste" (RF-08.20), el que
    -- avisa que pasados los minutos de gracia la reserva queda libre. Es una
    -- columna aparte de la anterior y no un contador porque son dos avisos
    -- con condiciones distintas: el recordatorio sale siempre, una hora
    -- antes; este solo si a los quince minutos del inicio no salió ninguna
    -- máquina de esta reserva.
    aviso_sin_retirar_en     TIMESTAMPTZ,

    -- El fin puede ser MENOR que el inicio: eso significa que el bloque termina
    -- al día siguiente (22:00–01:00). Lo que no puede es ser igual, que sería
    -- un bloque de cero horas o de veinticuatro y en la práctica es un tipeo.
    CHECK (hora_fin <> hora_inicio)
);

CREATE INDEX idx_reserva_grupo_materia    ON reserva_grupo (materia_id);
CREATE INDEX idx_reserva_grupo_creado_por ON reserva_grupo (creado_por);
CREATE INDEX idx_reserva_grupo_regla      ON reserva_grupo (regla_recurrencia_id) WHERE regla_recurrencia_id IS NOT NULL;

-- El barrido de recordatorios: las que todavía no se avisaron.
CREATE INDEX idx_grupo_sin_recordar
    ON reserva_grupo (fecha, hora_inicio) WHERE recordatorio_enviado_en IS NULL AND estado = 'CONFIRMADA';

-- Y el mismo patrón para el aviso de no retiro, por la misma razón: el
-- barrido corre cada cinco minutos y no puede recorrer la tabla entera para
-- descartar las que ya avisó.
CREATE INDEX idx_grupo_sin_aviso_retiro
    ON reserva_grupo (fecha, hora_inicio) WHERE aviso_sin_retirar_en IS NULL AND estado = 'CONFIRMADA';

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

    -- El fin puede ser MENOR que el inicio: eso significa que el bloque termina
    -- al día siguiente (22:00–01:00). Lo que no puede es ser igual, que sería
    -- un bloque de cero horas o de veinticuatro y en la práctica es un tipeo.
    CHECK (hora_fin <> hora_inicio),

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
--
-- El fin sale de fin_de_pared() y no de `fecha + hora_fin`: con una clase
-- nocturna, esa suma da un instante ANTERIOR al inicio, tsrange revienta con
-- "range lower bound must be less than or equal to range upper bound" y la
-- reserva no se puede ni insertar. Antes no pasaba porque ningún bloque podía
-- cruzar las 00:00.
ALTER TABLE reserva ADD CONSTRAINT no_solapamiento
    EXCLUDE USING gist (
        equipo_id WITH =,
        tsrange(fecha + hora_inicio, fin_de_pared(fecha, hora_inicio, hora_fin)) WITH &&
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
        'EQUIPO_SIN_DEVOLVER',
        'PEDIDO_DE_LIBERACION',
        'PEDIDO_DE_MATERIA',
        'PEDIDO_DE_MATERIA_RESUELTO',
        'SUGERENCIA',
        'SUGERENCIA_RESPONDIDA'
    ))
);

CREATE INDEX idx_notif_usuario_estado ON notificacion (usuario_id, estado);
CREATE INDEX idx_notif_sobre_usuario  ON notificacion (sobre_usuario_id, tipo) WHERE sobre_usuario_id IS NOT NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- Jornada de la institución — normativo
-- ═══════════════════════════════════════════════════════════════════════

-- Qué días y entre qué horas abre la escuela. Es la única tabla del sistema
-- sin dueño: describe a la institución entera, no a una persona.
--
-- Existe porque esto estaba hardcodeado. El código daba por sentado "lunes a
-- viernes" y ninguna escuela podía decir lo contrario, lo cual dejaba afuera
-- a las de jornada extendida o albergue —que dictan el fin de semana— y no
-- decía nada de las horas.
--
-- Tabla VACÍA significa "todavía no lo declararon", y en ese caso no hay
-- restricción: el sistema no supone un calendario que nadie le dijo. Con
-- filas cargadas, un día sin filas es un día en que la escuela no abre. Las
-- dos situaciones se ven parecidas y significan lo contrario, así que la
-- validación mira la tabla completa y no solo el día que le preguntan (ver
-- PermiteReserva en availability/domain).
--
-- Varias filas por día a propósito: una escuela con turno mañana y turno
-- noche declara 07:00–12:00 y 18:00–23:00, y el mediodía queda afuera.
CREATE TABLE jornada_institucion (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dia_semana   VARCHAR(10) NOT NULL,
    hora_inicio  TIME NOT NULL,
    hora_fin     TIME NOT NULL,

    -- El fin puede ser MENOR que el inicio: eso significa que el bloque termina
    -- al día siguiente (22:00–01:00). Lo que no puede es ser igual, que sería
    -- un bloque de cero horas o de veinticuatro y en la práctica es un tipeo.
    CHECK (hora_fin <> hora_inicio),
    CONSTRAINT chk_jornada_dia_valido
        CHECK (dia_semana IN ('LUNES','MARTES','MIERCOLES','JUEVES','VIERNES','SABADO','DOMINGO'))
);

-- El solapamiento entre bloques del mismo día se rechaza en la aplicación,
-- no con una constraint EXCLUDE. Es la misma decisión que en horario_admin,
-- su tabla hermana, y por la misma razón: la tabla es chica y de escritura
-- casi nula —una escuela declara su jornada una vez— así que la EXCLUDE
-- compraría poco, y a cambio obligaría a un tipo de rango sobre TIME que
-- Postgres no trae. La de `reserva` sí existe porque ahí hay concurrencia
-- real entre docentes reservando la misma máquina.
--
-- Tocarse no es pisarse: 07:00–12:00 y 12:00–18:00 son contiguos y válidos.

CREATE INDEX idx_jornada_dia ON jornada_institucion (dia_semana);

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
    CONSTRAINT chk_horario_admin_dia_valido
        CHECK (dia_semana IN ('LUNES','MARTES','MIERCOLES','JUEVES','VIERNES','SABADO','DOMINGO'))
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

-- ═══════════════════════════════════════════════════════════════════════
-- Perfil, pedidos de materia y buzón de sugerencias
-- ═══════════════════════════════════════════════════════════════════════

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


-- +goose Down

-- Deshacer el esquema inicial es borrar la base entera: no hay estado
-- anterior al que volver, porque esta migración ES el punto de partida.
--
-- Está escrito de verdad, y no vacío, porque un `down` que no hace nada y
-- termina bien es peor que uno que no existe: goose lo marca como
-- revertido, la tabla goose_db_version dice que la migración no está
-- aplicada, y las tablas siguen ahí. La próxima corrida intentaría crear lo
-- que ya existe y fallaría con un error que no menciona nada de esto.
--
-- Por eso mismo no hay un atajo en el Makefile para llegar acá, igual que
-- no lo hay para `docker compose down -v`: destruye los datos y no se puede
-- deshacer.
--
-- El orden es el inverso al de creación. CASCADE se encarga igual de las
-- foreign keys, pero mantenerlo legible es lo que permite revisarlo.

DROP TABLE IF EXISTS audit_log CASCADE;
DROP TABLE IF EXISTS historico_uso_docente CASCADE;
DROP TABLE IF EXISTS sugerencia CASCADE;
DROP TABLE IF EXISTS pedido_de_materia CASCADE;
DROP TABLE IF EXISTS foto_de_perfil CASCADE;
DROP TABLE IF EXISTS historico_uso_equipo CASCADE;
DROP TABLE IF EXISTS horario_admin_excepcion CASCADE;
DROP TABLE IF EXISTS horario_admin CASCADE;
DROP TABLE IF EXISTS jornada_institucion CASCADE;
DROP TABLE IF EXISTS notificacion CASCADE;
DROP TABLE IF EXISTS prestamo CASCADE;
DROP TABLE IF EXISTS reserva CASCADE;
DROP TABLE IF EXISTS reserva_grupo CASCADE;
DROP TABLE IF EXISTS regla_recurrencia CASCADE;
DROP TABLE IF EXISTS licencia_software CASCADE;
DROP TABLE IF EXISTS incidencia CASCADE;
DROP TABLE IF EXISTS equipo CASCADE;
DROP TABLE IF EXISTS carro CASCADE;
DROP TABLE IF EXISTS equipo_preferencia CASCADE;
DROP TABLE IF EXISTS docente_materia CASCADE;
DROP TABLE IF EXISTS materia CASCADE;
DROP TABLE IF EXISTS curso CASCADE;
DROP TABLE IF EXISTS ciclo_lectivo CASCADE;
DROP TABLE IF EXISTS codigo_recuperacion CASCADE;
DROP TABLE IF EXISTS usuario CASCADE;

DROP FUNCTION IF EXISTS fin_de_pared(DATE, TIME, TIME);

-- Las extensiones NO se borran: pgcrypto y btree_gist pueden estar en uso
-- por otra cosa en la misma base, y volver a crearlas es gratis.
