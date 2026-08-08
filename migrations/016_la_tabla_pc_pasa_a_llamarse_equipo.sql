-- SGRC — La tabla `pc` pasa a llamarse `equipo`
--
-- Es una deuda que se contrajo a propósito en la 015 y que se paga acá, en su
-- propio archivo. Desde aquella migración esta tabla también guarda lo que se
-- presta y no es una computadora de un carro —proyectores, cargadores,
-- notebooks sueltas—, así que llamarla `pc` era una mentira.
--
-- Se separó del cambio de comportamiento porque mezclarlos daba un diff
-- imposible de revisar: allá había reglas nuevas que discutir, acá no hay
-- ninguna. **Esta migración no cambia ni un dato ni una regla.** Todo lo que
-- hace es cambiar nombres, y por eso se puede leer de corrido y verificar
-- comparando el esquema de antes y el de después.
--
-- ══════════════════════════════════════════════════════════════════
-- Por qué también los índices y las constraints
-- ══════════════════════════════════════════════════════════════════
-- Postgres NO los renombra solos: después de un RENAME TO la tabla `equipo`
-- se queda con `pc_pkey`, `pc_estado_check` y compañía. Funciona igual, pero
-- deja el peor resultado posible — un esquema a medio renombrar, donde el
-- nombre viejo sobrevive justo en los mensajes de error, que es donde alguien
-- lo va a leer sin contexto: "duplicate key value violates unique constraint
-- pc_numero_serie_key" sobre una tabla que ya no se llama así.
--
-- Renombrar una constraint UNIQUE o PRIMARY KEY renombra también el índice
-- que la respalda, así que esos no aparecen dos veces acá.
--
-- ══════════════════════════════════════════════════════════════════
-- Qué NO se toca
-- ══════════════════════════════════════════════════════════════════
-- `carro` se queda como está, y no por deuda: es el nombre correcto. Un carro
-- es un mueble metálico con ruedas y zócalos numerados donde las
-- notebooks se guardan y se cargan. El `identificador` de cada equipo es
-- justamente el número de su zócalo, y por eso se repite entre carros y es
-- único dentro de uno solo.
--
-- Las migraciones anteriores tampoco se tocan: son el registro de lo que
-- pasó, y reescribirlas para que digan `equipo` haría que una base creada
-- desde cero y una migrada terminaran iguales por caminos que ya no se
-- pueden comparar con lo que quedó escrito.
--
-- Tampoco se tocan los VALORES ya guardados que dicen PC: las acciones de
-- auditoría (`PC_ESTADO_CAMBIADO`, `PC_DADA_DE_BAJA`, `PC_MOVIDA_DE_CARRO`) y
-- el tipo de notificación `PC_SIN_DEVOLVER`. Esos son datos, no esquema:
-- están escritos en filas que registran cosas que pasaron cuando la entidad
-- se llamaba así. Reescribir un log de auditoría para que diga otra cosa es
-- precisamente lo que un log de auditoría no debe permitir. En el código, los
-- identificadores de Go sí se renombraron; lo que queda igual es la cadena.

--
-- ══════════════════════════════════════════════════════════════════
-- Por qué todo esto está adentro de un DO
-- ══════════════════════════════════════════════════════════════════
-- Para que correrla dos veces no duela. Sin el IF de abajo, el segundo
-- intento muere en la primera línea con `relation "pc" does not exist`: un
-- mensaje que no menciona la migración, no dice que ya se aplicó y parece
-- mucho más grave de lo que es. Pasó en el despliegue real.
--
-- La comprobación es UNA sola, sobre la existencia de `pc`, y no un
-- `IF EXISTS` por sentencia: ese modificador existe para tablas e índices
-- pero NO para renombrar columnas ni constraints, así que la mitad de las
-- líneas fallaría igual. Que la tabla siga llamándose `pc` es exactamente
-- la pregunta "¿esta migración ya corrió?", y responde por las 34.

BEGIN;

DO $$
BEGIN
IF NOT EXISTS (
    SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'pc'
) THEN
    -- Se distinguen los dos motivos por los que `pc` puede no estar. Sin
    -- esto, correr la 016 sobre una base sin esquema decía "ya estaba
    -- aplicada", que es exactamente lo contrario de lo que pasa.
    IF EXISTS (
        SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'equipo'
    ) THEN
        RAISE NOTICE 'La 016 ya estaba aplicada: la tabla ya se llama equipo. No hay nada que hacer.';
        RETURN;
    END IF;
    RAISE EXCEPTION 'No existe ni `pc` ni `equipo`: esta base no tiene las migraciones anteriores aplicadas.';
