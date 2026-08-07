-- SGRC — Quién tiene cada computadora ahora mismo
--
-- Hasta hoy esto se anota en un papel: qué máquinas se lleva cada docente y
-- cuáles devuelve. Esta tabla lo reemplaza.
--
-- ══════════════════════════════════════════════════════════════════
-- Por qué no alcanza con `reserva`
-- ══════════════════════════════════════════════════════════════════
-- El papel y la reserva registran dos cosas distintas, y confundirlas es lo
-- que impide reemplazar uno con la otra:
--
--   reserva   = el DERECHO a usar una PC en una franja horaria
--   prestamo  = quién TIENE la máquina en este momento
--
-- Los tres casos de la escuela lo muestran:
--
--   * Reserva sin préstamo: el docente que reservó y no vino a buscarlas.
--   * Préstamo sin reserva: "necesito una compu para hacer un trámite",
--     que es espontáneo por definición y no se puede planificar.
--   * Préstamo que sobrevive a su reserva: la clase terminó a las 9:00 y a
--     las 9:20 las máquinas siguen sin volver. Ese es justamente el problema
--     que hoy no tiene forma de detectarse.
--
-- ══════════════════════════════════════════════════════════════════
-- "¿Dónde está la PC 3?" se deriva, no se guarda
-- ══════════════════════════════════════════════════════════════════
-- No hay ninguna columna en `pc` que diga "prestada". El estado sale de si
-- existe un préstamo abierto para esa PC, y por eso no puede quedar
-- desincronizado — que es exactamente lo que le pasa al papel cuando alguien
-- devuelve una máquina y nadie tacha el renglón.
--
-- Es la misma decisión que el contador de licencias (012): lo que se puede
-- calcular no se guarda.

BEGIN;

CREATE TABLE prestamo (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    pc_id       UUID NOT NULL REFERENCES pc(id),

    -- NULL = préstamo espontáneo, sin reserva detrás. Es un caso normal y
    -- frecuente, no una excepción.
    --
    -- ON DELETE SET NULL y no CASCADE: al archivar un ciclo lectivo se
    -- borran físicamente sus reservas (RF-02.4), y el registro de que
    -- alguien se llevó una máquina el 12 de agosto vale por sí mismo aunque
    -- la reserva que lo originó ya no exista. Mismo criterio que
    -- notificacion.reserva_id.
    reserva_id  UUID REFERENCES reserva(id) ON DELETE SET NULL,

    -- A quién se la llevó. El nombre va SIEMPRE, aunque además haya cuenta:
    -- es un snapshot, igual que reserva_grupo.nombre_docente_snapshot. Si la
    -- cuenta se elimina definitivamente (RF-01.9), el registro tiene que
    -- seguir diciendo quién se llevó la máquina.
    --
    -- El usuario es opcional porque quien pide una PC para un trámite puede
    -- no tener cuenta en el sistema: una preceptora, alguien de secretaría,
    -- un alumno. Obligar a que sea un usuario significaría o crear cuentas
    -- que nadie va a usar, o anotar el préstamo a nombre de otro.
    entregado_a_usuario_id UUID REFERENCES usuario(id) ON DELETE SET NULL,
    entregado_a_nombre     VARCHAR(200) NOT NULL
                           CHECK (entregado_a_nombre <> '' AND entregado_a_nombre = btrim(entregado_a_nombre)),

    motivo      TEXT,

    -- Cuándo debería volver. En un préstamo contra reserva sale del fin de
    -- esa reserva; en uno espontáneo es opcional, porque "vengo en un rato"
    -- es la respuesta honesta y una hora inventada solo generaría reclamos
    -- falsos.
    devolucion_estimada TIMESTAMPTZ,

    entregado_por UUID REFERENCES usuario(id) ON DELETE SET NULL,
    entregado_en  TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- NULL = todavía está afuera. Es la columna que define "abierto".
    devuelto_en   TIMESTAMPTZ,
    recibido_por  UUID REFERENCES usuario(id) ON DELETE SET NULL,

    -- Lo que se anota al recibir: "volvió sin el cargador", "la pantalla
    -- tiene una marca". Es el renglón al margen del papel.
    observaciones TEXT,

    CONSTRAINT chk_prestamo_devolucion_coherente CHECK (
        devuelto_en IS NULL OR devuelto_en >= entregado_en
    )
);

-- ══════════════════════════════════════════════════════════════════
-- Una PC no puede estar en dos manos
-- ══════════════════════════════════════════════════════════════════
-- Esta es la garantía que el papel no puede dar y la razón principal para
-- que esto viva en la base y no en una planilla: sin ella, entregar dos
-- veces la misma máquina —porque dos Admin la anotaron a la vez, o porque
-- nadie vio que ya estaba afuera— no lo detecta nadie hasta que aparece un
-- docente sin computadora.
--
-- Parcial (WHERE devuelto_en IS NULL) para que sí se pueda tener el
-- historial completo: la misma PC prestada cien veces son cien filas, pero
-- como mucho una abierta.
CREATE UNIQUE INDEX ux_prestamo_abierto ON prestamo(pc_id) WHERE devuelto_en IS NULL;

-- Los dos accesos reales: "qué hay afuera ahora" y el historial de una PC.
CREATE INDEX idx_prestamo_abiertos ON prestamo(entregado_en) WHERE devuelto_en IS NULL;
CREATE INDEX idx_prestamo_pc ON prestamo(pc_id, entregado_en DESC);
CREATE INDEX idx_prestamo_reserva ON prestamo(reserva_id) WHERE reserva_id IS NOT NULL;

COMMENT ON TABLE prestamo IS
    'Custodia física de una PC: quién la tiene ahora. Distinta de reserva, '
    'que es el derecho a usarla en una franja. Ver 013.';

COMMENT ON COLUMN prestamo.devuelto_en IS
    'NULL = la máquina todavía está afuera. Define qué préstamos están abiertos.';

COMMIT;
