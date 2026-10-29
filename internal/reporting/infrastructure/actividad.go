package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/agustin-y2k/sgrc/internal/reporting/application"
	"github.com/agustin-y2k/sgrc/internal/reporting/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
)

// Mantener estable el cierre entre leer el estado del ciclo y agregar sus fuentes.
func (r *PostgresRepo) CalcularActividad(ctx context.Context, cicloID string, desde, hasta *time.Time) (domain.Actividad, error) {
	var a domain.Actividad
	err := pgtransaccion.Coordinar(ctx, r.pool, false, func(ctx context.Context) error {
		var err error
		a, err = r.calcularActividad(ctx, cicloID, desde, hasta)
		return err
	})
	return a, err
}

// Ambas fuentes se agregan en una sola consulta, con el mismo snapshot de lectura.
// El año civil sitúa los préstamos sin contexto académico; sólo los vínculos
// explícitos permiten atribuir un préstamo a una reserva o a un ciclo concreto.
func (r *PostgresRepo) calcularActividad(ctx context.Context, cicloID string, desde, hasta *time.Time) (domain.Actividad, error) {
	var a domain.Actividad
	var anio int
	var archivado bool
	var guardado []byte
	err := pgtransaccion.Con(ctx, r.pool).QueryRow(ctx, `SELECT c.anio,c.archivado,s.actividad FROM ciclo_lectivo c LEFT JOIN snapshot_ciclo s ON s.ciclo_id=c.id WHERE c.id=$1`, cicloID).Scan(&anio, &archivado, &guardado)
	if errors.Is(err, pgx.ErrNoRows) || esIDInvalido(err) {
		return a, application.ErrIDInvalido
	}
	if err != nil {
		return a, err
	}
	if archivado {
		if desde != nil || hasta != nil {
			return a, application.ErrHistoricoSinRango
		}
		if len(guardado) == 0 {
			return domain.Actividad{Anio: anio, Historico: true, Disponible: false, Equipos: []domain.ActividadEquipo{}, Responsables: []domain.ActividadResponsable{}}, nil
		}
		err = json.Unmarshal(guardado, &a)
		a.Historico = true
		return a, err
	}
	a = domain.Actividad{Version: 1, Anio: anio, Disponible: true, Equipos: []domain.ActividadEquipo{}, Responsables: []domain.ActividadResponsable{}}
	rows, err := pgtransaccion.Con(ctx, r.pool).Query(ctx, `
 WITH fuentes AS (
  SELECT r.equipo_id,COALESCE(r.creado_por::text,r.docente_id_historico::text) AS identidad,
   r.creado_por AS usuario_id,COALESCE(u.nombre||' '||u.apellido,r.nombre_docente_snapshot,'Cuenta eliminada') AS nombre,
   1::bigint AS solicitadas,
   CASE WHEN EXISTS(SELECT 1 FROM prestamo p WHERE p.reserva_id=r.id) THEN 1 ELSE 0 END::bigint AS atendidas,
   CASE WHEN r.estado='NO_RETIRADA' AND NOT EXISTS(SELECT 1 FROM prestamo p WHERE p.reserva_id=r.id) THEN 1 ELSE 0 END::bigint AS no_retiradas,
   CASE WHEN r.estado='CANCELADA' THEN 1 ELSE 0 END::bigint AS canceladas,
   0::bigint AS con_reserva,0::bigint AS sin_reserva,0::bigint AS desconocido,0::bigint AS abiertos,0::bigint AS devueltos
  FROM reserva r LEFT JOIN usuario u ON u.id=r.creado_por
  WHERE r.tipo='NORMAL' AND r.materia_id IN(SELECT materia_id FROM contenedor_de_materia WHERE ciclo_lectivo_id=$1)
   AND ($3::date IS NULL OR r.fecha >= $3) AND ($4::date IS NULL OR r.fecha <= $4)
  UNION ALL
  SELECT p.equipo_id,COALESCE(p.entregado_a_usuario_id::text,'prestamo:'||p.id::text),p.entregado_a_usuario_id,p.entregado_a_nombre,
   0,0,0,0,
   CASE WHEN p.origen='CON_RESERVA' THEN 1 ELSE 0 END,CASE WHEN p.origen='SIN_RESERVA' THEN 1 ELSE 0 END,
   CASE WHEN p.origen='NO_DETERMINADO' THEN 1 ELSE 0 END,
   CASE WHEN p.devuelto_en IS NULL THEN 1 ELSE 0 END,CASE WHEN p.devuelto_en IS NOT NULL THEN 1 ELSE 0 END
  FROM prestamo p LEFT JOIN prestamo_contexto_academico ca ON ca.prestamo_id=p.id
  LEFT JOIN reserva r ON r.id=p.reserva_id
  WHERE (ca.ciclo_id=$1 OR (ca.prestamo_id IS NULL AND r.materia_id IN(SELECT materia_id FROM contenedor_de_materia WHERE ciclo_lectivo_id=$1))
   OR (ca.prestamo_id IS NULL AND r.materia_id IS NULL AND EXTRACT(YEAR FROM p.entregado_en AT TIME ZONE $2)=$5))
   AND ($3::date IS NULL OR (p.entregado_en AT TIME ZONE $2)::date >= $3)
   AND ($4::date IS NULL OR (p.entregado_en AT TIME ZONE $2)::date <= $4)
 ), agrupado AS (
 SELECT CASE WHEN GROUPING(f.equipo_id)=0 THEN 'equipo' WHEN GROUPING(f.identidad)=0 THEN 'responsable' ELSE 'total' END AS tipo,
  f.equipo_id::text,f.identidad,MAX(f.usuario_id::text),MAX(f.nombre),
  COALESCE(SUM(solicitadas),0),COALESCE(SUM(atendidas),0),COALESCE(SUM(no_retiradas),0),COALESCE(SUM(canceladas),0),
  COALESCE(SUM(con_reserva),0),COALESCE(SUM(sin_reserva),0),COALESCE(SUM(desconocido),0),COALESCE(SUM(abiertos),0),COALESCE(SUM(devueltos),0)
 FROM fuentes f GROUP BY GROUPING SETS ((f.equipo_id),(f.identidad),())
 ) SELECT a.*,COALESCE(e.nombre,'PC '||e.identificador,'Equipo eliminado') FROM agrupado a LEFT JOIN equipo e ON e.id::text=a.equipo_id ORDER BY tipo,identidad,equipo_id
 `, cicloID, r.zona.String(), desde, hasta, anio)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var tipo, etiqueta string
		var eq, identidad, usuario, nombre *string
		var m domain.MetricasActividad
		if err := rows.Scan(&tipo, &eq, &identidad, &usuario, &nombre, &m.ReservasSolicitadas, &m.ReservasAtendidas, &m.ReservasNoRetiradas, &m.ReservasCanceladas, &m.PrestamosConReserva, &m.PrestamosSinReserva, &m.PrestamosOrigenNoDeterminado, &m.PrestamosAbiertos, &m.PrestamosDevueltos, &etiqueta); err != nil {
			return a, err
		}
		switch tipo {
		case "total":
			a.Totales = m
		case "equipo":
			a.Equipos = append(a.Equipos, domain.ActividadEquipo{EquipoID: *eq, Etiqueta: etiqueta, MetricasActividad: m})
		case "responsable":
			a.Responsables = append(a.Responsables, domain.ActividadResponsable{Identidad: *identidad, Nombre: *nombre, UsuarioID: usuario, MetricasActividad: m})
		}
	}
	return a, rows.Err()
}
