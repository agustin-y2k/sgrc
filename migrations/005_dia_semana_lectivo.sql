-- SGRC — La semana lectiva es de lunes a viernes, también en la base
--
-- `dia_semana` es VARCHAR(10) sin ninguna restricción en las dos tablas que
-- la usan (horario_admin y regla_recurrencia): cualquier texto entra. La
-- regla de negocio existía solo en el código, y ni siquiera de forma
-- pareja — reservation ya había sacado SABADO de su enum (migración 002),
-- pero availability lo seguía aceptando, así que `POST /mi-horario` con
-- `"diaSemana":"SABADO"` creaba una fila que el frontend después tenía que
-- saber esquivar.
--
-- El sábado y el domingo no son días sobre los que la escuela opere: no hay
-- clase, así que ni se reserva ni tiene sentido publicar un horario de
-- presencia. Ver docs/01-requisitos.md.

BEGIN;

-- ══════════════════════════════════════════════════════════════════
-- 1. Cortar temprano si hay filas que la regla dejaría afuera
-- ══════════════════════════════════════════════════════════════════
-- Mismo criterio que la migración 004 con los emails duplicados: qué hacer
-- con un bloque cargado en sábado —borrarlo o moverlo a otro día— es una
-- decisión de quien lo cargó, no de una migración. Se corta acá, diciendo
-- exactamente qué filas son, en vez de fallar más abajo con un error de
-- constraint que no explica nada.
--
-- Sobre una base creada desde el frontend esto no encuentra nada: el
-- formulario solo ofrece lunes a viernes.
DO $$
DECLARE
    dias_lectivos CONSTANT text[] := ARRAY['LUNES', 'MARTES', 'MIERCOLES', 'JUEVES', 'VIERNES'];
    fuera text;
BEGIN
    SELECT string_agg(format('horario_admin %s (%s, %s a %s)', id, dia_semana, hora_inicio, hora_fin), '; ')
      INTO fuera
      FROM horario_admin
     WHERE dia_semana <> ALL (dias_lectivos);

    IF fuera IS NOT NULL THEN
        RAISE EXCEPTION
            'Hay bloques de horario fuera de la semana lectiva: %. Decidí si se borran o se mueven a un día hábil antes de aplicar esta migración (DELETE FROM horario_admin WHERE id = ... / UPDATE ... SET dia_semana = ...).',
            fuera;
    END IF;

    SELECT string_agg(format('regla_recurrencia %s (%s)', id, dia_semana), '; ')
      INTO fuera
      FROM regla_recurrencia
     WHERE dia_semana <> ALL (dias_lectivos);

    IF fuera IS NOT NULL THEN
        RAISE EXCEPTION
            'Hay reglas de recurrencia fuera de la semana lectiva: %. La migración 002 ya borró las de sábado; si quedó alguna, revisala a mano antes de aplicar esta.',
            fuera;
    END IF;
END $$;

-- ══════════════════════════════════════════════════════════════════
-- 2. Que la regla la sostenga la base
-- ══════════════════════════════════════════════════════════════════
-- Con el CHECK puesto, un INSERT con SABADO falla en la base aunque el
-- código de la aplicación se lo saltee. Es la misma idea que el UNIQUE de
-- email o la constraint EXCLUDE de anti-solapamiento: la regla que importa
-- no depende de que todos los caminos del código se acuerden de aplicarla.
ALTER TABLE horario_admin
    ADD CONSTRAINT chk_horario_admin_dia_lectivo
    CHECK (dia_semana IN ('LUNES', 'MARTES', 'MIERCOLES', 'JUEVES', 'VIERNES'));

ALTER TABLE regla_recurrencia
    ADD CONSTRAINT chk_regla_recurrencia_dia_lectivo
    CHECK (dia_semana IN ('LUNES', 'MARTES', 'MIERCOLES', 'JUEVES', 'VIERNES'));

COMMIT;
