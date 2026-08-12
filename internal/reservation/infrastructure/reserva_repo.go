package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
)

const columnasReserva = `id, reserva_grupo_id, equipo_id, materia_id, nombre_docente_snapshot, fecha, hora_inicio, hora_fin, estado, tipo, motivo_bloqueo, creado_por, creada_en, cancelado_por, motivo_cancelacion, cancelada_en`

// condicionNoTerminada arma "esta reserva todavía no terminó respecto del
// instante dado", comparando la hora de pared de la reserva contra la hora
// de pared de ese instante. Reemplaza al `fecha >= $2` que usaban
// ListarReservasFuturasDeEquipo/DeMateria, por dos razones.
//
// La primera es un bug concreto: comparar contra la columna DATE pelada
// ignora la hora, así que "las reservas futuras de esta PC" incluía las que
// ya habían terminado ese mismo día. Un Admin que saca una PC de servicio a
// las 14:00 (RF-03.8/03.9) cancelaba también la clase de 8 a 9 de esa
// mañana, y el docente recibía un aviso de que se le canceló una clase que
// ya había dado.
//
// La segunda es no depender de una inferencia implícita. `fecha + hora_fin`
// es un TIMESTAMP SIN ZONA —la hora de pared de la escuela, que es lo que
// significan las columnas DATE y TIME del modelo, ver
// docs/07-modelo-datos.md— y el parámetro llega como un time.Time que la app
// lee en APP_TIMEZONE. Que eso funcione depende hoy de que Postgres infiera
// el tipo del parámetro a partir del operando de la izquierda y de que pgx
// lo codifique tomando los campos de pared: se verificó contra Postgres real
// (ver los tests de zona horaria en postgres_repo_test.go) y así es, sin
// corrimiento. Pero es una propiedad de las reglas de resolución de tipos,
// no algo que el código diga. Los dos casts explícitos —`::date` toma
// año/mes/día, `::time` toma hora/minuto/segundo, del MISMO time.Time que se
// pasa dos veces— lo dejan escrito y lo vuelven inmune a la zona de la
// sesión. Mismo criterio que ListarEquiposDisponiblesEn, que ya usaba
// `$1::date + $2::time`.
func condicionNoTerminada(placeholderFecha, placeholderHora string) string {
	return "(fecha + hora_fin) > (" + placeholderFecha + "::date + " + placeholderHora + "::time)"
}

