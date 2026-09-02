package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
)

// formatoFechaSQL: el día viaja como texto con un cast a ::date explícito.
const formatoFechaSQL = "2006-01-02"

// Las dos consultas del barrido (RF-08.10 a RF-08.13) y sus cuatro marcas.

// ReservasAVigilar: las confirmadas de hoy y mañana, con el contacto del
// docente y el estado de custodia de cada máquina.
func (r *PostgresRepo) ReservasAVigilar(ctx context.Context, hoy time.Time) ([]application.ReservaParaVigilar, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			res.id, res.reserva_grupo_id, res.equipo_id, COALESCE(eq.identificador, 0),
			res.fecha, res.hora_inicio, res.hora_fin, res.tipo, m.nombre,
			`+etiquetaConCarroSQL("eq", "car")+`,
			g.creado_por,
			COALESCE(u.nombre || ' ' || u.apellido, res.nombre_docente_snapshot, ''),
			COALESCE(u.email, ''),
			g.recordatorio_enviado_en IS NOT NULL,
			p.id IS NOT NULL,
			p.devolucion_estimada,
			(SELECT max(pe.entregado_en)
			   FROM prestamo pe
			   JOIN reserva r2 ON r2.id = pe.reserva_id
			  WHERE r2.reserva_grupo_id = res.reserva_grupo_id)
		FROM reserva res
		JOIN equipo eq ON eq.id = res.equipo_id
		LEFT JOIN carro car ON car.id = eq.carro_id
		LEFT JOIN reserva_grupo g ON g.id = res.reserva_grupo_id
		LEFT JOIN materia m ON m.id = res.materia_id
		LEFT JOIN usuario u ON u.id = g.creado_por
		LEFT JOIN prestamo p ON p.equipo_id = res.equipo_id AND p.devuelto_en IS NULL
		WHERE res.estado = 'CONFIRMADA'
		  AND res.fecha BETWEEN $1::date AND $1::date + 1
		ORDER BY res.fecha, res.hora_inicio, eq.identificador
	`, hoy.Format(formatoFechaSQL))
	if err != nil {
		return nil, fmt.Errorf("listando reservas a vigilar: %w", err)
	}
	defer rows.Close()

	var resultado []application.ReservaParaVigilar
	for rows.Next() {
		var v application.ReservaParaVigilar
		var tipo string
		// hora_inicio y hora_fin son columnas TIME y pgx las entrega como
		// time.Time: escanearlas directo sobre los time.Duration de
		// ReservaParaVigilar corta con "cannot scan time (OID 1083) in binary
		// format into *time.Duration", y con eso muere el barrido entero — esta
		// consulta es la primera de las dos.
		var horaInicio, horaFin time.Time
		if err := rows.Scan(
			&v.ReservaID, &v.GrupoID, &v.EquipoID, &v.Identificador,
			&v.Fecha, &horaInicio, &horaFin, &tipo, &v.MateriaNombre, &v.Etiqueta,
			&v.DocenteID, &v.DocenteNombre, &v.DocenteEmail,
			&v.RecordatorioEnviado,
			&v.EquipoAfuera, &v.EquipoDebioVolverA, &v.UltimaEntregaDelGrupo,
		); err != nil {
			return nil, fmt.Errorf("escaneando reserva a vigilar: %w", err)
		}
		v.HoraInicio = horaComoDuracion(horaInicio)
		v.HoraFin = horaComoDuracion(horaFin)
		parsed, err := domain.ParseTipoReserva(tipo)
		if err != nil {
			return nil, fmt.Errorf("tipo inválido en la base para la reserva %s: %w", v.ReservaID, err)
		}
		v.Tipo = parsed
		resultado = append(resultado, v)
	}
	return resultado, errorDeFilas(rows)
}

// PrestamosAVigilar: todos los abiertos.
func (r *PostgresRepo) PrestamosAVigilar(ctx context.Context) ([]application.PrestamoParaVigilar, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+columnasPrestamoDetallado+`, COALESCE(u.email, '')
		FROM prestamo p
		JOIN equipo eq ON eq.id = p.equipo_id
		LEFT JOIN carro c ON c.id = eq.carro_id
		LEFT JOIN reserva r ON r.id = p.reserva_id
		LEFT JOIN materia m ON m.id = r.materia_id
		LEFT JOIN usuario u ON u.id = p.entregado_a_usuario_id
		WHERE p.devuelto_en IS NULL
		ORDER BY p.devolucion_estimada ASC NULLS LAST, p.entregado_en ASC`)
	if err != nil {
		return nil, fmt.Errorf("listando préstamos a vigilar: %w", err)
	}
	defer rows.Close()

	var resultado []application.PrestamoParaVigilar
	for rows.Next() {
		var p domain.Prestamo
		var v application.PrestamoParaVigilar
		var motivo, observaciones *string
		var materiaNombre *string

		// El orden y la cantidad los manda columnasPrestamoDetallado, que es
		// compartida con prestamo_repo.go: cualquier columna que se le agregue allá
		// hay que recibirla también acá, o pgx corta con "number of field
		// descriptions must equal number of destinations" y el barrido entero deja
		// de correr en silencio.
		if err := rows.Scan(
			&p.ID, &p.EquipoID, &p.ReservaID, &p.EntregadoAUsuarioID, &p.EntregadoANombre,
			&p.RetiradoPor, &motivo, &p.DevolucionEstimada, &p.EntregadoPor, &p.EntregadoEn,
			&p.DevueltoEn, &p.RecibidoPor, &observaciones,
			&p.AvisadoCierrePara,
			&v.Identificador, &v.Etiqueta, &v.CarroNombre, &materiaNombre,
			&v.Email,
		); err != nil {
			return nil, fmt.Errorf("escaneando préstamo a vigilar: %w", err)
		}
		if motivo != nil {
			p.Motivo = *motivo
		}
		if observaciones != nil {
			p.Observaciones = *observaciones
		}
		v.Prestamo = &p
		resultado = append(resultado, v)
	}
	return resultado, errorDeFilas(rows)
}

// ── Las marcas ──────────────────────────────────────────────────────────
// Cada una toca UNA columna.

func (r *PostgresRepo) MarcarRecordatorioEnviado(ctx context.Context, grupoID string, ahora time.Time) error {
	return r.marcar(ctx, `UPDATE reserva_grupo SET recordatorio_enviado_en=$2 WHERE id=$1`,
		grupoID, ahora, "recordatorio")
}

// ContarAvisadosSinDevolver cuenta los equipos por los que ya salió un aviso
// de cierre y que todavía no volvieron.
//
// Es la condición que cierra ese aviso en la campana: mientras quede uno
// afuera el aviso sigue siendo cierto, y cuando el último vuelve deja de
// tener a qué apuntar. Se mira `avisado_cierre_para` y no "todo lo que está
// afuera" a propósito: una máquina que salió esta mañana está afuera pero
// nadie avisó de ella, y no tiene por qué mantener abierto el aviso de anoche.
func (r *PostgresRepo) ContarAvisadosSinDevolver(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM prestamo
		WHERE avisado_cierre_para IS NOT NULL AND devuelto_en IS NULL
	`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contando los equipos avisados que siguen afuera: %w", err)
	}
	return n, nil
}

