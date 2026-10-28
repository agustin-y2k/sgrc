-- +goose Up
-- Una copia histórica por préstamo. Los identificadores académicos son datos
-- de procedencia, sin FK: borrar la estructura no debe borrar la constancia.
CREATE TABLE prestamo_contexto_academico (
    prestamo_id uuid PRIMARY KEY REFERENCES prestamo(id) ON DELETE CASCADE,
    reserva_id_original uuid NOT NULL,
    materia_id uuid NOT NULL,
    materia_nombre varchar(100) NOT NULL CHECK (btrim(materia_nombre) <> ''),
    contenedor_id uuid NOT NULL,
    contenedor_tipo text NOT NULL CHECK (contenedor_tipo IN ('CURSO','ESPACIO')),
    contenedor_nombre varchar(100) NOT NULL CHECK (btrim(contenedor_nombre) <> ''),
    curso_anio smallint,
    curso_division varchar(12),
    modalidad varchar(80),
    ciclo_id uuid NOT NULL,
    ciclo_anio smallint NOT NULL,
    reconstruido boolean NOT NULL DEFAULT false,
    CHECK ((contenedor_tipo='CURSO' AND curso_anio IS NOT NULL)
        OR (contenedor_tipo='ESPACIO' AND curso_anio IS NULL AND curso_division IS NULL AND modalidad IS NULL))
);

-- Sólo se reconstruyen vínculos que todavía existen. No se infieren materias
-- a partir del responsable, el destino libre o una coincidencia de fechas.
INSERT INTO prestamo_contexto_academico (
    prestamo_id,reserva_id_original,materia_id,materia_nombre,
    contenedor_id,contenedor_tipo,contenedor_nombre,curso_anio,curso_division,modalidad,
    ciclo_id,ciclo_anio,reconstruido
)
SELECT p.id,r.id,m.id,m.nombre,cm.id,cm.tipo,cm.nombre,cm.anio,c.division,cm.modalidad,
       cl.id,cl.anio,true
FROM prestamo p JOIN reserva r ON r.id=p.reserva_id
JOIN materia m ON m.id=r.materia_id
JOIN contenedor_de_materia cm ON cm.materia_id=m.id
JOIN ciclo_lectivo cl ON cl.id=cm.ciclo_lectivo_id
LEFT JOIN curso c ON c.id=m.curso_id;

UPDATE prestamo p SET destino=ca.contenedor_nombre ||
    CASE WHEN coalesce(ca.modalidad,'')='' THEN '' ELSE ' · '||ca.modalidad END
FROM prestamo_contexto_academico ca
WHERE ca.prestamo_id=p.id AND coalesce(btrim(p.destino),'')='';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM prestamo_contexto_academico) THEN
        RAISE EXCEPTION 'Hay préstamos con contexto histórico; respaldarlo y revisarlo antes de revertir la migración 031';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE prestamo_contexto_academico;