func (r *PostgresRepo) CrearReserva(ctx context.Context, res *domain.Reserva) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO reserva (id, reserva_grupo_id, equipo_id, materia_id, nombre_docente_snapshot, fecha, hora_inicio, hora_fin, estado, tipo, motivo_bloqueo, creado_por, creada_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, res.ID, res.ReservaGrupoID, res.EquipoID, res.MateriaID, res.NombreDocenteSnapshot, res.Fecha,
		duracionComoHora(res.HoraInicio), duracionComoHora(res.HoraFin),
		string(res.Estado), string(res.Tipo), nullSiVacio(res.MotivoBloqueo), res.CreadoPor, res.CreadaEn)
	if err != nil {
		// La constraint EXCLUDE (anti-solapamiento) es la garantía real
		// contra condiciones de carrera — application.verificarSinSolapamiento
		// ya intenta atraparlo antes, pero si dos pedidos llegan al mismo
		// tiempo, esta es la última línea de defensa.
		if esViolacionExclusion(err) {
			return application.ErrSolapamiento
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando reserva: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarReservaPorID(ctx context.Context, id string) (*domain.Reserva, error) {
	row := r.db.QueryRow(ctx, `SELECT `+columnasReserva+` FROM reserva WHERE id = $1`, id)
	return escanearReserva(row)
}

func escanearReserva(row pgx.Row) (*domain.Reserva, error) {
	var res domain.Reserva
	var horaInicio, horaFin time.Time
	var estadoStr, tipoStr string
	var motivoBloqueo *string

	err := row.Scan(
		&res.ID, &res.ReservaGrupoID, &res.EquipoID, &res.MateriaID, &res.NombreDocenteSnapshot,
		&res.Fecha, &horaInicio, &horaFin, &estadoStr, &tipoStr, &motivoBloqueo,
		&res.CreadoPor, &res.CreadaEn, &res.CanceladoPor, &res.MotivoCancelacion, &res.CanceladaEn,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrReservaNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando reserva: %w", err)
	}

	if motivoBloqueo != nil {
		res.MotivoBloqueo = *motivoBloqueo
	}

	estado, err := domain.ParseEstadoReserva(estadoStr)
	if err != nil {
		return nil, fmt.Errorf("estado inválido en la base para reserva %s: %w", res.ID, err)
	}
	tipo, err := domain.ParseTipoReserva(tipoStr)
	if err != nil {
		return nil, fmt.Errorf("tipo inválido en la base para reserva %s: %w", res.ID, err)
	}
	res.Estado = estado
	res.Tipo = tipo
	res.HoraInicio = horaComoDuracion(horaInicio)
	res.HoraFin = horaComoDuracion(horaFin)

	return &res, nil
}

// GuardarReserva persiste los campos mutables de una reserva ya creada.
//
// `equipo_id` está en la lista porque cambiar de máquina (RF-08.14) es una de
// las cosas que le pasan a una reserva viva, no solo cancelarla o
// finalizarla. Faltaba, y el modo en que fallaba es el que hay que tener
// presente al tocar este UPDATE: el servicio modificaba la reserva en
// memoria, esta función escribía todo menos el equipo, el UPDATE afectaba su
// fila igual —así que no había error— y la respuesta salía con el equipo
// nuevo. El cambio se veía aplicado en pantalla y no existía en la base.
//
// Enumerar columnas a mano tiene ese riesgo: lo que no está en la lista se
// pierde en silencio. Si mañana `Reserva` gana otro campo mutable, va acá.
//
// Para los demás llamadores —las cancelaciones y la finalización— escribir el
// equipo no cambia nada: traen la reserva leída de la base y no lo tocan, así
// que reescriben el mismo valor.
func (r *PostgresRepo) GuardarReserva(ctx context.Context, res *domain.Reserva) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE reserva SET equipo_id=$2, estado=$3, cancelado_por=$4, motivo_cancelacion=$5, cancelada_en=$6
		WHERE id=$1
	`, res.ID, res.EquipoID, string(res.Estado), res.CanceladoPor, res.MotivoCancelacion, res.CanceladaEn)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando reserva: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrReservaNoEncontrada
	}
	return nil
}

func (r *PostgresRepo) ListarReservasPorGrupo(ctx context.Context, reservaGrupoID string) ([]*domain.Reserva, error) {
	rows, err := r.db.Query(ctx, `SELECT `+columnasReserva+` FROM reserva WHERE reserva_grupo_id = $1`, reservaGrupoID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando reservas del grupo: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Reserva
	for rows.Next() {
		res, err := escanearReserva(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de reserva: %w", err)
		}
		resultado = append(resultado, res)
	}
	return resultado, errorDeFilas(rows)
}

// ListarReservasFuturasDeEquipo: todas las reservas CONFIRMADA de una PC que
// todavía no terminaron en el instante `desde`. Usado tanto para el chequeo
// anticipado de solapamiento como para el bloqueo administrativo (RF-04.7) y
// la cascada de cancelación de inventory/academic.
func (r *PostgresRepo) ListarReservasFuturasDeEquipo(ctx context.Context, equipoID string, desde time.Time) ([]*domain.Reserva, error) {
	// ORDER BY: quien llama puede necesitar LA PRÓXIMA, no una cualquiera.
	// Sin él, el aviso de "esta PC tiene una reserva encima" que sale al
	// entregarla suelta podía nombrar la reserva de la semana siguiente en
	// vez de la de dentro de una hora, que es la única que le importa a
	// quien está en el mostrador. Las cascadas de cancelación no dependen
	// del orden, así que esto no les cambia nada.
	rows, err := r.db.Query(ctx, `
		SELECT `+columnasReserva+` FROM reserva
		WHERE equipo_id = $1 AND `+condicionNoTerminada("$2", "$3")+` AND estado = 'CONFIRMADA'
		ORDER BY fecha, hora_inicio
	`, equipoID, desde, desde)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando reservas futuras del equipo: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Reserva
	for rows.Next() {
		res, err := escanearReserva(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de reserva: %w", err)
		}
		resultado = append(resultado, res)
	}
	return resultado, errorDeFilas(rows)
}

// BuscarSolapamientos resuelve el pre-chequeo del lote entero en una
// consulta: todos los equipos contra todas las fechas, con un único rango
// horario. Ver el puerto en application/ports.go para por qué.
//
// El solapamiento se expresa como `hora_inicio < fin AND hora_fin > inicio`,
// que es la misma condición que la constraint EXCLUDE con `&&` sobre
// tsrange: rangos semiabiertos, así que dos bloques que se tocan en el borde
// —uno termina 10:00 y el otro empieza 10:00— NO se pisan. Es el caso más
// común de todos y tiene que poder reservarse.
//
// Se une a equipo y carro para traer la etiqueta ya resuelta (RF-03.17). El
// JOIN a carro va LEFT: un proyector no está en ninguno, y con INNER
// desaparecería del resultado justo cuando es el que choca.
func (r *PostgresRepo) BuscarSolapamientos(ctx context.Context, equipoIDs []string, fechas []time.Time, horaInicio, horaFin time.Duration) ([]application.Solapamiento, error) {
	if len(equipoIDs) == 0 || len(fechas) == 0 {
		return nil, nil
	}

	rows, err := r.db.Query(ctx, `
		SELECT res.equipo_id,
		       COALESCE(e.nombre, 'PC ' || e.identificador, ''),
		       res.fecha, res.hora_inicio, res.hora_fin,
		       COALESCE(res.nombre_docente_snapshot, ''),
		       COALESCE(res.motivo_bloqueo, '')
		FROM reserva res
		JOIN equipo e ON e.id = res.equipo_id
		LEFT JOIN carro c ON c.id = e.carro_id
		WHERE res.equipo_id = ANY($1)
		  AND res.fecha = ANY($2)
		  AND res.estado = 'CONFIRMADA'
		  AND res.hora_inicio < $4 AND res.hora_fin > $3
		ORDER BY res.fecha, res.hora_inicio, e.identificador NULLS LAST, res.equipo_id
	`, equipoIDs, fechas, duracionComoHora(horaInicio), duracionComoHora(horaFin))
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("buscando solapamientos: %w", err)
	}
	defer rows.Close()

	var resultado []application.Solapamiento
	for rows.Next() {
		var s application.Solapamiento
		var inicio, fin time.Time
		if err := rows.Scan(&s.EquipoID, &s.Etiqueta, &s.Fecha, &inicio, &fin, &s.Docente, &s.MotivoBloqueo); err != nil {
			return nil, fmt.Errorf("escaneando solapamiento: %w", err)
		}
		s.HoraInicio, s.HoraFin = horaComoDuracion(inicio), horaComoDuracion(fin)
		resultado = append(resultado, s)
	}
	return resultado, errorDeFilas(rows)
}

// ListarReservasFuturasDeMateria: todas las reservas CONFIRMADA vinculadas
// a una materia (vía su ReservaGrupo) que todavía no terminaron en el
// instante `desde`. Usado por la cascada de auth (RF-02.8: dar de baja al
// último docente de una materia).
func (r *PostgresRepo) ListarReservasFuturasDeMateria(ctx context.Context, materiaID string, desde time.Time) ([]*domain.Reserva, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+columnasReserva+` FROM reserva
		WHERE materia_id = $1 AND `+condicionNoTerminada("$2", "$3")+` AND estado = 'CONFIRMADA'
	`, materiaID, desde, desde)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando reservas futuras de la materia: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Reserva
	for rows.Next() {
		res, err := escanearReserva(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de reserva: %w", err)
		}
		resultado = append(resultado, res)
	}
	return resultado, errorDeFilas(rows)
}

// ListarReservasConfirmadasVencidas: para el job RF-04.9 — todas las
// Reserva CONFIRMADA cuya fecha+horaFin ya pasó respecto de "ahora".
// La comparación se hace con aritmética date+time (mismo criterio que la
// constraint EXCLUDE, ver docs/07-modelo-datos.md) para evitar el mismo
// problema de IMMUTABLE que tuvimos en la migración original.
func (r *PostgresRepo) ListarReservasConfirmadasVencidas(ctx context.Context, ahora time.Time, limite int) ([]*domain.Reserva, error) {
	// De la más vieja a la más nueva: el job procesa por lotes, y así el
	// primero se lleva el atraso más antiguo en vez de una franja al azar.
	rows, err := r.db.Query(ctx, `
		SELECT `+columnasReserva+` FROM reserva
		WHERE estado = 'CONFIRMADA' AND (fecha + hora_fin) < ($1::date + $2::time)
		ORDER BY fecha, hora_fin, id
		LIMIT $3
	`, ahora, ahora, limite)
	if err != nil {
		return nil, fmt.Errorf("listando reservas vencidas: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Reserva
	for rows.Next() {
		res, err := escanearReserva(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de reserva: %w", err)
		}
		resultado = append(resultado, res)
	}
	return resultado, errorDeFilas(rows)
}

// ── ReglaRecurrencia ────────────────────────────────────────────────────

func (r *PostgresRepo) CrearReglaRecurrencia(ctx context.Context, regla *domain.ReglaRecurrencia) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO regla_recurrencia (id, materia_id, creado_por, dia_semana, hora_inicio, hora_fin, fecha_inicio, fecha_fin)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, regla.ID, regla.MateriaID, regla.CreadoPor, string(regla.DiaSemana),
		duracionComoHora(regla.HoraInicio), duracionComoHora(regla.HoraFin), regla.FechaInicio, regla.FechaFin)
	if err != nil {
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando regla de recurrencia: %w", err)
	}
	return nil
}

func (r *PostgresRepo) ListarGruposFuturosDeRegla(ctx context.Context, reglaID string, desde time.Time) ([]*domain.ReservaGrupo, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, materia_id, creado_por, nombre_docente_snapshot, fecha, hora_inicio, hora_fin, estado, regla_recurrencia_id, creada_en
		FROM reserva_grupo
		WHERE regla_recurrencia_id = $1 AND fecha > $2
		ORDER BY fecha
	`, reglaID, desde)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando grupos futuros de la regla: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.ReservaGrupo
	for rows.Next() {
		g, err := escanearReservaGrupo(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de reserva grupo: %w", err)
		}
		resultado = append(resultado, g)
	}
	return resultado, errorDeFilas(rows)
}