// MarcarCierreAvisado guarda la JORNADA avisada, no un instante: el corte de
// fin de día se repite mientras la máquina siga afuera, así que lo que hay
// que recordar es "de este día ya avisé", no "avisé alguna vez".
func (r *PostgresRepo) MarcarCierreAvisado(ctx context.Context, prestamoID string, jornada time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE prestamo SET avisado_cierre_para=$2::date WHERE id=$1`,
		prestamoID, jornada.Format(formatoFechaSQL))
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("marcando el aviso de cierre: %w", err)
	}
	return nil
}

func (r *PostgresRepo) marcar(ctx context.Context, sql, id string, ahora time.Time, que string) error {
	if _, err := r.db.Exec(ctx, sql, id, ahora); err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("marcando %s: %w", que, err)
	}
	return nil
}

// ProximaReservaDeEquipo: la siguiente reserva confirmada de esa máquina, con
// el contacto del docente ya resuelto.
func (r *PostgresRepo) ProximaReservaDeEquipo(ctx context.Context, equipoID string, desde time.Time) (*application.ProximaReserva, error) {
	var p application.ProximaReserva
	// hora_inicio es una columna TIME y pgx la entrega como time.Time:
	// escanearla directo sobre el time.Duration de ProximaReserva corta con
	// "cannot scan time (OID 1083) in binary format into *time.Duration".
	var horaInicio time.Time
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(g.creado_por::text, ''),
		       COALESCE(u.email, ''),
		       COALESCE(u.nombre || ' ' || u.apellido, res.nombre_docente_snapshot, ''),
		       res.fecha, res.hora_inicio
		FROM reserva res
		LEFT JOIN reserva_grupo g ON g.id = res.reserva_grupo_id
		LEFT JOIN usuario u ON u.id = g.creado_por
		WHERE res.equipo_id = $1 AND `+condicionNoTerminada("res", "$2", "$3")+` AND res.estado = 'CONFIRMADA'
		ORDER BY res.fecha, res.hora_inicio
		LIMIT 1
	`, equipoID, desde, desde).Scan(&p.UsuarioID, &p.Email, &p.Nombre, &p.Fecha, &horaInicio)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // no hay próxima: no es un error
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("buscando la próxima reserva del equipo: %w", err)
	}
	p.HoraInicio = horaComoDuracion(horaInicio)
	return &p, nil
}
