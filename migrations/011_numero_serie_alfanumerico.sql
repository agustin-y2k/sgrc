-- SGRC — El número de serie de una PC lleva letras
--
-- `pc.numero_serie` era BIGINT: la columna solo aceptaba dígitos. Los
-- números de serie de fábrica no son números — son códigos alfanuméricos
-- ("5CD1234ABC", "PF2K9L3M"), y prácticamente ningún fabricante usa solo
-- dígitos. Así que la primera PC que alguien intenta cargar con el número
-- que dice la etiqueta no entra.
--
-- No es un caso raro que se descubre tarde: aparece al cargar el inventario
-- por primera vez, que es lo primero que hace un Admin después de instalar
-- el sistema.
--
-- Pasa a TEXT, y la app lo normaliza a mayúsculas sin espacios al crear
-- (ver domain.NormalizarNumeroSerie). El UNIQUE se conserva: sigue siendo
-- único en toda la institución, porque es el identificador de fábrica del
-- equipo.
--
-- Por qué normalizar y no guardar tal cual se tipeó: mismo motivo que la
-- 004 con los emails. Sin una forma canónica, "5cd1234abc" y "5CD1234ABC"
-- son dos filas distintas para Postgres, o sea la misma máquina cargada dos
-- veces — y el UNIQUE, que existe justo para impedir eso, no se entera. Las
-- etiquetas vienen en mayúsculas, así que la forma canónica es la que ya
-- está impresa en el equipo.

BEGIN;

-- ══════════════════════════════════════════════════════════════════
-- 1. El tipo
-- ══════════════════════════════════════════════════════════════════
-- USING con cast explícito: un BIGINT se convierte a texto sin ambigüedad
-- ni pérdida, y los que ya estaban cargados quedan como la misma cadena de
-- dígitos que se veía en pantalla. El UNIQUE viaja con la columna, no hace
-- falta recrearlo.
--
-- VARCHAR(50) y no TEXT libre: es un dato de etiqueta, no un campo de
-- notas. El tope corta el pegado accidental de media página, que sin él
-- entraría sin que nada lo objete.
ALTER TABLE pc
    ALTER COLUMN numero_serie TYPE VARCHAR(50) USING numero_serie::text;

-- ══════════════════════════════════════════════════════════════════
-- 2. Las reglas que antes daba el tipo gratis
-- ══════════════════════════════════════════════════════════════════
-- BIGINT NOT NULL impedía por construcción una cadena vacía o con espacios.
-- Con texto eso hay que pedirlo: sin este CHECK, un INSERT a mano con ''
-- pasaría, y una PC sin número de serie es una PC que no se puede
-- identificar físicamente cuando aparece rota.
--
-- La forma canónica también se exige acá y no solo en la app, por lo mismo
-- que la 004: es lo que la hace cumplirse aunque alguien escriba en la base
-- por afuera del sistema.
ALTER TABLE pc
    ADD CONSTRAINT pc_numero_serie_no_vacio
    CHECK (numero_serie <> '' AND numero_serie = upper(btrim(numero_serie)));

COMMENT ON COLUMN pc.numero_serie IS
    'Número de serie de fábrica, alfanumérico. Único en toda la institución. '
    'Se guarda en mayúsculas y sin espacios al borde (ver 011).';

COMMIT;
