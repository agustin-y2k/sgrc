// Package infrastructure implementa application.Repo de availability contra
// PostgreSQL real (pgx), además del adaptador ListadorAdmins hacia auth
// (RF-07, ver docs/07-modelo-datos.md para las tablas horario_admin y
// horario_admin_excepcion).
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/availability/application"
	"github.com/ramiro/sgrc/internal/availability/domain"
)

const codigoTextoInvalido = "22P02"

func esIDInvalido(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoTextoInvalido
}

func errorDeFilas(rows pgx.Rows) error {
	err := rows.Err()
	if err == nil {
		return nil
	}
	if esIDInvalido(err) {
		return application.ErrIDInvalido
	}
	return fmt.Errorf("iterando filas: %w", err)
}

// horaComoDuracion / duracionComoHora convierten entre time.Duration (offset
// desde medianoche, el tipo que usa domain) y time.Time (lo que pgx
// espera/devuelve para una columna TIME) — mismo criterio que
// reservation/infrastructure.
func horaComoDuracion(t time.Time) time.Duration {
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute + time.Duration(t.Second())*time.Second
}

func duracionComoHora(d time.Duration) time.Time {
	// Fecha de referencia neutra — pgx solo mira la hora al escribir a una
	// columna TIME, ignora la fecha de este time.Time.
	return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).Add(d)
}

var _ application.Repo = (*PostgresRepo)(nil)

type PostgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

// ── BloqueHorario (horario_admin) ──────────────────────────────────────

const columnasBloque = `id, usuario_id, dia_semana, hora_inicio, hora_fin`

func (r *PostgresRepo) ListarBloquesDeUsuario(ctx context.Context, usuarioID string) ([]*domain.BloqueHorario, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+columnasBloque+` FROM horario_admin WHERE usuario_id = $1 ORDER BY dia_semana, hora_inicio`,
		usuarioID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando bloques de horario: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.BloqueHorario
	for rows.Next() {
		b, err := escanearBloque(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando bloque de horario: %w", err)
		}
		resultado = append(resultado, b)
	}
	return resultado, errorDeFilas(rows)
}

// ListarBloquesDeUsuarios trae los bloques de varios Admin en una sola
// consulta, en vez de una por Admin (ver el puerto).
func (r *PostgresRepo) ListarBloquesDeUsuarios(ctx context.Context, usuarioIDs []string) (map[string][]*domain.BloqueHorario, error) {
	resultado := make(map[string][]*domain.BloqueHorario, len(usuarioIDs))
	if len(usuarioIDs) == 0 {
		return resultado, nil
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+columnasBloque+` FROM horario_admin WHERE usuario_id = ANY($1) ORDER BY usuario_id, dia_semana, hora_inicio`,
		usuarioIDs)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando bloques de horario: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		b, err := escanearBloque(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando bloque de horario: %w", err)
		}
		resultado[b.UsuarioID] = append(resultado[b.UsuarioID], b)
	}
	return resultado, errorDeFilas(rows)
}

func escanearBloque(row pgx.Row) (*domain.BloqueHorario, error) {
	var b domain.BloqueHorario
	var diaStr string
	var horaInicio, horaFin time.Time

	err := row.Scan(&b.ID, &b.UsuarioID, &diaStr, &horaInicio, &horaFin)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrBloqueNoEncontrado
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando bloque: %w", err)
	}

	dia, err := domain.ParseDiaSemana(diaStr)
	if err != nil {
		return nil, fmt.Errorf("día de semana inválido en la base para bloque %s: %w", b.ID, err)
	}
	b.DiaSemana = dia
	b.HoraInicio = horaComoDuracion(horaInicio)
	b.HoraFin = horaComoDuracion(horaFin)

	return &b, nil
}

