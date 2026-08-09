-- SGRC — Un bloqueo no es siempre una evaluación
--
-- `reserva.tipo` distinguía dos cosas: 'NORMAL' (un docente reservó para su
-- clase) y 'EVALUACION_ESTATAL' (un Admin se tomó los equipos y canceló lo
-- que hubiera encima). La segunda estaba mal nombrada.
--
-- Un Admin toma los equipos por muchos motivos: una evaluación, una jornada
-- docente, una capacitación, una obra en el aula, un acto. Lo que tienen en
-- común no es la evaluación: es que **alguien con autoridad decidió que ese
-- rato el laboratorio se usa para otra cosa**, y que las clases que había
-- encima se cancelan. Eso es lo que el tipo tiene que nombrar.
--
-- El nombre viejo no era solo cosmético. Estaba en el `CHECK` de la tabla, en
-- los mensajes que le llegan al docente cancelado, en el título de la
-- pantalla y en el rótulo con el que esas filas aparecen en el calendario y
-- en el mostrador. Una institución que bloquea por una jornada docente leía
-- "evaluación estatal" en todos lados, incluido el aviso que le llega al
-- docente al que le cancelaron la clase.
--
-- ══════════════════════════════════════════════════════════════════
-- El motivo pasa a guardarse en el bloqueo
-- ══════════════════════════════════════════════════════════════════
-- El motivo ya se pedía al crear el bloqueo, pero se usaba SOLO para armar el
-- texto de las cancelaciones que disparaba, y no se guardaba en ningún lado.
-- Dos consecuencias:
--
--   1. Un bloqueo que no pisó ninguna reserva —el caso más común, porque se
--      suele avisar con tiempo— no dejaba rastro de por qué existe. Quedaba
--      un rato ocupado sin explicación, y el único lugar donde mirarlo era
--      la auditoría.
--   2. Al archivar el ciclo lectivo el motivo se perdía del todo, porque
--      vivía en la reserva cancelada y esa se borra.
--
-- Ahora vive en la fila del bloqueo. Es texto libre y no una lista cerrada,
-- por el mismo criterio que el tipo de equipo (015) y la categoría de falla
-- (017): la razón por la que una institución se toma el laboratorio no la
-- puede prever el sistema.
--
-- Es OBLIGATORIO para los bloqueos, y ahí se aparta de esos dos casos. La
-- diferencia es a quién le cuesta: una falla sin clasificar es trabajo
-- pendiente de quien la reporta, mientras que un bloqueo sin motivo le
-- cancela la clase a otra persona. Quien tiene la autoridad para hacer eso
-- puede escribir por qué.
--
-- Para las reservas normales queda en NULL, y el CHECK lo exige en ese
-- sentido: la reserva de un docente ya dice para qué es —su materia—, así
-- que un motivo ahí sería un segundo lugar donde escribir lo mismo.
--
-- ══════════════════════════════════════════════════════════════════
-- Qué pasa con los bloqueos que ya existen
-- ══════════════════════════════════════════════════════════════════
-- Se les pone 'Evaluación estatal' como motivo. No es un relleno: es
-- exactamente lo que eran cuando se crearon, porque el sistema no ofrecía
-- otra cosa. Inventarles un motivo genérico ("Bloqueo") sería peor — diría
-- menos de lo que se sabe.
--
-- El valor guardado en `auditoria.accion` (`BLOQUEO_EVALUACION_CREADO`) NO se
-- reescribe, por el mismo motivo que en la 016: un registro de auditoría dice
-- qué pasó y con qué nombre se llamaba entonces, y reescribirlo es
-- precisamente lo que un registro de auditoría no debe permitir.

BEGIN;

DO $$
BEGIN
IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'reserva' AND column_name = 'motivo_bloqueo'
) THEN

    -- 1. La columna, todavía sin exigir nada: las filas viejas no la tienen.
    ALTER TABLE reserva ADD COLUMN motivo_bloqueo TEXT;

    -- 2. Los CHECK viejos nombran el valor que está por dejar de existir, así
    --    que salen antes del UPDATE. Con ellos puestos, renombrar el tipo
    --    fallaría fila por fila.
    ALTER TABLE reserva DROP CONSTRAINT IF EXISTS reserva_tipo_check;
    ALTER TABLE reserva DROP CONSTRAINT IF EXISTS chk_reserva_tipo_coherente;

    -- 3. El renombre y el motivo de lo que ya existía, en la misma pasada.
    UPDATE reserva
       SET tipo = 'BLOQUEO',
           motivo_bloqueo = 'Evaluación estatal'
     WHERE tipo = 'EVALUACION_ESTATAL';

    -- 4. Los CHECK nuevos. El de coherencia gana una tercera condición: un
    --    bloqueo sin motivo deja de ser representable, que es lo que hace que
    --    la regla valga también para lo que escriba otra aplicación contra
    --    esta base.
    ALTER TABLE reserva ADD CONSTRAINT reserva_tipo_check
        CHECK (tipo IN ('NORMAL', 'BLOQUEO'));

    ALTER TABLE reserva ADD CONSTRAINT chk_reserva_tipo_coherente CHECK (
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
    );

    ALTER TABLE reserva ALTER COLUMN tipo SET DEFAULT 'NORMAL';

    COMMENT ON COLUMN reserva.tipo IS
        'NORMAL: la reserva de un docente para su clase. BLOQUEO: un Admin se '
        'tomó el equipo para otra cosa y canceló lo que hubiera encima.';

    COMMENT ON COLUMN reserva.motivo_bloqueo IS
        'Por qué se tomó el equipo, en texto libre: una evaluación, una '
        'jornada docente, una obra en el aula. Obligatorio en los BLOQUEO '
        'porque cancelan clases ajenas; NULL en las reservas normales, que '
        'ya dicen para qué son por su materia.';

ELSE
    RAISE NOTICE 'La 019 ya estaba aplicada: reserva.motivo_bloqueo ya existe.';
END IF;
END $$;

COMMIT;
