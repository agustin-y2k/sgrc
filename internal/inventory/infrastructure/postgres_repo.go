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

// consultor es el subconjunto de pgx que usan las consultas de este paquete:
// lo cumplen tanto el pool como una transacción, que es lo que permite que las
// mismas funciones sirvan dentro y fuera de una.
type consultor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type PostgresRepo struct {
	db consultor
	// pool es nil cuando este repo ya está atado a una transacción — es lo que
	// hace que EnTransaccion no intente abrir una anidada.
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{db: pool, pool: pool}
}

// EnTransaccion corre fn con un repo atado a una única transacción: o se aplica
// todo, o no queda nada.
//
// Es el mismo patrón que usa reservation desde siempre, portado acá porque
// inventory tiene cuatro operaciones que escriben de a muchas filas —dar de
// alta licencias en un lote de equipos, renovarlas, marcar preferencias, y el
// barrido que anota los avisos ya enviados— y ninguna era atómica. Una que
// falla en la máquina 40 de 60 dejaba las 39 anteriores escritas y devolvía un
// error: el Admin no tenía forma de saber qué había entrado, y el único camino
// era revisar equipo por equipo.
func (r *PostgresRepo) EnTransaccion(ctx context.Context, fn func(application.Repo) error) error {
	if r.pool == nil {
		// Ya venimos dentro de una transacción: se reusa la misma en vez de
		// anidar, para que el alcance del commit siga siendo el de afuera.
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
	_, err := r.db.Exec(ctx,
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
	row := r.db.QueryRow(ctx, `SELECT `+columnasCarro+` FROM carro WHERE id = $1`, id)
	return escanearCarro(row)
}

// columnasCarro incluye las dos de la baja lógica: un carro retirado se lee
// igual, sólo que no aparece en los listados por defecto.
const columnasCarro = `id, nombre, descripcion, dado_de_baja, fecha_baja`

func escanearCarro(row pgx.Row) (*domain.Carro, error) {
	var c domain.Carro
	var descripcion *string
	if err := row.Scan(&c.ID, &c.Nombre, &descripcion, &c.DadoDeBaja, &c.FechaBaja); err != nil {
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
	tag, err := r.db.Exec(ctx,
		`UPDATE carro SET nombre=$2, descripcion=$3, dado_de_baja=$4, fecha_baja=$5 WHERE id=$1`,
		c.ID, c.Nombre, c.Descripcion, c.DadoDeBaja, c.FechaBaja)
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

// ListarCarros devuelve por defecto sólo los que siguen en circulación: un
// carro retirado no tiene nada que hacer en el selector donde se elige dónde va
// un equipo.
//
// `incluirRetirados` es la excepción, y existe por una sola razón: para
// reactivar un carro hay que poder verlo primero. Los retirados van al final y
// no mezclados por nombre, para que la lista habitual no cambie de orden cuando
// se los pide.
func (r *PostgresRepo) ListarCarros(ctx context.Context, incluirRetirados bool) ([]*domain.Carro, error) {
	filtro := "WHERE dado_de_baja = false"
	if incluirRetirados {
		filtro = ""
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+columnasCarro+` FROM carro `+filtro+` ORDER BY dado_de_baja, nombre`)
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
