-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — La división de un curso la escribe la institución
-- ═══════════════════════════════════════════════════════════════════════
--
-- El nombre de un curso venía cerrado en `^[1-6]°[A-Z]$`: año del 1 al 6 y
-- división de una sola letra. Eso da "1°A", que es como nombran sus cursos
-- muchas escuelas y no todas. Hay instituciones cuya división también es un
-- número —"1°4°", "2°1°"— y otras que usan "1ra", "B bis" o un turno. Con el
-- patrón viejo esos cursos NO SE PUEDEN CARGAR: no hay forma de escribirlos, y
-- tampoco un rodeo, porque el nombre es la identidad del curso en todo el
-- sistema.
--
-- Lo que se abre es SOLO LA DIVISIÓN. El año sigue siendo del 1 al 6 y sigue
-- ocupando el primer carácter, y esa es la razón: el ordenamiento de equipos
-- por preferencia de materia (RF-03.21) acota su alcance a un año, o a un año
-- y una división, y saca las dos partes del nombre del curso. Abrir el nombre
-- entero dejaría a esas marcas sin de dónde leer el año.
--
-- ── El UNIQUE pasa a ser insensible a mayúsculas ───────────────────────
--
-- Antes el patrón obligaba a mayúsculas, así que "1°a" ni siquiera era un
-- nombre válido y `UNIQUE (ciclo, nombre)` alcanzaba. Ahora que la división es
-- texto libre se guarda como la escribieron —"1ra" no debería quedar "1RA"— y
-- sin esto "3°B" y "3°b" serían dos cursos distintos en el mismo ciclo, cada
-- uno con sus materias y sus reservas. El índice funcional sobre `upper(nombre)`
-- conserva la garantía vieja sin imponer la capitalización.
--
-- ── Qué pasa con los cursos ya cargados ────────────────────────────────
--
-- Nada: todo lo que cumplía `^[1-6]°[A-Z]$` cumple el patrón nuevo, que es más
-- ancho. Esta migración no reescribe ninguna fila.

-- +goose Up

-- El nombre entra en VARCHAR(4) sólo mientras la división es un carácter.
ALTER TABLE curso ALTER COLUMN nombre TYPE VARCHAR(16);

ALTER TABLE curso DROP CONSTRAINT curso_nombre_check;

-- `\S` y no `.`: la división puede tener espacios adentro ("B bis") pero no
-- empezar con uno, y `btrim` cierra el otro extremo. Sin las dos, "1° " y
-- "1°  A" serían nombres distintos del mismo curso.
ALTER TABLE curso ADD CONSTRAINT curso_nombre_check
    CHECK (nombre ~ '^[1-6]°\S' AND nombre = btrim(nombre));

ALTER TABLE curso DROP CONSTRAINT curso_ciclo_lectivo_id_nombre_key;

CREATE UNIQUE INDEX ux_curso_ciclo_nombre ON curso (ciclo_lectivo_id, upper(nombre));

-- La división de una marca de preferencia sale del mismo lugar y tiene que
-- poder guardar lo mismo: si existe el curso "1°4°", tiene que poder existir
-- la marca "preferente para Matemática de 1°4°".
ALTER TABLE equipo_preferencia ALTER COLUMN division TYPE VARCHAR(12);

ALTER TABLE equipo_preferencia DROP CONSTRAINT equipo_preferencia_division_check;

ALTER TABLE equipo_preferencia ADD CONSTRAINT equipo_preferencia_division_check
    CHECK (division IS NULL OR (division <> '' AND division = btrim(division)));

COMMENT ON COLUMN curso.nombre IS
    'Año (1 a 6) más división, ej. "1°A", "1°4°", "3°B bis". El año es fijo '
    'porque las marcas de preferencia de equipo lo leen del primer carácter; '
    'la división la escribe la institución. Único por ciclo sin distinguir '
    'mayúsculas.';

-- +goose Down

-- Ojo: esta vuelta atrás puede fallar, y es correcto que falle. Si mientras la
-- 007 estuvo aplicada se cargó un curso cuya división no es una sola letra de
-- la A a la Z —que es justamente para lo que se hizo—, esa fila no cumple el
-- CHECK viejo y no hay forma automática de decidir qué letra le corresponde.
-- Hay que renombrar esos cursos a mano antes de bajar de versión.

DROP INDEX ux_curso_ciclo_nombre;

ALTER TABLE curso ADD CONSTRAINT curso_ciclo_lectivo_id_nombre_key UNIQUE (ciclo_lectivo_id, nombre);

ALTER TABLE curso DROP CONSTRAINT curso_nombre_check;

ALTER TABLE curso ADD CONSTRAINT curso_nombre_check CHECK (nombre ~ '^[1-6]°[A-Z]$');

ALTER TABLE curso ALTER COLUMN nombre TYPE VARCHAR(4);

ALTER TABLE equipo_preferencia DROP CONSTRAINT equipo_preferencia_division_check;

ALTER TABLE equipo_preferencia ADD CONSTRAINT equipo_preferencia_division_check
    CHECK (division ~ '^[A-Z]$');

ALTER TABLE equipo_preferencia ALTER COLUMN division TYPE CHAR(1);
