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
// Format ya trunca a año/mes/día tomando la zona del propio time.Time —que
// es la de la escuela, ver APP_TIMEZONE— así que no hay ninguna conversión
// de zona de por medio ni depende de la de la sesión de Postgres.
const formatoFechaSQL = "2006-01-02"

// Las dos consultas del barrido (RF-08.10 a RF-08.13) y sus cuatro marcas.
//
// El JOIN con `usuario` para sacar nombre y email es SQL directo, sin pasar
// por internal/auth — mismo criterio que ObtenedorNombreDocentePostgres y
// que el ListadorAdmins de notification: es una lectura de dos columnas, no
// una regla de negocio.

// ReservasAVigilar: las confirmadas de hoy y mañana, con el contacto del
// docente y el estado de custodia de cada máquina.
//
// El LEFT JOIN con prestamo es lo que distingue "el docente no vino" de "el
// docente vino y se la llevó": sin él, el barrido liberaría reservas cuya PC
// está en manos de alguien. Se cruza por equipo_id y no por reserva_id a
// propósito — si la máquina salió por una entrega espontánea en vez de
// contra la reserva, igual está afuera y la franja no se puede liberar.
//
// El rango de fechas es grueso (hoy y mañana) porque la ventana fina la
// decide el dominio. Traer un día de más no cuesta nada; que la consulta
// tenga su propia idea de "una hora antes" sí, porque sería la segunda copia
// de una regla que ya existe.
func (r *PostgresRepo) ReservasAVigilar(ctx context.Context, hoy time.Time) ([]application.ReservaParaVigilar, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			res.id, res.reserva_grupo_id, res.equipo_id, COALESCE(eq.identificador, 0),
			res.fecha, res.hora_inicio, res.hora_fin, res.tipo, m.nombre,
			COALESCE(eq.nombre, 'PC ' || eq.identificador),
			g.creado_por,
			COALESCE(u.nombre || ' ' || u.apellido, res.nombre_docente_snapshot, ''),
			COALESCE(u.email, ''),
			g.recordatorio_enviado_en IS NOT NULL,
			res.avisado_equipo_no_disponible_en IS NOT NULL,
			p.id IS NOT NULL,
			p.devolucion_estimada
		FROM reserva res
		JOIN equipo eq ON eq.id = res.equipo_id
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
		if err := rows.Scan(
			&v.ReservaID, &v.GrupoID, &v.EquipoID, &v.Identificador,
			&v.Fecha, &v.HoraInicio, &v.HoraFin, &tipo, &v.MateriaNombre, &v.Etiqueta,
			&v.DocenteID, &v.DocenteNombre, &v.DocenteEmail,
			&v.RecordatorioEnviado, &v.AvisoEquipoNoDisponibleEnviado,
			&v.EquipoAfuera, &v.EquipoDebioVolverA,
		); err != nil {
			return nil, fmt.Errorf("escaneando reserva a vigilar: %w", err)
		}
		parsed, err := domain.ParseTipoReserva(tipo)
		if err != nil {
			return nil, fmt.Errorf("tipo inválido en la base para la reserva %s: %w", v.ReservaID, err)
		}
		v.Tipo = parsed
		resultado = append(resultado, v)
	}
	return resultado, errorDeFilas(rows)
}

// PrestamosAVigilar: todos los abiertos. Son pocos por definición —lo que
// hay afuera del laboratorio en este momento— así que no hace falta filtrar
// por demora acá; eso lo decide el dominio, que además es quien sabe que un
// préstamo sin hora pactada nunca está demorado.
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

		if err := rows.Scan(
			&p.ID, &p.EquipoID, &p.ReservaID, &p.EntregadoAUsuarioID, &p.EntregadoANombre,
			&motivo, &p.DevolucionEstimada, &p.EntregadoPor, &p.EntregadoEn,
			&p.DevueltoEn, &p.RecibidoPor, &observaciones,
			&p.AvisadoDemoraEn, &p.AvisadoCierrePara,
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
//
// Cada una toca UNA columna. Entre que el barrido lee y termina de hablar
// con el servidor de correo pueden pasar decenas de segundos, y en ese rato
// un Admin puede haber cancelado la reserva o recibido la máquina desde la
// pantalla: un UPDATE completo pisaría eso con lo que el barrido tenía en
// memoria. Es la misma lección que MarcarAvisosEnviados en licencias.

func (r *PostgresRepo) MarcarRecordatorioEnviado(ctx context.Context, grupoID string, ahora time.Time) error {
	return r.marcar(ctx, `UPDATE reserva_grupo SET recordatorio_enviado_en=$2 WHERE id=$1`,
		grupoID, ahora, "recordatorio")
}

func (r *PostgresRepo) MarcarAvisoEquipoNoDisponible(ctx context.Context, reservaID string, ahora time.Time) error {
	return r.marcar(ctx, `UPDATE reserva SET avisado_equipo_no_disponible_en=$2 WHERE id=$1`,
		reservaID, ahora, "aviso de PC no disponible")
}

func (r *PostgresRepo) MarcarDemoraAvisada(ctx context.Context, prestamoID string, ahora time.Time) error {
	return r.marcar(ctx, `UPDATE prestamo SET avisado_demora_en=$2 WHERE id=$1`,
		prestamoID, ahora, "reclamo de devolución")
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

// ProximaReservaDeEquipo: la siguiente reserva confirmada de esa máquina, con el
// contacto del docente ya resuelto.
//
// LIMIT 1 con el mismo orden que ListarReservasFuturasDeEquipo. Es una consulta
// aparte y no un filtro sobre aquella porque lo que cambia no es el criterio
// sino lo que hace falta traer: para avisar por correo hace falta la
// dirección, y esa consulta devuelve reservas peladas.
func (r *PostgresRepo) ProximaReservaDeEquipo(ctx context.Context, equipoID string, desde time.Time) (*application.ProximaReserva, error) {
	var p application.ProximaReserva
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(g.creado_por::text, ''),
		       COALESCE(u.email, ''),
		       COALESCE(u.nombre || ' ' || u.apellido, res.nombre_docente_snapshot, ''),
		       res.fecha, res.hora_inicio
		FROM reserva res
		LEFT JOIN reserva_grupo g ON g.id = res.reserva_grupo_id
		LEFT JOIN usuario u ON u.id = g.creado_por
		WHERE res.equipo_id = $1 AND `+condicionNoTerminada("$2", "$3")+` AND res.estado = 'CONFIRMADA'
		ORDER BY res.fecha, res.hora_inicio
		LIMIT 1
	`, equipoID, desde, desde).Scan(&p.UsuarioID, &p.Email, &p.Nombre, &p.Fecha, &p.HoraInicio)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // no hay próxima: no es un error
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("buscando la próxima reserva del equipo: %w", err)
	}
	return &p, nil
}
