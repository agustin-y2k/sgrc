-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Configuración de la institución
-- ═══════════════════════════════════════════════════════════════════════
--
-- Nace para responder una pregunta que hasta ahora no se podía hacer:
-- ¿la escuela YA decidió cuál es su jornada?
--
-- Hasta acá, "no hay ningún tramo cargado" significaba las dos cosas a la
-- vez: que todavía nadie la declaró, y que alguien decidió a propósito
-- dejarla sin restricción. Son estados distintos y piden cosas distintas —al
-- primero hay que preguntarle, al segundo no— pero desde la base se veían
-- iguales: una lista vacía.
--
-- El comportamiento de reservas NO cambia. Sin tramos cargados se sigue
-- pudiendo reservar cualquier día y a cualquier hora, que es lo correcto
-- para una instalación nueva: el sistema no supone un calendario que nadie
-- le dijo. Esta bandera solo decide si al Admin se le pregunta.

-- +goose Up

-- Una sola fila, porque una instalación atiende a una sola institución.
-- `unica` es la clave primaria fijada en TRUE, y el CHECK cierra la puerta:
-- una segunda fila necesitaría otro valor de la clave, y el único otro valor
-- posible —FALSE— no pasa el CHECK. Sale más barato que un trigger y no se
-- puede evadir escribiendo directo contra la base.
CREATE TABLE configuracion_institucion (
    unica            BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (unica),

    -- FALSE = todavía no lo decidieron, hay que preguntar.
    -- TRUE  = ya lo decidieron, sea declarando tramos o eligiendo dejarla
    --         libre. En los dos casos no se vuelve a preguntar.
    jornada_definida BOOLEAN NOT NULL DEFAULT FALSE
);

-- La fila se crea acá y no desde la aplicación: así el código nunca tiene que
-- contemplar el caso "todavía no existe", que es una rama que se escribe una
-- vez y se prueba nunca.
--
-- El valor NO es FALSE a secas. Una instalación que viene funcionando ya
-- declaró su jornada hace meses, y arrancarla en FALSE la mandaría a un
-- cuestionario de primer arranque para que responda algo que ya respondió.
-- Si hay tramos cargados, la decisión está tomada.
INSERT INTO configuracion_institucion (unica, jornada_definida)
VALUES (TRUE, EXISTS (SELECT 1 FROM jornada_institucion));

-- +goose Down

DROP TABLE IF EXISTS configuracion_institucion;