// EliminarReservasYGruposDeCiclo: borrado físico (no cancelación) de todo
// ReservaGrupo, Reserva y ReglaRecurrencia de un ciclo lectivo — la cascada
// de archivado de RF-02.4. Se llama siempre DESPUÉS de calcular el snapshot
// histórico, que es lo único que sobrevive del año.
//
// Tres borrados, en este orden:
//
//  1. reserva_grupo — las Reserva hijas se van solas por el ON DELETE
//     CASCADE de reserva.reserva_grupo_id (docs/07-modelo-datos.md).
//  2. regla_recurrencia de las materias del ciclo. RF-02.4 las nombra
//     explícitamente: sin este paso quedan huérfanas para siempre,
//     apuntando a materias archivadas.
//  3. Los bloqueos administrativos del año del ciclo. No tienen
//     materia (RF-04.7), así que la subconsulta de arriba no los alcanza y
//     se acumularían ciclo tras ciclo. Se los ubica por año de la fecha,
//     que es lo único que los ata a un ciclo lectivo.
func (r *PostgresRepo) EliminarReservasYGruposDeCiclo(ctx context.Context, cicloID string) (int, int, error) {
	gruposDelCiclo := `
		SELECT rg.id FROM reserva_grupo rg
		JOIN materia m ON m.id = rg.materia_id
		JOIN curso c ON c.id = m.curso_id
		WHERE c.ciclo_lectivo_id = $1
	`
	materiasDelCiclo := `
		SELECT m.id FROM materia m
		JOIN curso c ON c.id = m.curso_id
		WHERE c.ciclo_lectivo_id = $1
	`
	bloqueosDelCiclo := `
		tipo = 'BLOQUEO'
		AND EXTRACT(YEAR FROM fecha) = (SELECT anio FROM ciclo_lectivo WHERE id = $1)
	`

	var reservasEliminadas int
	err := r.db.QueryRow(ctx,
		`SELECT
			(SELECT COUNT(*) FROM reserva WHERE reserva_grupo_id IN (`+gruposDelCiclo+`))
			+ (SELECT COUNT(*) FROM reserva WHERE `+bloqueosDelCiclo+`)`,
		cicloID,
	).Scan(&reservasEliminadas)
	if err != nil {
		if esIDInvalido(err) {
			return 0, 0, application.ErrIDInvalido
		}
		return 0, 0, fmt.Errorf("contando reservas a eliminar: %w", err)
	}

	tag, err := r.db.Exec(ctx, `DELETE FROM reserva_grupo WHERE id IN (`+gruposDelCiclo+`)`, cicloID)
	if err != nil {
		if esIDInvalido(err) {
			return 0, 0, application.ErrIDInvalido
		}
		return 0, 0, fmt.Errorf("eliminando reserva_grupo del ciclo: %w", err)
	}

	if _, err := r.db.Exec(ctx,
		`DELETE FROM regla_recurrencia WHERE materia_id IN (`+materiasDelCiclo+`)`, cicloID,
	); err != nil {
		return 0, 0, fmt.Errorf("eliminando reglas de recurrencia del ciclo: %w", err)
	}

	if _, err := r.db.Exec(ctx,
		`DELETE FROM reserva WHERE `+bloqueosDelCiclo, cicloID,
	); err != nil {
		return 0, 0, fmt.Errorf("eliminando bloqueos administrativos del ciclo: %w", err)
	}

	return int(tag.RowsAffected()), reservasEliminadas, nil
}

