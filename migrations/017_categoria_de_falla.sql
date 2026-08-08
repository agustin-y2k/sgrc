-- SGRC — Poder contar qué se rompe, no solo cuánto se rompe
--
-- Hasta acá una incidencia guardaba una descripción en texto libre y una
-- gravedad. Alcanza para saber CUÁNTAS fallas tiene una máquina, pero no QUÉ
-- le pasa: responder "cuántas están rotas de batería" obligaba a leer las
-- descripciones de a una, y descripciones escritas por personas distintas
-- ("no carga", "batería hinchada", "se apaga sin el cargador") no se agrupan
-- solas.
--
-- Con muchas máquinas fuera de circulación eso deja de ser un detalle: es la
-- diferencia entre poder pedir presupuesto por diez baterías y tener que
-- revisar el inventario a mano.
--
-- ══════════════════════════════════════════════════════════════════
-- Texto libre y no una lista cerrada
-- ══════════════════════════════════════════════════════════════════
-- La tentación es un enum: batería, pantalla, teclado, otro. Daría una
-- estadística perfecta y es lo que se suele hacer.
--
-- Se eligió texto libre igual, por el mismo motivo que el tipo de equipo
-- (015): cada institución rompe cosas distintas y arregla cosas distintas.
-- Con una lista cerrada, la primera falla que no esté prevista —una bisagra,
-- una fuente, un teclado en un idioma que nadie puede reemplazar— pide una
-- migración y un despliegue para poder anotarse.
--
-- El costo conocido es que "Batería" y "batería" son dos cadenas distintas.
-- Se ataca por dos lados, y ninguno de los dos le quita libertad a quien
-- escribe:
--
--   1. El formulario sugiere las categorías ya usadas (mismo mecanismo que
--      el tipo de equipo y el nombre de una licencia). Es lo que hace que
--      converjan solas sin obligar a nadie.
--   2. Los reportes agrupan por lower(categoria), así que la diferencia de
--      caja no fragmenta la cuenta.
--
-- Lo que NO se resuelve, y conviene saberlo: los acentos. "bateria" y
-- "batería" van a contar separado. Normalizarlos pediría la extensión
-- `unaccent` en la base o reescribir lo que la persona tipeó, y ninguna de
-- las dos parece valer la pena frente a una sugerencia que aparece sola al
-- segundo caracter.
--
-- ══════════════════════════════════════════════════════════════════
-- Por qué es NULL-able
-- ══════════════════════════════════════════════════════════════════
-- Las incidencias ya cargadas no tienen categoría y no hay forma honesta de
-- adivinarla: inventar una las mezclaría con las que sí se clasificaron.
--
-- Pero además NULL es un estado legítimo hacia adelante, y es el que el
-- problema real pide: una máquina que no enciende y que nadie pudo
-- diagnosticar todavía tiene una falla perfectamente real y ninguna
-- categoría. Obligar a completarla llevaría a que alguien escriba "otro" o
-- "no sé", que es peor que el vacío: parece un diagnóstico y no lo es.
-- En los reportes, esas filas se cuentan aparte como "sin clasificar".

BEGIN;

ALTER TABLE incidencia
    ADD COLUMN IF NOT EXISTS categoria VARCHAR(50)
        CHECK (categoria IS NULL OR (categoria <> '' AND categoria = btrim(categoria)));

COMMENT ON COLUMN incidencia.categoria IS
    'Qué tipo de falla es, en texto libre normalizado (ej: batería, pantalla). '
    'NULL cuando todavía no se pudo diagnosticar, que es un estado real y no '
    'un dato faltante. Los reportes agrupan por lower(categoria).';

-- El acceso que habilita el reporte por categoría: agrupar sin distinguir
-- caja. Parcial porque las no clasificadas se cuentan por separado y no
-- entran en ese agrupamiento.
CREATE INDEX IF NOT EXISTS idx_incidencia_categoria
    ON incidencia (lower(categoria)) WHERE categoria IS NOT NULL;

-- El reporte de equipos fuera de circulación busca la incidencia más
-- reciente de cada máquina. Sin esto es un recorrido de la tabla entera por
-- cada equipo listado.
CREATE INDEX IF NOT EXISTS idx_incidencia_equipo_fecha
    ON incidencia (equipo_id, fecha DESC);

COMMIT;