END IF;


    -- ══════════════════════════════════════════════════════════════════
    -- 1. Las tablas
    -- ══════════════════════════════════════════════════════════════════
    ALTER TABLE pc                RENAME TO equipo;
    ALTER TABLE historico_uso_pc  RENAME TO historico_uso_equipo;

    -- ══════════════════════════════════════════════════════════════════
    -- 2. La columna que la referencia, en las cinco tablas que la usan
    -- ══════════════════════════════════════════════════════════════════
    ALTER TABLE reserva              RENAME COLUMN pc_id TO equipo_id;
    ALTER TABLE prestamo             RENAME COLUMN pc_id TO equipo_id;
    ALTER TABLE incidencia           RENAME COLUMN pc_id TO equipo_id;
    ALTER TABLE licencia_software    RENAME COLUMN pc_id TO equipo_id;
    ALTER TABLE historico_uso_equipo RENAME COLUMN pc_id TO equipo_id;

    -- El aviso de "la máquina de tu reserva no volvió y no está disponible"
    -- (014). Nombra a la cosa que falta, no a la clase de cosa que era.
    ALTER TABLE reserva RENAME COLUMN avisado_pc_no_disponible_en TO avisado_equipo_no_disponible_en;

    -- Concordancia: era "la PC dada de baja" y ahora es "el equipo dado de
    -- baja". Podría dejarse en femenino y nadie se rompería, pero el modelo está
    -- escrito en castellano correcto de punta a punta y `equipo.dada_de_baja` se
    -- lee como un error de tipeo cada vez que alguien abre la tabla.
    ALTER TABLE equipo RENAME COLUMN dada_de_baja TO dado_de_baja;

    -- ══════════════════════════════════════════════════════════════════
    -- 3. Las constraints de la tabla renombrada
    -- ══════════════════════════════════════════════════════════════════
    ALTER TABLE equipo RENAME CONSTRAINT pc_pkey                       TO equipo_pkey;
    ALTER TABLE equipo RENAME CONSTRAINT pc_carro_id_fkey              TO equipo_carro_id_fkey;
    ALTER TABLE equipo RENAME CONSTRAINT pc_carro_id_identificador_key TO equipo_carro_id_identificador_key;
    ALTER TABLE equipo RENAME CONSTRAINT pc_numero_serie_key           TO equipo_numero_serie_key;
    ALTER TABLE equipo RENAME CONSTRAINT pc_numero_serie_no_vacio      TO equipo_numero_serie_no_vacio;
    ALTER TABLE equipo RENAME CONSTRAINT pc_estado_check               TO equipo_estado_check;
    ALTER TABLE equipo RENAME CONSTRAINT pc_tipo_check                 TO equipo_tipo_check;
    ALTER TABLE equipo RENAME CONSTRAINT pc_nombre_check               TO equipo_nombre_check;
    ALTER TABLE equipo RENAME CONSTRAINT chk_pc_identificable          TO chk_equipo_identificable;

    -- ══════════════════════════════════════════════════════════════════
    -- 4. Las constraints que la nombran desde otras tablas
    -- ══════════════════════════════════════════════════════════════════
    ALTER TABLE reserva              RENAME CONSTRAINT reserva_pc_id_fkey           TO reserva_equipo_id_fkey;
    ALTER TABLE prestamo             RENAME CONSTRAINT prestamo_pc_id_fkey          TO prestamo_equipo_id_fkey;
    ALTER TABLE incidencia           RENAME CONSTRAINT incidencia_pc_id_fkey        TO incidencia_equipo_id_fkey;
    ALTER TABLE licencia_software    RENAME CONSTRAINT licencia_software_pc_id_fkey TO licencia_software_equipo_id_fkey;
    ALTER TABLE historico_uso_equipo RENAME CONSTRAINT historico_uso_pc_pkey        TO historico_uso_equipo_pkey;
    ALTER TABLE historico_uso_equipo RENAME CONSTRAINT historico_uso_pc_pc_id_fkey  TO historico_uso_equipo_equipo_id_fkey;
    ALTER TABLE historico_uso_equipo RENAME CONSTRAINT historico_uso_pc_anio_pc_id_key TO historico_uso_equipo_anio_equipo_id_key;

    -- ══════════════════════════════════════════════════════════════════
    -- 5. Los índices
    -- ══════════════════════════════════════════════════════════════════
    ALTER INDEX idx_pc_carro_estado    RENAME TO idx_equipo_carro_estado;
    ALTER INDEX idx_pc_sueltos         RENAME TO idx_equipo_sueltos;
    ALTER INDEX idx_reserva_pc_fecha   RENAME TO idx_reserva_equipo_fecha;
    ALTER INDEX idx_prestamo_pc        RENAME TO idx_prestamo_equipo;
    ALTER INDEX idx_incidencia_pc      RENAME TO idx_incidencia_equipo;
    ALTER INDEX idx_historico_pc_anio  RENAME TO idx_historico_equipo_anio;
    ALTER INDEX ux_licencia_pc_nombre  RENAME TO ux_licencia_equipo_nombre;

    -- `no_solapamiento` y `ux_prestamo_abierto` NO se renombran: sus nombres
    -- dicen qué garantizan, no sobre qué tabla, y siguen siendo exactos. Sus
    -- definiciones se actualizan solas — Postgres guarda la referencia a la
    -- columna por identidad interna, no por nombre.

    -- ══════════════════════════════════════════════════════════════════
    -- 6. Los comentarios que explicaban el nombre viejo
    -- ══════════════════════════════════════════════════════════════════
    COMMENT ON TABLE equipo IS
        'Todo lo que la escuela presta: las computadoras de un carro y también '
        'proyectores, cargadores o notebooks sueltas. Lo que está en un carro se '
        'nombra por su identificador ("PC 3"); lo que no, por su nombre.';

    COMMENT ON COLUMN equipo.nombre IS
        'Cómo se lo llama cuando no tiene número de carro. NULL en las '
        'computadoras de un carro, que se nombran por su identificador.';

END $$;

COMMIT;