// ListarReservas arma el WHERE dinámicamente a partir del filtro — mismo
// criterio que auth.Listar: con varios filtros opcionales es más legible
// que un COALESCE por columna.
//
// Devuelve los nombres de PC, carro, materia y curso ya resueltos: es un
// listado para mostrar en pantalla, y sin ellos una reserva de varios equipos
// se ve como N filas idénticas. Los JOIN son de solo lectura, igual que en
// CalendarioDeEquipo. El de materia/curso es LEFT porque los bloqueos
// administrativos no tienen materia.
func (r *PostgresRepo) ListarReservas(ctx context.Context, f application.FiltroReservas) ([]application.ReservaDetallada, int, error) {
	// El FROM y el WHERE se arman una sola vez y los comparten las dos
	// consultas posibles (la página y, si hace falta, el conteo suelto):
	// dos copias del mismo filtro es cómo el total termina contando algo
	// distinto de lo que devuelve la lista.
	desde := `
		FROM reserva res
		JOIN equipo p ON p.id = res.equipo_id
		-- LEFT: un proyector reservable no está en ningún carro, y con INNER
		-- su reserva no aparece sin carro — se cae de la consulta entera,
		-- incluido el total paginado. El docente vería que la reserva se hizo
		-- y después no la encontraría para cancelarla.
		LEFT JOIN carro ca ON ca.id = p.carro_id
		LEFT JOIN reserva_grupo rg ON rg.id = res.reserva_grupo_id
		LEFT JOIN materia m ON m.id = res.materia_id
		LEFT JOIN curso cu ON cu.id = m.curso_id
		WHERE 1=1`
	args := []any{}

	agregar := func(condicion string, valor any) {
		args = append(args, valor)
		desde += fmt.Sprintf(" AND res."+condicion, len(args))
	}

	if f.CreadoPor != nil {
		agregar("creado_por = $%d", *f.CreadoPor)
	}
	if f.EquipoID != nil {
		agregar("equipo_id = $%d", *f.EquipoID)
	}
	if f.MateriaID != nil {
		agregar("materia_id = $%d", *f.MateriaID)
	}
	if f.Desde != nil {
		agregar("fecha >= $%d", *f.Desde)
	}
	if f.Hasta != nil {
		agregar("fecha <= $%d", *f.Hasta)
	}
	if !f.IncluirCanceladas {
		desde += " AND res.estado <> 'CANCELADA'"
	}

	query := `
		SELECT ` + columnasReservaConPrefijo("res") + `,
		       COALESCE(p.identificador, 0),
		       COALESCE(p.nombre, 'PC ' || p.identificador),
		       COALESCE(ca.nombre, ''),
		       COALESCE(m.nombre, ''), COALESCE(cu.nombre, ''),
		       rg.regla_recurrencia_id,
		       COUNT(*) OVER() AS total` + desde

	// El ORDER BY es determinista hasta la última columna a propósito: con
	// un orden ambiguo, dos filas empatadas pueden salir en distinto orden
	// entre dos consultas y una misma reserva aparecer dos veces (o ninguna)
	// al pasar de página.
	//
	// El identificador NO alcanza para desempatar: se repite entre carros
	// distintos y es NULL en todo lo que no está en uno —los equipos sueltos
	// empatarían todos entre sí—. res.equipo_id cierra el orden, que es lo
	// único único de verdad acá.
	query += " ORDER BY res.fecha, res.hora_inicio, p.identificador NULLS LAST, res.equipo_id"

	argsPagina := append(append([]any{}, args...), f.Pagina.Limit(), f.Pagina.Offset())
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(argsPagina)-1, len(argsPagina))

	rows, err := r.db.Query(ctx, query, argsPagina...)
	if err != nil {
		if esIDInvalido(err) {
			return nil, 0, application.ErrIDInvalido
		}
		return nil, 0, fmt.Errorf("listando reservas: %w", err)
	}
	defer rows.Close()

	var resultado []application.ReservaDetallada
	total := 0
	for rows.Next() {
		var res domain.Reserva
		var horaInicio, horaFin time.Time
		var estadoStr, tipoStr string
		var motivoBloqueo *string
		var detalle application.ReservaDetallada

		// COUNT(*) OVER() cuenta las filas que matchean el WHERE, antes del
		// LIMIT: da el total en la misma pasada, sin una segunda consulta que
		// podría además ver un estado distinto de la tabla.
		if err := rows.Scan(
			&res.ID, &res.ReservaGrupoID, &res.EquipoID, &res.MateriaID, &res.NombreDocenteSnapshot,
			&res.Fecha, &horaInicio, &horaFin, &estadoStr, &tipoStr, &motivoBloqueo,
			&res.CreadoPor, &res.CreadaEn, &res.CanceladoPor, &res.MotivoCancelacion, &res.CanceladaEn,
			&detalle.Identificador, &detalle.Etiqueta, &detalle.CarroNombre,
			&detalle.MateriaNombre, &detalle.CursoNombre,
			&detalle.ReglaRecurrenciaID,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("escaneando fila de reserva: %w", err)
		}
		if motivoBloqueo != nil {
			res.MotivoBloqueo = *motivoBloqueo
		}

		res.HoraInicio = horaComoDuracion(horaInicio)
		res.HoraFin = horaComoDuracion(horaFin)

		// Por Parse y no por conversión directa, como ya hacían
		// escanearReserva y CalendarioDeEquipo. La conversión acepta cualquier
		// texto: un estado que la base no debería tener entraba igual al
		// dominio y recién se notaba mucho más tarde, como una reserva que no
		// se puede cancelar ni finalizar porque su estado no matchea ninguna
		// transición. Que falle acá deja el error donde está el dato malo.
		estado, err := domain.ParseEstadoReserva(estadoStr)
		if err != nil {
			return nil, 0, fmt.Errorf("estado inválido en la base para reserva %s: %w", res.ID, err)
		}
		tipo, err := domain.ParseTipoReserva(tipoStr)
		if err != nil {
			return nil, 0, fmt.Errorf("tipo inválido en la base para reserva %s: %w", res.ID, err)
		}
		res.Estado = estado
		res.Tipo = tipo

		detalle.Reserva = &res
		resultado = append(resultado, detalle)
	}
	if err := errorDeFilas(rows); err != nil {
		return nil, 0, err
	}

	// Sin filas no hay ventana de la que leer COUNT(*) OVER(). Puede ser que
	// no haya nada, o que la página pedida esté más allá del final —y ahí un
	// total en 0 haría que la pantalla dijera "no tenés reservas" con la
	// página 1 llena. Solo en ese caso vale la segunda consulta.
	if len(resultado) == 0 && f.Pagina.Offset() > 0 {
		if err := r.db.QueryRow(ctx, "SELECT COUNT(*)"+desde, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("contando reservas: %w", err)
		}
	}

	return resultado, total, nil
}