func (r *PostgresRepo) CrearBloque(ctx context.Context, b *domain.BloqueHorario) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO horario_admin (id, usuario_id, dia_semana, hora_inicio, hora_fin)
		VALUES ($1, $2, $3, $4, $5)
	`, b.ID, b.UsuarioID, string(b.DiaSemana), duracionComoHora(b.HoraInicio), duracionComoHora(b.HoraFin))
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando bloque de horario: %w", err)
	}
	return nil
}

// BuscarBloqueDeUsuario acota por (id, usuario_id) en la propia query — un
// bloque de otro usuario da el mismo ErrBloqueNoEncontrado que un ID
// inexistente (ver application/ports.go sobre el criterio de titularidad).
func (r *PostgresRepo) BuscarBloqueDeUsuario(ctx context.Context, id, usuarioID string) (*domain.BloqueHorario, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+columnasBloque+` FROM horario_admin WHERE id = $1 AND usuario_id = $2`,
		id, usuarioID)
	return escanearBloque(row)
}

// GuardarBloque actualiza acotando por (id, usuario_id) — si b.UsuarioID no
// coincide con el dueño real de esa fila, RowsAffected queda en 0 y se
// devuelve ErrBloqueNoEncontrado, igual que un ID inexistente.
func (r *PostgresRepo) GuardarBloque(ctx context.Context, b *domain.BloqueHorario) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE horario_admin SET dia_semana = $3, hora_inicio = $4, hora_fin = $5 WHERE id = $1 AND usuario_id = $2`,
		b.ID, b.UsuarioID, string(b.DiaSemana), duracionComoHora(b.HoraInicio), duracionComoHora(b.HoraFin))
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando bloque de horario: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrBloqueNoEncontrado
	}
	return nil
}

func (r *PostgresRepo) EliminarBloqueDeUsuario(ctx context.Context, id, usuarioID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM horario_admin WHERE id = $1 AND usuario_id = $2`, id, usuarioID)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("eliminando bloque de horario: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrBloqueNoEncontrado
	}
	return nil
}

// ── Excepcion (horario_admin_excepcion) ────────────────────────────────

const columnasExcepcion = `id, usuario_id, fecha, tipo, hora_inicio, hora_fin, motivo`

func (r *PostgresRepo) BuscarExcepcionDeFecha(ctx context.Context, usuarioID string, fecha time.Time) (*domain.Excepcion, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+columnasExcepcion+` FROM horario_admin_excepcion WHERE usuario_id = $1 AND fecha = $2`,
		usuarioID, fecha)
	e, err := escanearExcepcion(row)
	if errors.Is(err, application.ErrExcepcionNoEncontrada) {
		// No tener excepción cargada para una fecha es el caso normal
		// (RF-07.4 es opcional) — no es un error para el caller.
		return nil, nil
	}
	return e, err
}

// BuscarExcepcionesDeFecha es la versión en lote.
func (r *PostgresRepo) BuscarExcepcionesDeFecha(ctx context.Context, usuarioIDs []string, fecha time.Time) (map[string]*domain.Excepcion, error) {
	resultado := make(map[string]*domain.Excepcion, len(usuarioIDs))
	if len(usuarioIDs) == 0 {
		return resultado, nil
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+columnasExcepcion+` FROM horario_admin_excepcion WHERE usuario_id = ANY($1) AND fecha = $2`,
		usuarioIDs, fecha)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("buscando excepciones de la fecha: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		e, err := escanearExcepcion(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando excepción: %w", err)
		}
		// UNIQUE(usuario_id, fecha) garantiza una sola por Admin y día.
		resultado[e.UsuarioID] = e
	}
	return resultado, errorDeFilas(rows)
}

func escanearExcepcion(row pgx.Row) (*domain.Excepcion, error) {
	var e domain.Excepcion
	var tipoStr string
	var horaInicio, horaFin *time.Time

	err := row.Scan(&e.ID, &e.UsuarioID, &e.Fecha, &tipoStr, &horaInicio, &horaFin, &e.Motivo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrExcepcionNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando excepción: %w", err)
	}

	tipo, err := domain.ParseTipoExcepcion(tipoStr)
	if err != nil {
		return nil, fmt.Errorf("tipo de excepción inválido en la base para %s: %w", e.ID, err)
	}
	e.Tipo = tipo
	if horaInicio != nil {
		d := horaComoDuracion(*horaInicio)
		e.HoraInicio = &d
	}
	if horaFin != nil {
		d := horaComoDuracion(*horaFin)
		e.HoraFin = &d
	}

	return &e, nil
}

// GuardarExcepcion hace upsert por (usuario_id, fecha) — la UNIQUE de la
// tabla — así volver a postear para la misma fecha REEMPLAZA la anterior por
// completo (incluyendo el ID) en vez de violar la constraint (RF-07.4,
// docs/08-api-spec.yaml: "reemplaza la excepción anterior si ya existía una
// para esa fecha").
func (r *PostgresRepo) GuardarExcepcion(ctx context.Context, e *domain.Excepcion) error {
	var horaInicio, horaFin *time.Time
	if e.HoraInicio != nil {
		t := duracionComoHora(*e.HoraInicio)
		horaInicio = &t
	}
	if e.HoraFin != nil {
		t := duracionComoHora(*e.HoraFin)
		horaFin = &t
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO horario_admin_excepcion (id, usuario_id, fecha, tipo, hora_inicio, hora_fin, motivo)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (usuario_id, fecha) DO UPDATE SET
			id = EXCLUDED.id, tipo = EXCLUDED.tipo, hora_inicio = EXCLUDED.hora_inicio,
			hora_fin = EXCLUDED.hora_fin, motivo = EXCLUDED.motivo
	`, e.ID, e.UsuarioID, e.Fecha, string(e.Tipo), horaInicio, horaFin, e.Motivo)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("guardando excepción: %w", err)
	}
	return nil
}

