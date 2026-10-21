package infrastructure

import (
	"context"
	"fmt"

	"github.com/agustin-y2k/sgrc/internal/inventory/application"
	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
)

func (r *PostgresRepo) CatalogoDePrioridades(ctx context.Context) ([]application.MateriaParaPreferencia, error) {
	rows, err := r.conexion(ctx).Query(ctx, `
 SELECT m.id,m.nombre,cm.id,cm.nombre,cm.tipo,cm.modalidad,cm.anio,c.division
 FROM materia m JOIN contenedor_de_materia cm ON cm.materia_id=m.id
 JOIN ciclo_lectivo cl ON cl.id=cm.ciclo_lectivo_id
 LEFT JOIN curso c ON c.id=m.curso_id
 WHERE cl.activo AND NOT cl.archivado AND NOT cm.archivado AND NOT m.archivado
 ORDER BY cm.modalidad NULLS FIRST,cm.anio NULLS LAST,c.division NULLS FIRST,m.nombre,m.id`)
	if err != nil {
		return nil, fmt.Errorf("consultando el catálogo de prioridades: %w", err)
	}
	defer rows.Close()
	resultado := []application.MateriaParaPreferencia{}
	for rows.Next() {
		var f application.MateriaParaPreferencia
		if err := rows.Scan(&f.MateriaID, &f.MateriaNombre, &f.ContenedorID, &f.ContenedorNombre, &f.Tipo, &f.Modalidad, &f.Anio, &f.Division); err != nil {
			return nil, err
		}
		resultado = append(resultado, f)
	}
	return resultado, rows.Err()
}

func (r *PostgresRepo) ExisteAlcancePreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) (bool, error) {
	var existe bool
	err := r.conexion(ctx).QueryRow(ctx, `
 SELECT EXISTS (
 SELECT 1 FROM materia m JOIN contenedor_de_materia cm ON cm.materia_id=m.id
 JOIN ciclo_lectivo cl ON cl.id=cm.ciclo_lectivo_id
 WHERE cl.activo AND NOT cl.archivado AND NOT cm.archivado AND NOT m.archivado
 AND m.nombre_norm=translate(lower(btrim($1::text)),'áéíóúüñ','aeiouun')
 AND CASE WHEN $5 THEN
   cm.anio IS NOT DISTINCT FROM $3::smallint
   AND cm.modalidad_norm=translate(lower(btrim(coalesce($2::text,''))),'áéíóúüñ','aeiouun')
   AND cm.division_norm=translate(lower(btrim(coalesce($4::text,''))),'áéíóúüñ','aeiouun')
 ELSE
   ($3::smallint IS NULL OR cm.anio IS NOT DISTINCT FROM $3::smallint)
   AND ($2::text IS NULL OR cm.modalidad_norm=translate(lower(btrim($2::text)),'áéíóúüñ','aeiouun'))
   AND ($4::text IS NULL OR cm.division_norm=translate(lower(btrim($4::text)),'áéíóúüñ','aeiouun'))
 END)`, p.MateriaNombre, p.Modalidad, p.Anio, p.Division, p.CursoExacto).Scan(&existe)
	return existe, err
}