// CalendarioDeEquipo implementa RF-04.4. El LEFT JOIN hacia materia/curso es
// de solo lectura (mismo criterio que los validadores de este paquete
// hacia academic): tiene que ser LEFT porque los bloqueos administrativos
// estatal no tienen materia asociada y también ocupan la PC, así que
// también deben verse en el calendario.
func (r *PostgresRepo) CalendarioDeEquipo(ctx context.Context, equipoID string, desde, hasta time.Time) ([]application.BloqueCalendario, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+columnasReservaConPrefijo("res")+`,
		       COALESCE(m.nombre, ''), COALESCE(c.nombre, '')
		FROM reserva res
		LEFT JOIN materia m ON m.id = res.materia_id
		LEFT JOIN curso c ON c.id = m.curso_id
		WHERE res.equipo_id = $1
		  AND res.fecha >= $2 AND res.fecha <= $3
		  AND res.estado <> 'CANCELADA'
		ORDER BY res.fecha, res.hora_inicio
	`, equipoID, desde, hasta)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando calendario del equipo: %w", err)
	}
	defer rows.Close()

	var resultado []application.BloqueCalendario
	for rows.Next() {
		var res domain.Reserva
		var horaInicio, horaFin time.Time
		var estadoStr, tipoStr string
		var motivoBloqueo *string
		var materiaNombre, cursoNombre string

		if err := rows.Scan(
			&res.ID, &res.ReservaGrupoID, &res.EquipoID, &res.MateriaID, &res.NombreDocenteSnapshot,
			&res.Fecha, &horaInicio, &horaFin, &estadoStr, &tipoStr, &motivoBloqueo,
			&res.CreadoPor, &res.CreadaEn, &res.CanceladoPor, &res.MotivoCancelacion, &res.CanceladaEn,
			&materiaNombre, &cursoNombre,
		); err != nil {
			return nil, fmt.Errorf("escaneando bloque del calendario: %w", err)
		}
		if motivoBloqueo != nil {
			res.MotivoBloqueo = *motivoBloqueo
		}

		estado, err := domain.ParseEstadoReserva(estadoStr)
		if err != nil {
			return nil, fmt.Errorf("estado inválido en la base para reserva %s: %w", res.ID, err)
		}
		tipo, err := domain.ParseTipoReserva(tipoStr)
		if err != nil {
			return nil, fmt.Errorf("tipo inválido en la base para reserva %s: %w", res.ID, err)
		}
		res.Estado = estado
		res.Tipo = tipo
		res.HoraInicio = horaComoDuracion(horaInicio)
		res.HoraFin = horaComoDuracion(horaFin)

		resultado = append(resultado, application.BloqueCalendario{
			Reserva: &res, MateriaNombre: materiaNombre, CursoNombre: cursoNombre,
		})
	}
	return resultado, errorDeFilas(rows)
}

