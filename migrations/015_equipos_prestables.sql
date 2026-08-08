-- SGRC — No todo lo que se presta es una computadora de un carro
--
-- La escuela presta dos cargadores, dos notebooks de otro modelo y un
-- proyector. De todo eso, solo el proyector podría llegar a reservarse; el
-- resto sale siempre de forma espontánea. Y la lista es distinta en cada
-- escuela: otra tiene proyector pero quizá ni cargadores ni notebooks
-- sueltas.
--
-- ══════════════════════════════════════════════════════════════════
-- Por qué acá y no en una tabla nueva
-- ══════════════════════════════════════════════════════════════════
-- La tentación es crear `articulo` al lado de `pc` y que `prestamo` apunte a
-- una o a otra. Se ve más prolijo y es peor, porque rompe lo único que hace
-- funcionar al mostrador: **"qué hay afuera del laboratorio" tiene que ser
-- UNA sola lista**.
--
-- Con dos tablas, `prestamo` necesita dos FK opcionales y un CHECK; el índice
-- de "un préstamo abierto por cosa" se duplica; la pantalla de entregas pasa
-- a mezclar dos consultas; el panel del Admin también; y el barrido también.
-- Esa complejidad se derrama por todo lo que ya está hecho.
--
-- Generalizando esta tabla, en cambio, el proyector queda reservable,
-- prestable, reclamable y liberable **sin una línea de código nueva** en
-- reservas, préstamos, incidencias, calendario ni reportes: todos ya cuelgan
-- de acá.
--
-- ══════════════════════════════════════════════════════════════════
-- La tabla sigue llamándose `pc`, y es una deuda
-- ══════════════════════════════════════════════════════════════════
-- Un proyector en una tabla llamada `pc` es una mentira, la segunda del
-- modelo después de `carro` —que en realidad es un laboratorio fijo, ver
-- docs/01-requisitos.md—. Se decidió a propósito: renombrar a `equipo` toca
-- 419 sitios en 93 archivos, y mezclar ese renombre con este cambio de
-- comportamiento daría un diff imposible de revisar. Va en su propio commit,
-- después.

BEGIN;

-- ══════════════════════════════════════════════════════════════════
-- 1. Lo que deja de ser obligatorio
-- ══════════════════════════════════════════════════════════════════
-- Un proyector no está en ningún carro. Un cargador no tiene "PC 3" ni
-- número de serie de fábrica. Los tres campos eran NOT NULL porque hasta hoy
-- todo lo que había acá era una computadora de un carro.
ALTER TABLE pc
    ALTER COLUMN carro_id      DROP NOT NULL,
    ALTER COLUMN identificador DROP NOT NULL,
    ALTER COLUMN numero_serie  DROP NOT NULL;

-- ══════════════════════════════════════════════════════════════════
-- 2. Lo que hace falta para nombrarlos
-- ══════════════════════════════════════════════════════════════════

-- El tipo es texto libre y no un enum: "podría ser cualquier cosa", y otra
-- escuela tiene un proyector pero no cargadores. Con una lista cerrada,
-- agregar "impresora 3D" pediría una migración y un despliegue. El
-- formulario ofrece los tipos que ya existen, igual que con los nombres de
-- las licencias (012).
ALTER TABLE pc
    ADD COLUMN IF NOT EXISTS tipo VARCHAR(50) NOT NULL DEFAULT 'PC'
        CHECK (tipo <> '' AND tipo = btrim(tipo));

-- El nombre es cómo se lo llama cuando no tiene número: "Proyector Epson",
-- "Cargador 1". Nulo en las PCs de carro, que se llaman por su identificador.
ALTER TABLE pc
    ADD COLUMN IF NOT EXISTS nombre VARCHAR(100)
        CHECK (nombre IS NULL OR (nombre <> '' AND nombre = btrim(nombre)));

-- reservable separa el proyector de los cargadores. Sin esto, los dos
-- cargadores aparecerían en la lista de máquinas libres cada vez que un
-- docente va a reservar: ruido, y la primera vez que alguien reserve un
-- cargador sin querer hay que explicarlo.
--
-- DEFAULT true porque todo lo que ya existe son PCs de carro, que sí se
-- reservan. Lo que se cargue después elige.
ALTER TABLE pc
    ADD COLUMN IF NOT EXISTS reservable BOOLEAN NOT NULL DEFAULT true;

