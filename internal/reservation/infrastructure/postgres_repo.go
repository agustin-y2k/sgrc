// Package infrastructure implementa application.Repo de reservation contra
// PostgreSQL real (pgx), además de los tres adaptadores hacia academic,
// inventory y auth.
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
)

const (
	codigoTextoInvalido = "22P02"

	// codigoViolacionExclusion: SQLSTATE 23P01 — "exclusion_violation".
	codigoViolacionExclusion = "23P01"
)

var _ application.Repo = (*PostgresRepo)(nil)

// consultor es el subconjunto de pgx que usan las consultas de este paquete.
type consultor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type PostgresRepo struct {
	db consultor
	// pool es nil cuando este repo ya está atado a una transacción — es
	// lo que hace que EnTransaccion no intente abrir una anidada.
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{db: pool, pool: pool}
}

// EnTransaccion corre fn con un repo atado a una única transacción: o se
// aplica todo, o no queda nada.
func (r *PostgresRepo) EnTransaccion(ctx context.Context, fn func(application.Repo) error) error {
	if r.pool == nil {
		// Ya venimos dentro de una transacción: se reusa la misma en vez de anidar,
		// para que el alcance del commit siga siendo el de afuera.
		return fn(r)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciando transacción: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op si ya se hizo Commit

	if err := fn(&PostgresRepo{db: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func esIDInvalido(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoTextoInvalido
}

// codigoViolacionFK: SQLSTATE 23503 — "foreign_key_violation".
const codigoViolacionFK = "23503"

func esViolacionFK(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoViolacionFK
}

func esViolacionExclusion(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoViolacionExclusion
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
// espera/devuelve para una columna TIME).
func horaComoDuracion(t time.Time) time.Duration {
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute + time.Duration(t.Second())*time.Second
}

func duracionComoHora(d time.Duration) time.Time {
	// Se usa una fecha de referencia neutra — pgx solo mira la hora al
	// escribir a una columna TIME, ignora la fecha de este time.Time.
	return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).Add(d)
}

// ── ReservaGrupo ────────────────────────────────────────────────────────

func (r *PostgresRepo) CrearReservaGrupo(ctx context.Context, g *domain.ReservaGrupo) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO reserva_grupo (id, materia_id, creado_por, nombre_docente_snapshot, fecha, hora_inicio, hora_fin, estado, regla_recurrencia_id, creada_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, g.ID, g.MateriaID, g.CreadoPor, g.NombreDocenteSnapshot, g.Fecha,
		duracionComoHora(g.HoraInicio), duracionComoHora(g.HoraFin),
		string(g.Estado), g.ReglaRecurrenciaID, g.CreadaEn)
	if err != nil {
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando reserva grupo: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarReservaGrupoPorID(ctx context.Context, id string) (*domain.ReservaGrupo, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, materia_id, creado_por, nombre_docente_snapshot, fecha, hora_inicio, hora_fin, estado, regla_recurrencia_id, creada_en
		FROM reserva_grupo WHERE id = $1
	`, id)
	return escanearReservaGrupo(row)
}

func escanearReservaGrupo(row pgx.Row) (*domain.ReservaGrupo, error) {
	var g domain.ReservaGrupo
	var horaInicio, horaFin time.Time
	var estadoStr string

	err := row.Scan(&g.ID, &g.MateriaID, &g.CreadoPor, &g.NombreDocenteSnapshot, &g.Fecha,
		&horaInicio, &horaFin, &estadoStr, &g.ReglaRecurrenciaID, &g.CreadaEn)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrReservaGrupoNoEncontrado
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando reserva grupo: %w", err)
	}

	estado, err := domain.ParseEstadoReservaGrupo(estadoStr)
	if err != nil {
		return nil, fmt.Errorf("estado inválido en la base para reserva_grupo %s: %w", g.ID, err)
	}
	g.Estado = estado
	g.HoraInicio = horaComoDuracion(horaInicio)
	g.HoraFin = horaComoDuracion(horaFin)

	return &g, nil
}

func (r *PostgresRepo) GuardarReservaGrupo(ctx context.Context, g *domain.ReservaGrupo) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE reserva_grupo SET estado = $2 WHERE id = $1`,
		g.ID, string(g.Estado))
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando reserva grupo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrReservaGrupoNoEncontrado
	}
	return nil
}