// columnasReservaConPrefijo devuelve la lista de columnas calificada con
// el alias de tabla — hace falta en las consultas con JOIN, donde "id" a
// secas sería ambiguo.
//
// Parte por ", ", así que columnasReserva tiene que ser una lista de nombres
// pelados: una expresión con coma adentro —un COALESCE, por ejemplo— se
// partiría al medio y saldría SQL inválido. Por eso motivo_bloqueo se escanea
// como puntero en vez de coalescerse en la consulta.
func columnasReservaConPrefijo(alias string) string {
	columnas := strings.Split(columnasReserva, ", ")
	for i, c := range columnas {
		columnas[i] = alias + "." + c
	}
	return strings.Join(columnas, ", ")
}

// ReservasDeLaSerieDesde: la misma máquina en las ocurrencias que le quedan a
// la serie, de esta fecha en adelante (RF-08.14).
//
// Devuelve vacío cuando el grupo no tiene regla de recurrencia, porque la
// comparación con NULL no matchea nada. Eso es lo correcto y no un descuido:
// en una reserva suelta "esta y las siguientes" no significa nada distinto de
// "solo esta", y quien llama ya tiene esa reserva en la mano.
func (r *PostgresRepo) ReservasDeLaSerieDesde(ctx context.Context, reservaID string) ([]*domain.Reserva, error) {
	rows, err := r.db.Query(ctx, `
		WITH origen AS (
			SELECT res.equipo_id, g.regla_recurrencia_id, g.fecha
			FROM reserva res
			JOIN reserva_grupo g ON g.id = res.reserva_grupo_id
			WHERE res.id = $1
		)
		SELECT `+prefijar(columnasReserva, "res")+`
		FROM reserva res
		JOIN reserva_grupo g ON g.id = res.reserva_grupo_id
		JOIN origen o ON true
		WHERE res.estado = 'CONFIRMADA'
		  AND res.equipo_id = o.equipo_id
		  AND g.regla_recurrencia_id IS NOT NULL
		  AND g.regla_recurrencia_id = o.regla_recurrencia_id
		  AND g.fecha >= o.fecha
		ORDER BY g.fecha
	`, reservaID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando las reservas de la serie: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Reserva
	for rows.Next() {
		res, err := escanearReserva(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando reserva de la serie: %w", err)
		}
		resultado = append(resultado, res)
	}
	return resultado, errorDeFilas(rows)
}

// prefijar califica una lista de columnas con el alias de su tabla, para
// poder reusar `columnasReserva` en una consulta con JOIN sin que "id" quede
// ambiguo.
func prefijar(columnas, alias string) string {
	partes := strings.Split(columnas, ", ")
	for i, c := range partes {
		partes[i] = alias + "." + c
	}
	return strings.Join(partes, ", ")
}

// DatosParaPedirLiberacion resuelve en una consulta las cuatro condiciones del
// pedido (RF-04.12) y los datos del aviso.
func (r *PostgresRepo) DatosParaPedirLiberacion(ctx context.Context, reservaID string) (*application.ReservaParaPedido, error) {
	row := r.db.QueryRow(ctx, `
		SELECT res.estado, res.tipo, res.fecha, res.hora_inicio, res.hora_fin,
		       COALESCE(eq.nombre, 'PC ' || eq.identificador),
		       COALESCE(m.nombre, ''),
		       g.creado_por,
		       COALESCE(u.nombre || ' ' || u.apellido, res.nombre_docente_snapshot, ''),
		       COALESCE(u.email, '')
		FROM reserva res
		JOIN equipo eq ON eq.id = res.equipo_id
		LEFT JOIN reserva_grupo g ON g.id = res.reserva_grupo_id
		LEFT JOIN materia m ON m.id = res.materia_id
		LEFT JOIN usuario u ON u.id = g.creado_por
		WHERE res.id = $1
	`, reservaID)

	var p application.ReservaParaPedido
	var estado, tipo string
	var inicio, fin time.Time
	if err := row.Scan(&estado, &tipo, &p.Fecha, &inicio, &fin,
		&p.Etiqueta, &p.MateriaNombre, &p.DuenoID, &p.DuenoNombre, &p.DuenoEmail); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrReservaNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("leyendo la reserva para el pedido: %w", err)
	}

	estadoParseado, err := domain.ParseEstadoReserva(estado)
	if err != nil {
		return nil, fmt.Errorf("estado inválido en la base para la reserva %s: %w", reservaID, err)
	}
	p.Estado = estadoParseado
	p.EsBloqueo = tipo == string(domain.TipoBloqueo)
	p.HoraInicio = horaComoDuracion(inicio)
	p.HoraFin = horaComoDuracion(fin)
	return &p, nil
}

