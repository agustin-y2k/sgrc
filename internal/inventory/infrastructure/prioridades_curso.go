package infrastructure

import (
	"context"
	"fmt"

	"github.com/agustin-y2k/sgrc/internal/shared/prioridadcurso"
)

func (r *PostgresRepo) PrioridadesDeCurso(ctx context.Context, cursoID string, destino prioridadcurso.Alcance) ([]prioridadcurso.Marca, error) {
	rows, err := r.conexion(ctx).Query(ctx, `
WITH alcances AS (
 SELECT ep.*,
  CASE WHEN ep.curso_exacto THEN ep.anio=cu.anio AND ep.division_norm=cu.division_norm AND ep.modalidad_norm=cu.modalidad_norm
   ELSE (ep.anio IS NULL OR ep.anio=cu.anio) AND (ep.division_norm='' OR ep.division_norm=cu.division_norm) AND (ep.modalidad_norm='' OR ep.modalidad_norm=cu.modalidad_norm) END AS antes,
  CASE WHEN ep.curso_exacto THEN ep.anio=$2 AND ep.division_norm=translate(lower($3), 'áéíóúüñ', 'aeiouun') AND ep.modalidad_norm=translate(lower($4), 'áéíóúüñ', 'aeiouun')
   ELSE (ep.anio IS NULL OR ep.anio=$2) AND (ep.division_norm='' OR ep.division_norm=translate(lower($3), 'áéíóúüñ', 'aeiouun')) AND (ep.modalidad_norm='' OR ep.modalidad_norm=translate(lower($4), 'áéíóúüñ', 'aeiouun')) END AS despues
 FROM equipo_preferencia ep JOIN curso cu ON cu.id=$1
 WHERE EXISTS(SELECT 1 FROM materia m WHERE m.curso_id=cu.id AND m.nombre_norm=ep.materia_norm)
)
SELECT ep.id,ep.equipo_id,COALESCE(e.nombre,'PC '||e.identificador,''),COALESCE(c.nombre,''),ep.materia_nombre,ep.prioridad,ep.curso_exacto,ep.antes,ep.despues,
 EXISTS(SELECT 1 FROM curso otro JOIN materia m ON m.curso_id=otro.id
   WHERE otro.id<>$1 AND m.nombre_norm=ep.materia_norm AND otro.anio=ep.anio AND otro.division_norm=ep.division_norm AND otro.modalidad_norm=ep.modalidad_norm),
 COALESCE(dest.id::text,''),COALESCE(dest.prioridad,0)
FROM alcances ep JOIN equipo e ON e.id=ep.equipo_id LEFT JOIN carro c ON c.id=e.carro_id
LEFT JOIN equipo_preferencia dest ON dest.equipo_id=ep.equipo_id AND dest.materia_norm=ep.materia_norm
 AND dest.curso_exacto AND dest.anio=$2 AND dest.division_norm=translate(lower($3), 'áéíóúüñ', 'aeiouun') AND dest.modalidad_norm=translate(lower($4), 'áéíóúüñ', 'aeiouun')
WHERE ep.antes OR ep.despues
ORDER BY ep.id`, cursoID, destino.Anio, destino.Division, destino.Modalidad)
	if err != nil {
		return nil, fmt.Errorf("evaluando prioridades del curso: %w", err)
	}
	defer rows.Close()
	marcas := []prioridadcurso.Marca{}
	for rows.Next() {
		var m prioridadcurso.Marca
		if err := rows.Scan(&m.ID, &m.EquipoID, &m.EquipoEtiqueta, &m.CarroNombre, &m.MateriaNombre, &m.Prioridad, &m.Exacta, &m.Antes, &m.Despues, &m.Compartida, &m.DestinoID, &m.DestinoPrioridad); err != nil {
			return nil, err
		}
		marcas = append(marcas, m)
	}
	return marcas, rows.Err()
}
