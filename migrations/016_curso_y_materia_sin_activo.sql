-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — Se van dos banderas que nunca significaron nada
-- ═══════════════════════════════════════════════════════════════════════
--
-- `curso.activo` y `materia.activo` existen desde el esquema inicial y nunca
-- llegaron a querer decir algo:
--
--   * ninguna línea del código las escribe con otro valor que el `true` del
--     alta — se puede verificar con un grep, no hay ni un UPDATE que las baje;
--   * ninguna regla las consulta para decidir nada: quien pregunta si una
--     materia acepta reservas mira `archivado`, en los tres niveles
--     (materia, curso y ciclo);
--   * en los datos cargados están todas en `true`.
--
-- Pero sí se leían, se escribían, se serializaban en la respuesta JSON de la
-- API y estaban declaradas en los tipos del frontend. Es decir: el contrato
-- público ofrecía dos banderas que parecen significar «este curso está en
-- uso», y cualquiera que escribiera un cliente contra esta API iba a filtrar
-- por ellas y a no filtrar nada.
--
-- ── Por qué se van y no se les da un significado ───────────────────────
--
-- Porque el significado que tendrían ya lo tiene `archivado`, que sí es real:
-- archivar un ciclo lo propaga a sus cursos y a sus materias, y eso es lo que
-- saca a una materia de circulación sin borrarla. Inventarle a `activo` una
-- semántica aparte —«existe pero no se dicta este año»— sería agregar un
-- requisito que la institución no pidió, y un segundo eje de estados que
-- habría que explicar en cada pantalla.
--
-- Ojo con el nombre: `ciclo_lectivo.activo` NO se toca y no tiene nada que ver.
-- Ésa marca cuál es el único ciclo abierto, la sostiene un índice único parcial
-- y la usa hasta el frontend para saber si se puede crear otro.
--
-- ── Es un cambio incompatible del contrato, y es seguro ────────────────
--
-- Los campos `activo` desaparecen de las respuestas de curso y de materia. No
-- puede cambiar el comportamiento de ningún cliente: el valor era siempre el
-- mismo, así que nadie pudo haber construido una decisión sobre él que hoy
-- estuviera haciendo algo.

-- +goose Up

ALTER TABLE curso   DROP COLUMN activo;
ALTER TABLE materia DROP COLUMN activo;

COMMENT ON COLUMN curso.archivado IS
    'El curso salió de circulación porque se archivó su ciclo (RF-02.4). Es el '
    'ÚNICO estado de un curso: la bandera `activo` que lo acompañaba no '
    'significaba nada y se quitó en la migración 016.';

COMMENT ON COLUMN materia.archivado IS
    'La materia salió de circulación porque se archivó su ciclo (RF-02.4). '
    'Junto con el archivado del curso y del ciclo es lo que decide si acepta '
    'reservas; ver ValidadorMateriaPostgres.MateriaAceptaReservas.';

-- +goose Down

-- Vuelven en `true`, que es el único valor que tuvieron en su vida.
ALTER TABLE curso   ADD COLUMN activo BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE materia ADD COLUMN activo BOOLEAN NOT NULL DEFAULT true;
