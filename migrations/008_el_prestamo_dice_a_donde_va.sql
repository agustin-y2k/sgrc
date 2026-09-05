-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Un préstamo anota a dónde va el equipo, no para qué
-- ═══════════════════════════════════════════════════════════════════════
--
-- `prestamo.motivo` nació como el "¿para qué?" de una entrega sin reserva, y
-- en el mostrador esa pregunta no sirve para nada: la respuesta es siempre
-- "para dar clase" o "para un trámite", y ninguna de las dos ayuda cuando hay
-- que ir a buscar la máquina, ni cuando a las seis de la tarde falta una y hay
-- que reconstruir por dónde anduvo.
--
-- La pregunta que sí sirve es DÓNDE ESTÁ: "1°4°", "Biblioteca", "Dirección",
-- "Sección Alumnos". Eso es lo que la columna guarda a partir de ahora.
--
-- La salida a reparación ya la usaba así —su formulario pregunta "¿a dónde va
-- y por qué?" y la aplicación la exige justamente como constancia de a dónde
-- fue el equipo—, así que el nombre nuevo no le cambia el significado a esas
-- filas: las alcanza.
--
-- ── Por qué un rename y no una columna nueva ───────────────────────────
--
-- Porque no son dos datos distintos conviviendo: es el mismo campo con la
-- pregunta corregida. Una columna nueva dejaría el historial partido en dos —lo
-- viejo en `motivo`, lo nuevo en `destino`— y obligaría a leer las dos en cada
-- pantalla para siempre. El rename conserva el contenido tal cual está: lo que
-- alguien anotó como "trámite en secretaría" sigue ahí, y sigue siendo la mejor
-- pista que hay sobre dónde estuvo esa máquina.

-- +goose Up

ALTER TABLE prestamo RENAME COLUMN motivo TO destino;

COMMENT ON COLUMN prestamo.destino IS
    'A dónde va el equipo: un curso ("1°4°"), una dependencia ("Biblioteca", '
    '"Dirección") o el service, en una salida a reparación. Es texto libre '
    'porque la lista de destinos de una institución no se puede cerrar.';

-- +goose Down

COMMENT ON COLUMN prestamo.destino IS NULL;

ALTER TABLE prestamo RENAME COLUMN destino TO motivo;