// ── BloqueJornada (jornada_institucion) ──────────────────────────────── Sin
// usuario_id: la jornada describe a la institución, no a una persona.

const columnasJornada = `id, dia_semana, hora_inicio, hora_fin`

// ListarJornada trae la jornada completa.
func (r *PostgresRepo) ListarJornada(ctx context.Context) ([]*domain.BloqueJornada, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+columnasJornada+` FROM jornada_institucion ORDER BY dia_semana, hora_inicio`)
	if err != nil {
		return nil, fmt.Errorf("listando la jornada de la institución: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.BloqueJornada
	for rows.Next() {
		b, err := escanearBloqueJornada(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando bloque de jornada: %w", err)
		}
		resultado = append(resultado, b)
	}
	return resultado, errorDeFilas(rows)
}

// ReemplazarJornada borra la jornada vieja, escribe la nueva y marca que la
// institución ya decidió, en una sola transacción.
//
// El borrar-y-escribir no es pereza frente a un diff: los tramos no tienen
// identidad propia para el que los mira —nadie dice "el tramo tal"—, así que
// calcular cuáles sobreviven sería trabajo para que las filas conserven un
// UUID que nadie va a volver a nombrar.
//
// Lo que sí importa es que sea atómico. A mitad de camino la jornada está
// incompleta, y PermiteReserva la lee para aceptar o rechazar reservas: un
// docente que guarda justo en ese momento recibiría un rechazo por un horario
// que la escuela sí abre.
func (r *PostgresRepo) ReemplazarJornada(ctx context.Context, bloques []*domain.BloqueJornada) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciando la transacción de la jornada: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op si ya se hizo Commit

	if _, err := tx.Exec(ctx, `DELETE FROM jornada_institucion`); err != nil {
		return fmt.Errorf("vaciando la jornada anterior: %w", err)
	}

	for _, b := range bloques {
		if _, err := tx.Exec(ctx,
			`INSERT INTO jornada_institucion (`+columnasJornada+`) VALUES ($1, $2, $3, $4)`,
			b.ID, string(b.DiaSemana), duracionComoHora(b.HoraInicio), duracionComoHora(b.HoraFin),
		); err != nil {
			if esIDInvalido(err) {
				return application.ErrIDInvalido
			}
			return fmt.Errorf("insertando bloque de jornada: %w", err)
		}
	}

	// La marca viaja en la misma transacción que los tramos: si se guardara
	// aparte y fallara, la escuela habría declarado su jornada y el sistema
	// seguiría preguntándole cuál es.
	if _, err := tx.Exec(ctx,
		`UPDATE configuracion_institucion SET jornada_definida = TRUE`); err != nil {
		return fmt.Errorf("marcando la jornada como definida: %w", err)
	}

	return tx.Commit(ctx)
}

// JornadaDefinida lee la bandera de la fila única de configuración.
func (r *PostgresRepo) JornadaDefinida(ctx context.Context) (bool, error) {
	var definida bool
	err := r.pool.QueryRow(ctx,
		`SELECT jornada_definida FROM configuracion_institucion`).Scan(&definida)
	if err != nil {
		return false, fmt.Errorf("leyendo si la jornada ya fue definida: %w", err)
	}
	return definida, nil
}

func escanearBloqueJornada(row pgx.Row) (*domain.BloqueJornada, error) {
	var b domain.BloqueJornada
	var diaStr string
	var horaInicio, horaFin time.Time

	if err := row.Scan(&b.ID, &diaStr, &horaInicio, &horaFin); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrBloqueNoEncontrado
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, err
	}

	b.DiaSemana = domain.DiaSemana(diaStr)
	b.HoraInicio = horaComoDuracion(horaInicio)
	b.HoraFin = horaComoDuracion(horaFin)
	return &b, nil
}