// YaPidioLiberacionHoy mira las notificaciones ya emitidas en vez de una
// tabla propia: el pedido no es una entidad, y la fila que igual se escribe
// alcanza para saber que salió.
//
// `sobre_usuario_id` es de quién HABLA el aviso, y en este caso eso es quien
// pide — el aviso le llega al dueño y trata sobre el otro docente. Es el
// mismo uso que en "hay una cuenta esperando aprobación", y cae sobre el
// índice que ya existe para esa columna.
func (r *PostgresRepo) YaPidioLiberacionHoy(ctx context.Context, reservaID, solicitanteID string, dia time.Time) (bool, error) {
	var existe bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM notificacion
			WHERE tipo = 'PEDIDO_DE_LIBERACION'
			  AND reserva_id = $1
			  AND sobre_usuario_id = $2
			  AND creada_en >= $3::date
			  AND creada_en < $3::date + 1
		)
	`, reservaID, solicitanteID, dia.Format(formatoFechaSQL)).Scan(&existe)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando si ya se pidió esa reserva hoy: %w", err)
	}
	return existe, nil
}

// ListarEquiposOcupadosEn es la otra mitad de la franja (RF-04.11): del mismo
// universo que la consulta de abajo —DISPONIBLE, reservable, no dado de baja—
// los que YA tiene alguien, con quién los tiene.
//
// El JOIN con reserva es interno y no un LEFT: acá interesa exactamente lo
// contrario que en la otra consulta. Y no puede devolver dos filas para el
// mismo equipo, porque la constraint EXCLUDE ya garantiza que dos reservas
// confirmadas no se pisen sobre la misma máquina.
func (r *PostgresRepo) ListarEquiposOcupadosEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) ([]application.EquipoOcupado, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id,
		       COALESCE(p.nombre, 'PC ' || p.identificador),
		       COALESCE(c.nombre, ''),
		       res.id, res.tipo, res.hora_inicio, res.hora_fin,
		       g.creado_por,
		       COALESCE(u.nombre || ' ' || u.apellido, res.nombre_docente_snapshot, ''),
		       COALESCE(m.nombre, ''),
		       COALESCE(res.motivo_bloqueo, '')
		FROM equipo p
		JOIN reserva res ON res.equipo_id = p.id
		 AND res.estado = 'CONFIRMADA'
		 AND res.fecha = $1
		 AND tsrange(res.fecha + res.hora_inicio, res.fecha + res.hora_fin)
		     && tsrange($1::date + $2::time, $1::date + $3::time)
		LEFT JOIN carro c ON c.id = p.carro_id
		LEFT JOIN reserva_grupo g ON g.id = res.reserva_grupo_id
		LEFT JOIN materia m ON m.id = res.materia_id
		LEFT JOIN usuario u ON u.id = g.creado_por
		WHERE p.estado = 'DISPONIBLE'
		  AND p.dado_de_baja = false
		  AND p.reservable = true
		ORDER BY p.carro_id IS NULL, c.nombre, p.identificador, p.nombre
	`, fecha, duracionComoHora(horaInicio), duracionComoHora(horaFin))
	if err != nil {
		return nil, fmt.Errorf("listando equipos ocupados: %w", err)
	}
	defer rows.Close()

	var resultado []application.EquipoOcupado
	for rows.Next() {
		var oc application.EquipoOcupado
		var tipo string
		var inicio, fin time.Time
		if err := rows.Scan(&oc.EquipoID, &oc.Etiqueta, &oc.CarroNombre,
			&oc.ReservaID, &tipo, &inicio, &fin,
			&oc.DocenteID, &oc.DocenteNombre, &oc.MateriaNombre, &oc.Motivo); err != nil {
			return nil, fmt.Errorf("escaneando equipo ocupado: %w", err)
		}
		oc.EsBloqueo = tipo == string(domain.TipoBloqueo)
		oc.HoraInicio = horaComoDuracion(inicio)
		oc.HoraFin = horaComoDuracion(fin)
		resultado = append(resultado, oc)
	}
	return resultado, errorDeFilas(rows)
}

