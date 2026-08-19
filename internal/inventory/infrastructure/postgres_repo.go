// Package infrastructure implementa application.Repo de inventory contra
// PostgreSQL real (pgx), además del stub de ValidadorReservas.
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

const (
	codigoViolacionUnica = "23505"
	codigoTextoInvalido  = "22P02" // ver el mismo bug encontrado en academic
)

var _ application.Repo = (*PostgresRepo)(nil)

type PostgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

func esViolacionUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoViolacionUnica
}

// nombreDeConstraint devuelve qué constraint se violó, o "" si no fue una
// violación de Postgres.
func nombreDeConstraint(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
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

// errorDeFilas centraliza el chequeo de rows.Err(): pool.Query() no siempre
// devuelve el error de sintaxis inmediatamente — a veces aparece recién en
// rows.Err(), después del loop.
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

// ── Carro ───────────────────────────────────────────────────────────────

func (r *PostgresRepo) CrearCarro(ctx context.Context, c *domain.Carro) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO carro (id, nombre, descripcion) VALUES ($1, $2, $3)`,
		c.ID, c.Nombre, c.Descripcion)
	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrNombreCarroDuplicado
		}
		return fmt.Errorf("creando carro: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarCarroPorID(ctx context.Context, id string) (*domain.Carro, error) {
	row := r.pool.QueryRow(ctx, `SELECT id, nombre, descripcion FROM carro WHERE id = $1`, id)
	return escanearCarro(row)
}

func escanearCarro(row pgx.Row) (*domain.Carro, error) {
	var c domain.Carro
	var descripcion *string
	if err := row.Scan(&c.ID, &c.Nombre, &descripcion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrCarroNoEncontrado
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando carro: %w", err)
	}
	if descripcion != nil {
		c.Descripcion = *descripcion
	}
	return &c, nil
}

func (r *PostgresRepo) GuardarCarro(ctx context.Context, c *domain.Carro) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE carro SET nombre=$2, descripcion=$3 WHERE id=$1`,
		c.ID, c.Nombre, c.Descripcion)
	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrNombreCarroDuplicado
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando carro: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrCarroNoEncontrado
	}
	return nil
}

func (r *PostgresRepo) ListarCarros(ctx context.Context) ([]*domain.Carro, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, nombre, descripcion FROM carro ORDER BY nombre`)
	if err != nil {
		return nil, fmt.Errorf("listando carros: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Carro
	for rows.Next() {
		c, err := escanearCarro(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de carro: %w", err)
		}
		resultado = append(resultado, c)
	}
	return resultado, errorDeFilas(rows)
}