-- ══════════════════════════════════════════════════════════════════
-- 3. Que nada quede sin forma de nombrarse
-- ══════════════════════════════════════════════════════════════════
-- La regla: o está en un carro y tiene número, o no está en un carro y tiene
-- nombre. Sin esto, aflojar los tres NOT NULL de arriba deja la puerta
-- abierta a una fila sin carro, sin número y sin nombre — una cosa que
-- existe en la base y que nadie puede señalar.
ALTER TABLE pc
    ADD CONSTRAINT chk_pc_identificable CHECK (
        (carro_id IS NOT NULL AND identificador IS NOT NULL)
        OR
        (carro_id IS NULL AND nombre IS NOT NULL)
    );

-- Entre los equipos sueltos, el nombre es lo único que los distingue: dos
-- filas llamadas "Cargador" serían indistinguibles en la lista de entregas,
-- que es justo donde hay que elegir cuál se está prestando. Ignora
-- mayúsculas por lo mismo que el nombre de una licencia (012).
--
-- Parcial por dos motivos. Las PCs de un carro no tienen nombre y siguen
-- distinguiéndose por UNIQUE (carro_id, identificador), que la 001 ya
-- garantiza. Y las dadas de baja quedan afuera: a diferencia de un número de
-- serie, que es único de fábrica y no se reusa nunca, "Cargador 1" es un
-- apodo — si el cargador se rompe y compran otro, lo van a seguir llamando
-- igual, y sin esta excepción el alta fallaría con un 409 sin salida.
CREATE UNIQUE INDEX IF NOT EXISTS ux_equipo_suelto_nombre
    ON pc (lower(nombre)) WHERE carro_id IS NULL AND dada_de_baja = false;

-- El acceso nuevo: "qué equipos hay que no estén en ningún carro". Sin
-- filtrar por dada_de_baja, porque el listado tampoco filtra —las trae todas
-- y la pantalla decide, igual que el de las PCs de un carro—.
CREATE INDEX IF NOT EXISTS idx_pc_sueltos
    ON pc (tipo, nombre) WHERE carro_id IS NULL;

-- ══════════════════════════════════════════════════════════════════
-- 4. Cómo se llamaba el equipo, congelado en el histórico
-- ══════════════════════════════════════════════════════════════════
-- historico_uso_pc congela identificador y carro al archivar el ciclo, para
-- poder decir "PC 3 (Carro 1)" años después aunque la máquina ya no exista.
-- Un proyector no tiene ninguna de las dos cosas: el reporte del año pasado
-- diría "PC 0 ()".
--
-- La columna guarda la etiqueta ya armada y no el nombre suelto porque eso
-- es lo que se muestra, y porque congelarla es justamente el punto: si
-- mañana renombran el proyector, el reporte de 2026 tiene que seguir
-- diciendo cómo se lo llamaba en 2026.
ALTER TABLE historico_uso_pc
    ADD COLUMN IF NOT EXISTS etiqueta_snapshot VARCHAR(100);

-- Backfill: hasta hoy todo lo archivado era una PC de carro, así que su
-- etiqueta se reconstruye exactamente desde lo que ya está guardado.
UPDATE historico_uso_pc
   SET etiqueta_snapshot = 'PC ' || identificador_snapshot
 WHERE etiqueta_snapshot IS NULL;

ALTER TABLE historico_uso_pc
    ALTER COLUMN etiqueta_snapshot SET NOT NULL;

-- Las dos viejas dejan de ser obligatorias: un equipo suelto no las tiene y
-- la etiqueta ya cubre lo que se muestra. Se conservan porque el reporte
-- sigue ordenando y agrupando por carro.
ALTER TABLE historico_uso_pc
    ALTER COLUMN identificador_snapshot DROP NOT NULL,
    ALTER COLUMN carro_nombre_snapshot  DROP NOT NULL;

COMMENT ON TABLE pc IS
    'Todo lo que la escuela presta: computadoras de un carro y también '
    'proyectores, cargadores o lo que sea. Se llama `pc` por historia, no '
    'porque todo lo de acá lo sea (ver 015).';

COMMENT ON COLUMN pc.reservable IS
    'Si aparece en la lista de equipos libres al reservar. Un proyector sí, '
    'un cargador no: se presta en el momento y nadie planifica con él.';

COMMENT ON COLUMN pc.nombre IS
    'Cómo se lo llama cuando no tiene número de carro. NULL en las PCs, que '
    'se nombran por su identificador.';

COMMIT;
