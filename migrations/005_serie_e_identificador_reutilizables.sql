-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Lo que se dio de baja deja libres su número de serie y su zócalo
-- ═══════════════════════════════════════════════════════════════════════
--
-- Dar de baja un equipo es una baja lógica: la fila se queda para que su
-- historial de incidencias, préstamos y reservas siga existiendo. Eso está
-- bien y no cambia acá.
--
-- Lo que estaba mal es que la fila también se quedaba con los identificadores
-- puestos. `numero_serie` era UNIQUE de tabla y `(carro_id, identificador)`
-- también, sin distinguir un equipo vivo de uno que ya salió del inventario.
-- El resultado es que dar de baja no libera el lugar: al volver a cargar la
-- misma máquina el sistema responde "ya existe un equipo con ese número de
-- serie", y el equipo que lo tiene no aparece en ninguna pantalla porque está
-- dado de baja. Con el número de zócalo pasa lo mismo: el "7" del carro que
-- se dio de baja no se puede volver a usar.
--
-- Y es una inconsistencia del esquema, no una decisión: el nombre de los
-- equipos sueltos ya se libera al dar de baja desde la 001, con el argumento
-- —correcto— de que el nombre de algo que salió del inventario puede
-- reutilizarse. Esta migración le aplica el mismo criterio a los otros dos.
--
-- Un caso concreto que esto destraba: una notebook de un carro que cambia de
-- naturaleza —tiene otro hardware y pasa a ser un equipo suelto— se da de baja
-- y se vuelve a crear con su serie de fábrica intacta.
--
-- Lo que NO cambia: entre los equipos vivos la serie sigue siendo única en
-- toda la institución, y el número de zócalo sigue siendo único dentro de su
-- carro. Dos máquinas en uso no pueden compartir ninguno de los dos.

-- +goose Up

-- Las dos restricciones nacieron como UNIQUE de tabla, así que se van con
-- DROP CONSTRAINT (que se lleva puesto su índice) y vuelven como índices
-- únicos parciales, que es la única forma de expresar "único entre los que
-- siguen en el inventario".

ALTER TABLE equipo DROP CONSTRAINT equipo_numero_serie_key;

CREATE UNIQUE INDEX ux_equipo_numero_serie
    ON equipo (numero_serie) WHERE dado_de_baja = false;

ALTER TABLE equipo DROP CONSTRAINT equipo_carro_id_identificador_key;

CREATE UNIQUE INDEX ux_equipo_carro_identificador
    ON equipo (carro_id, identificador) WHERE dado_de_baja = false;

-- +goose Down

-- Ojo: esta vuelta atrás puede fallar, y es correcto que falle. Si mientras
-- la 005 estuvo aplicada alguien reutilizó una serie o un zócalo que tenía un
-- equipo dado de baja, la base ya contiene el duplicado que el UNIQUE global
-- prohíbe, y no hay forma automática de elegir cuál de los dos equipos pierde
-- su identificador. Hay que resolverlo a mano antes de bajar de versión.

DROP INDEX ux_equipo_numero_serie;

ALTER TABLE equipo ADD CONSTRAINT equipo_numero_serie_key UNIQUE (numero_serie);

DROP INDEX ux_equipo_carro_identificador;

ALTER TABLE equipo ADD CONSTRAINT equipo_carro_id_identificador_key UNIQUE (carro_id, identificador);
