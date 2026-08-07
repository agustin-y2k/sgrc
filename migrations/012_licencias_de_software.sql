-- SGRC — Vencimiento de licencias de software por PC
--
-- Una de las PCs tiene AutoCAD con licencia que vence cada 30 días. Cuando
-- vence, deja de abrir, y el Admin se entera el día que un docente no puede
-- dar la clase. Lo que falta no es una pantalla más de inventario: es saber
-- de antemano qué día hay que renovar.
--
-- Puede ser cualquier software, más de uno por PC, en PCs del mismo carro o
-- de carros distintos, y la duración puede cambiar (de 30 a 60 días) sin
-- aviso.
--
-- ══════════════════════════════════════════════════════════════════
-- El contador NO se guarda
-- ══════════════════════════════════════════════════════════════════
-- Los días que faltan son `fecha_vencimiento - hoy`, calculado al leer. No
-- hay ninguna columna que haya que decrementar todos los días, así que un
-- servidor apagado tres días no descuadra nada — mismo criterio que el
-- vencimiento de reservas (RF-04.9), que compara contra el instante actual
-- en vez de llevar una cuenta.
--
-- ══════════════════════════════════════════════════════════════════
-- Por qué fecha_vencimiento admite NULL
-- ══════════════════════════════════════════════════════════════════
-- NULL significa "todavía no sé cuándo vence", no "no vence nunca".
--
-- Es el estado real al cargar el inventario: el Admin sabe que la PC tiene
-- AutoCAD antes de poder sentarse delante de la máquina a mirar cuántos días
-- le quedan. Con la columna NOT NULL, la única salida sería inventar una
-- fecha — y la tentación es poner "se renovó hoy", que es el error que falla
-- en la dirección peligrosa: si en realidad vencía en tres días, el sistema
-- regala treinta de silencio justo cuando tendría que estar avisando.
--
-- Un contador que miente para arriba es peor que ninguno, porque además hace
-- que se deje de mirar la máquina. El job ignora las filas sin fecha y la
-- pantalla las muestra arriba de todo para que se completen.

BEGIN;

CREATE TABLE licencia_software (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- ON DELETE CASCADE: la licencia no significa nada sin su PC. Hoy las
    -- PCs se dan de baja con soft delete y la fila sobrevive, pero la
    -- cascada deja cerrado el caso de un borrado real (mismo criterio que
    -- notificacion.sobre_usuario_id en la 007).
    pc_id              UUID NOT NULL REFERENCES pc(id) ON DELETE CASCADE,

    -- El nombre lo escribe el Admin ("AutoCAD 2027"). Sin catálogo: para
    -- una escuela con un puñado de programas, mantener una tabla de
    -- software sería más trabajo que el que ahorra.
    nombre             VARCHAR(100) NOT NULL
                       CHECK (nombre <> '' AND nombre = btrim(nombre)),

    -- Cuánto dura una renovación. Es el paso que usa el botón "Renovar",
    -- no una propiedad inmutable: cambiarlo de 30 a 60 es un caso previsto.
    dias_duracion      INTEGER NOT NULL
                       CHECK (dias_duracion > 0 AND dias_duracion <= 3650),

    -- Con cuánta antelación avisar. 0 es válido y significa "avisame recién
    -- el día que vence".
    dias_aviso         INTEGER NOT NULL DEFAULT 1
                       CHECK (dias_aviso >= 0 AND dias_aviso <= 365),

    -- DATE y no TIMESTAMPTZ: una licencia vence un día, no un instante. Es
    -- el mismo criterio que reserva.fecha (ver 003) — "vence el 3/9"
    -- significa lo mismo sin importar la zona desde la que se mire.
    fecha_vencimiento  DATE,

    -- Cuándo se renovó, cuando se sabe. Queda NULL si el Admin cargó la
    -- licencia por los días que le quedaban ("quedan 12"): deducirlo como
    -- vencimiento - dias_duracion sería inventar un dato apoyado en otro
    -- que todavía no está confirmado.
    ultima_renovacion  DATE,

    -- Quién y cuándo escribió el vencimiento en el sistema. NO es lo mismo
    -- que ultima_renovacion, y esa diferencia es justamente el caso que
    -- motivó todo esto: "la renové el martes" (ultima_renovacion) "y lo
    -- cargué recién el jueves" (vencimiento_fijado_en). Sirve para que el
    -- segundo Admin sepa a quién preguntarle sin ir al audit_log.
    --
    -- ON DELETE SET NULL, no CASCADE ni nada: sin política de borrado, dar
    -- de baja definitivamente a un usuario muere con un 500 arrastrado por
    -- esta FK. Ya pasó con regla_recurrencia.creado_por.
    vencimiento_fijado_por  UUID REFERENCES usuario(id) ON DELETE SET NULL,
    vencimiento_fijado_en   TIMESTAMPTZ,

    -- ── Las dos marcas de aviso ─────────────────────────────────────
    -- Guardan LA FECHA DE VENCIMIENTO para la que ya salió cada aviso, no
    -- un booleano. Eso es lo que hace al job idempotente sin tener que
    -- acordarse de resetear nada: al renovar cambia fecha_vencimiento, las
    -- dos marcas dejan de coincidir solas, y el ciclo nuevo vuelve a
    -- avisar. Reiniciar el contenedor diez veces en un día manda un mail.
    avisado_previo_para       DATE,
    avisado_vencimiento_para  DATE,

    creada_en          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Unicidad por PC ignorando mayúsculas: "AutoCAD 2027" y "autocad 2027" en
-- la misma máquina son la misma licencia cargada dos veces, con dos
-- contadores que se van a contradecir. Índice funcional y no una columna
-- normalizada como en email (004) porque acá el nombre se MUESTRA: pasarlo
-- todo a minúsculas dejaría "autocad 2027" en la pantalla y en los mails.
CREATE UNIQUE INDEX ux_licencia_pc_nombre
    ON licencia_software (pc_id, lower(nombre));

-- El acceso del job: "las que vencen dentro de los próximos N días". Las
-- filas sin fecha no le sirven —no hay nada que comparar— así que quedan
-- fuera del índice.
CREATE INDEX idx_licencia_vencimiento
    ON licencia_software (fecha_vencimiento)
    WHERE fecha_vencimiento IS NOT NULL;

COMMENT ON COLUMN licencia_software.fecha_vencimiento IS
    'Día en que vence. NULL = a verificar (todavía no se miró la máquina), '
    'no "no vence nunca": el job ignora estas filas.';

-- ══════════════════════════════════════════════════════════════════
-- El tipo de notificación
-- ══════════════════════════════════════════════════════════════════
-- Igual que en la 006: el tipo existe para que la pantalla ofrezca el botón
-- que corresponde sin leer el texto del mensaje.
ALTER TABLE notificacion
    DROP CONSTRAINT IF EXISTS chk_notificacion_tipo;

ALTER TABLE notificacion
    ADD CONSTRAINT chk_notificacion_tipo
    CHECK (tipo IN ('GENERAL', 'DOCENTE_PENDIENTE', 'RESERVA_CANCELADA', 'LICENCIA_POR_VENCER'));

COMMIT;