// ListarEquiposLibresEnLaSerie: los equipos libres en TODAS las ocurrencias
// que le quedan a la serie de ese grupo (RF-08.14).
//
// El NOT EXISTS se evalúa contra el conjunto entero de fechas y no contra una:
// un equipo que está libre en catorce martes y ocupado en el decimoquinto no
// sirve, porque el cambio en serie es todo o nada. Resolverlo acá y no
// preguntando fecha por fecha es lo que evita tantas idas a la base como
// fechas tenga la serie.
//
// Un grupo sin recurrencia devuelve los libres de su propia franja: es el
// mismo caso con una sola fecha.
func (r *PostgresRepo) ListarEquiposLibresEnLaSerie(ctx context.Context, grupoID string) ([]application.EquipoDisponible, error) {
	rows, err := r.db.Query(ctx, `
		WITH origen AS (
			SELECT fecha, hora_inicio, hora_fin, regla_recurrencia_id
			FROM reserva_grupo WHERE id = $1
		),
		ocurrencias AS (
			SELECT g.fecha, g.hora_inicio, g.hora_fin
			FROM reserva_grupo g, origen o
			WHERE g.estado IN ('CONFIRMADA','PARCIALMENTE_CANCELADA')
			  AND g.fecha >= o.fecha
			  AND (
				(o.regla_recurrencia_id IS NOT NULL AND g.regla_recurrencia_id = o.regla_recurrencia_id)
				OR g.id = $1
			  )
		)
		SELECT p.id, COALESCE(p.identificador, 0),
		       COALESCE(p.nombre, 'PC ' || p.identificador),
		       p.tipo,
		       COALESCE(c.id::text, ''), COALESCE(c.nombre, ''),
		       p.freezado, COALESCE(p.software_instalado, '')
		FROM equipo p
		LEFT JOIN carro c ON c.id = p.carro_id
		WHERE p.estado = 'DISPONIBLE'
		  AND p.dado_de_baja = false
		  AND p.reservable = true
		  AND NOT EXISTS (
			SELECT 1 FROM reserva res JOIN ocurrencias oc ON oc.fecha = res.fecha
			WHERE res.equipo_id = p.id
			  AND res.estado = 'CONFIRMADA'
			  AND tsrange(res.fecha + res.hora_inicio, res.fecha + res.hora_fin)
			      && tsrange(oc.fecha + oc.hora_inicio, oc.fecha + oc.hora_fin)
		  )
		ORDER BY p.carro_id IS NULL, c.nombre, p.identificador, p.nombre
	`, grupoID)
	if err != nil {
		return nil, fmt.Errorf("listando equipos libres en la serie: %w", err)
	}
	defer rows.Close()

	return escanearEquiposDisponibles(rows)
}

// ListarEquiposDisponiblesEn implementa RF-04.2: las PCs que se pueden
// reservar en una franja concreta. El NOT EXISTS usa el mismo criterio de
// solapamiento que la constraint EXCLUDE de la migración (tsrange con
// aritmética date+time), para que lo que se ofrece coincida exactamente
// con lo que la base va a aceptar después. Consulta pc/carro de solo
// lectura, mismo criterio que los validadores de este paquete.
func (r *PostgresRepo) ListarEquiposDisponiblesEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) ([]application.EquipoDisponible, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, COALESCE(p.identificador, 0),
		       COALESCE(p.nombre, 'PC ' || p.identificador),
		       p.tipo,
		       COALESCE(c.id::text, ''), COALESCE(c.nombre, ''),
		       p.freezado, COALESCE(p.software_instalado, '')
		FROM equipo p
		LEFT JOIN carro c ON c.id = p.carro_id
		WHERE p.estado = 'DISPONIBLE'
		  AND p.dado_de_baja = false
		  -- RF-03.16: un cargador no se planifica, se pide en el momento. Sin
		  -- este filtro aparecería en la lista cada vez que un docente va a
		  -- reservar, y la primera vez que alguien reserve uno sin querer hay
		  -- que explicarlo.
		  AND p.reservable = true
		  AND NOT EXISTS (
			SELECT 1 FROM reserva res
			WHERE res.equipo_id = p.id
			  AND res.estado = 'CONFIRMADA'
			  AND res.fecha = $1
			  AND tsrange(res.fecha + res.hora_inicio, res.fecha + res.hora_fin)
			      && tsrange($1::date + $2::time, $1::date + $3::time)
		  )
		-- Los equipos sueltos van al final: lo habitual es reservar PCs de
		-- un carro, y el proyector no tiene por qué colarse entre ellas.
		ORDER BY p.carro_id IS NULL, c.nombre, p.identificador, p.nombre
	`, fecha, duracionComoHora(horaInicio), duracionComoHora(horaFin))
	if err != nil {
		return nil, fmt.Errorf("listando equipos disponibles: %w", err)
	}
	defer rows.Close()

	return escanearEquiposDisponibles(rows)
}

func escanearEquiposDisponibles(rows pgx.Rows) ([]application.EquipoDisponible, error) {
	var resultado []application.EquipoDisponible
	for rows.Next() {
		var pc application.EquipoDisponible
		if err := rows.Scan(&pc.EquipoID, &pc.Identificador, &pc.Etiqueta, &pc.Tipo,
			&pc.CarroID, &pc.CarroNombre,
			&pc.Freezado, &pc.SoftwareInstalado); err != nil {
			return nil, fmt.Errorf("escaneando equipo disponible: %w", err)
		}
		resultado = append(resultado, pc)
	}
	return resultado, errorDeFilas(rows)
}
