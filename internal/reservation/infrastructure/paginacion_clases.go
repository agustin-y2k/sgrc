package infrastructure

import "fmt"

// Misma unidad que agruparReservas en la pantalla: grupo normal, bloqueo del
// mismo actor/fecha/franja o reserva histórica sin grupo.
const unidadDeReserva = `CASE
	WHEN res.reserva_grupo_id IS NOT NULL THEN 'grupo:' || res.reserva_grupo_id::text
	WHEN res.tipo='BLOQUEO' THEN 'bloqueo:' || COALESCE(res.creado_por::text,'sistema') || ':' ||
		res.fecha::text || ':' || to_char(res.hora_inicio,'HH24:MI') || ':' || to_char(res.hora_fin,'HH24:MI')
	ELSE 'sin-grupo:' || res.id::text END`

func consultasPorClases(desde, desdeVisibles string, incluirCanceladas bool, limite, offset int) (string, string) {
	// Los filtros eligen las clases. Una vez elegidas, se devuelven sus miembros
	// visibles completos, incluidos los cancelados dentro de una clase vigente.
	base := `WITH candidatas AS (
		SELECT ` + unidadDeReserva + ` AS unidad, res.fecha, res.hora_inicio, res.estado ` + desde + `
	), unidades AS (
		SELECT unidad, MIN(fecha) AS fecha, MIN(hora_inicio) AS hora_inicio
		FROM candidatas GROUP BY unidad`
	if !incluirCanceladas {
		base += ` HAVING bool_or(estado <> 'CANCELADA')`
	}
	base += `)`
	conteo := base + ` SELECT COUNT(*) FROM unidades`
	query := base + fmt.Sprintf(`, pagina AS (
		SELECT unidad, fecha, hora_inicio, COUNT(*) OVER() AS total
		FROM unidades ORDER BY fecha, hora_inicio, unidad LIMIT $%d OFFSET $%d
	), visibles AS (
		SELECT `+columnasReservaConPrefijo("res")+`,
			COALESCE(p.identificador,0) AS identificador,
			COALESCE(p.nombre,'PC ' || p.identificador) AS etiqueta,
			COALESCE(ca.nombre,'') AS carro_nombre,
			COALESCE(m.nombre,'') AS materia_nombre, COALESCE(cu.nombre,'') AS curso_nombre,
			rg.regla_recurrencia_id, `+unidadDeReserva+` AS unidad `+desdeVisibles+`
	)
	SELECT `+columnasReservaConPrefijo("visibles")+`,
		visibles.identificador, visibles.etiqueta, visibles.carro_nombre,
		visibles.materia_nombre, visibles.curso_nombre, visibles.regla_recurrencia_id, pagina.total
	FROM visibles JOIN pagina ON pagina.unidad=visibles.unidad
	ORDER BY pagina.fecha, pagina.hora_inicio, pagina.unidad,
		visibles.identificador, visibles.equipo_id`, limite, offset)
	return query, conteo
}
