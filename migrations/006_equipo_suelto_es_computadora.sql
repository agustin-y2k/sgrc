-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Distinguir una computadora de lo demás que se presta
-- ═══════════════════════════════════════════════════════════════════════
--
-- `equipo` es una sola tabla para todo lo prestable, y eso no cambia. Lo que
-- faltaba es responder una pregunta que la tabla no hacía: **si esto es una
-- computadora**. Es lo único que decide si tiene sentido preguntar CPU, RAM,
-- sistema operativo, software instalado y con qué cuenta se entra. Un
-- cargador no tiene nada de eso y el formulario no debería preguntárselo; una
-- notebook suelta lo tiene todo y hasta ahora no había dónde anotarlo.
--
-- **No reemplaza a `tipo`, que sigue siendo texto libre.** Los dos dicen cosas
-- distintas y se complementan: `es_computadora` decide QUÉ SE PREGUNTA, `tipo`
-- dice QUÉ ES ("Notebook", "Tablet", "Proyector", "Cargador"). Cerrar `tipo` en
-- una lista de dos valores sería el error opuesto, el mismo que el sistema
-- evita en la categoría de incidencia y en el tipo de cuenta: la primera cosa
-- no prevista que preste la institución pediría una migración.
--
-- Y por eso tampoco se llama "es_notebook": el día que entre una tablet o una
-- PC de escritorio que no vuelve al carro, tienen la misma ficha técnica y las
-- mismas cuentas que una notebook. Lo que las agrupa es ser computadoras.
--
-- ── Qué valor toman las filas que ya existen ───────────────────────────
--
-- Las de un carro son computadoras de laboratorio por definición: `true` sin
-- ambigüedad. Los equipos sueltos que ya están cargados quedan en `false`, y
-- quien administra marca a mano los que sean máquinas.
--
-- Deliberadamente NO se adivina por el texto de `tipo`. Es lo que escribió una
-- persona, y "Note book", "Notebook HP" o "Ultrabook" son todas la misma cosa
-- para quien la escribió y tres cadenas distintas para un LIKE. Una adivinanza
-- acá deja datos mal clasificados que nadie va a revisar, y son pocos equipos
-- sueltos: marcarlos a mano se hace una vez y queda bien.

-- +goose Up

ALTER TABLE equipo ADD COLUMN es_computadora BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN equipo.es_computadora IS
    'Si el equipo es una computadora (notebook, tablet, PC de escritorio). '
    'Decide si se le piden los datos de la máquina y si se le pueden anotar '
    'cuentas de acceso. No reemplaza a tipo, que sigue siendo texto libre.';

UPDATE equipo SET es_computadora = true WHERE carro_id IS NOT NULL;

-- +goose Down

ALTER TABLE equipo DROP COLUMN es_computadora;
