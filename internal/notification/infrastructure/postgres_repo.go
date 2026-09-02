// Package infrastructure implementa application.Repo de notification
// contra PostgreSQL real (pgx), además del adaptador ListadorAdmins.
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/notification/application"
	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
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

var _ application.Repo = (*PostgresRepo)(nil)

type PostgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

const columnasNotificacion = `id, usuario_id, reserva_id, sobre_usuario_id, mensaje, tipo, estado, creada_en, leida_en`

func (r *PostgresRepo) Crear(ctx context.Context, n *domain.Notificacion) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notificacion (id, usuario_id, reserva_id, sobre_usuario_id, mensaje, tipo, estado, creada_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, n.ID, n.UsuarioID, n.ReservaID, n.SobreUsuarioID, n.Mensaje, string(n.Tipo), string(n.Estado), n.CreadaEn)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando notificación: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarPorID(ctx context.Context, id string) (*domain.Notificacion, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+columnasNotificacion+` FROM notificacion WHERE id = $1`, id)
	return escanearNotificacion(row)
}

func escanearNotificacion(row pgx.Row) (*domain.Notificacion, error) {
	var n domain.Notificacion
	var estadoStr string

	var tipoStr string
	err := row.Scan(&n.ID, &n.UsuarioID, &n.ReservaID, &n.SobreUsuarioID, &n.Mensaje, &tipoStr, &estadoStr, &n.CreadaEn, &n.LeidaEn)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrNotificacionNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando notificación: %w", err)
	}

	estado, err := domain.ParseEstado(estadoStr)
	if err != nil {
		return nil, fmt.Errorf("estado inválido en la base para notificación %s: %w", n.ID, err)
	}
	n.Estado = estado

	tipo, err := domain.ParseTipo(tipoStr)
	if err != nil {
		return nil, fmt.Errorf("tipo inválido en la base para notificación %s: %w", n.ID, err)
	}
	n.Tipo = tipo

	return &n, nil
}

// escanearNotificacionConTotal es escanearNotificacion más la columna que
// agrega COUNT(*) OVER() al listado paginado.
func escanearNotificacionConTotal(row pgx.Row, total *int) (*domain.Notificacion, error) {
	var n domain.Notificacion
	var estadoStr string

	var tipoStr string
	if err := row.Scan(&n.ID, &n.UsuarioID, &n.ReservaID, &n.SobreUsuarioID, &n.Mensaje, &tipoStr, &estadoStr, &n.CreadaEn, &n.LeidaEn, total); err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando notificación: %w", err)
	}

	estado, err := domain.ParseEstado(estadoStr)
	if err != nil {
		return nil, fmt.Errorf("estado inválido en la base para notificación %s: %w", n.ID, err)
	}
	n.Estado = estado

	tipo, err := domain.ParseTipo(tipoStr)
	if err != nil {
		return nil, fmt.Errorf("tipo inválido en la base para notificación %s: %w", n.ID, err)
	}
	n.Tipo = tipo

	return &n, nil
}

func (r *PostgresRepo) Guardar(ctx context.Context, n *domain.Notificacion) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE notificacion SET estado = $2, leida_en = $3 WHERE id = $1`,
		n.ID, string(n.Estado), n.LeidaEn)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando notificación: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrNotificacionNoEncontrada
	}
	return nil
}

// ListarPorUsuario devuelve una página de las notificaciones de un usuario y
// el total que matchean el filtro.
func (r *PostgresRepo) ListarPorUsuario(ctx context.Context, usuarioID string, filtroEstado *domain.Estado, pagina paginacion.Pagina) ([]*domain.Notificacion, int, error) {
	desde := ` FROM notificacion WHERE usuario_id = $1`
	args := []any{usuarioID}

	if filtroEstado != nil {
		args = append(args, string(*filtroEstado))
		desde += fmt.Sprintf(" AND estado = $%d", len(args))
	}

	// creada_en no es única (dos avisos de la misma cascada se escriben en el
	// mismo instante), así que el desempate por id evita que una notificación
	// salte de página entre dos consultas.
	query := `SELECT ` + columnasNotificacion + `, COUNT(*) OVER() AS total` + desde +
		" ORDER BY creada_en DESC, id"

	argsPagina := append(append([]any{}, args...), pagina.Limit(), pagina.Offset())
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(argsPagina)-1, len(argsPagina))

	rows, err := r.pool.Query(ctx, query, argsPagina...)
	if err != nil {
		if esIDInvalido(err) {
			return nil, 0, application.ErrIDInvalido
		}
		return nil, 0, fmt.Errorf("listando notificaciones: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Notificacion
	total := 0
	for rows.Next() {
		n, err := escanearNotificacionConTotal(rows, &total)
		if err != nil {
			return nil, 0, fmt.Errorf("escaneando fila de notificación: %w", err)
		}
		resultado = append(resultado, n)
	}
	if err := errorDeFilas(rows); err != nil {
		return nil, 0, err
	}

	// Una página más allá del final no trae ventana de la que leer el total, y
	// sin él la campana mostraría "0 notificaciones" con la primera página
	// llena.
	if len(resultado) == 0 && pagina.Offset() > 0 {
		if err := r.pool.QueryRow(ctx, "SELECT COUNT(*)"+desde, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("contando notificaciones: %w", err)
		}
	}

	return resultado, total, nil
}

// ListarNoLeidasSobreUsuario: los avisos sin leer que hablan de una persona,
// sin importar a quién le llegaron — un pendiente de aprobación le llega a
// todos los Admin y se cierra para todos a la vez.
func (r *PostgresRepo) ListarNoLeidasSobreUsuario(ctx context.Context, sobreUsuarioID string, tipo domain.Tipo) ([]*domain.Notificacion, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+columnasNotificacion+` FROM notificacion
		 WHERE sobre_usuario_id = $1 AND tipo = $2 AND estado = 'NO_LEIDA'`,
		sobreUsuarioID, string(tipo))
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando avisos sobre el usuario: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Notificacion
	for rows.Next() {
		n, err := escanearNotificacion(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando aviso: %w", err)
		}
		resultado = append(resultado, n)
	}
	return resultado, errorDeFilas(rows)
}

// MarcarTodasLeidasDe: un solo UPDATE para todas las NO_LEIDA del usuario.
// MarcarLeidasPorTipo cierra de una todas las NO_LEIDA de ese tipo, sin
// importar de quién sean: un UPDATE y no un recorrido, porque con cuatro Admin
// y varios días sin mirar la campana son varias decenas de filas y no hay nada
// que decidir fila por fila.
func (r *PostgresRepo) MarcarLeidasPorTipo(ctx context.Context, tipo domain.Tipo, ahora time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notificacion SET estado = 'LEIDA', leida_en = $2
		WHERE tipo = $1 AND estado = 'NO_LEIDA'
	`, string(tipo), ahora)
	if err != nil {
		return 0, fmt.Errorf("cerrando las notificaciones de tipo %s: %w", tipo, err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *PostgresRepo) MarcarTodasLeidasDe(ctx context.Context, usuarioID string, ahora time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notificacion SET estado = 'LEIDA', leida_en = $2
		WHERE usuario_id = $1 AND estado = 'NO_LEIDA'
	`, usuarioID, ahora)
	if err != nil {
		if esIDInvalido(err) {
			return 0, application.ErrIDInvalido
		}
		return 0, fmt.Errorf("marcando todas las notificaciones como leídas: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
