// Package infrastructure implementa el repositorio del buzón contra
// PostgreSQL (pgx), más el adaptador que resuelve nombre y mail de quien
// escribió, que viven en la tabla `usuario` de auth.
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/shared/paginacion"
	"github.com/ramiro/sgrc/internal/sugerencias/application"
	"github.com/ramiro/sgrc/internal/sugerencias/domain"
)

const codigoTextoInvalido = "22P02"

func esIDInvalido(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoTextoInvalido
}

// NuevoID es el generador que se le inyecta al servicio.
func NuevoID() string { return uuid.NewString() }

type PostgresRepo struct {
	db *pgxpool.Pool
}

func NewPostgresRepo(db *pgxpool.Pool) *PostgresRepo { return &PostgresRepo{db: db} }

// columnas en un solo lugar: el orden lo comparten el SELECT y el escaneo,
// así que separarlos es la forma clásica de que un campo nuevo entre
// desalineado (ver el barrido de reservation, donde eso rompió el sistema
// entero en silencio).
const columnas = `id, usuario_id, tipo, texto, coalesce(pantalla, ''), coalesce(version, ''),
	estado, coalesce(respuesta, ''), respondida_por, respondida_en, creada_en`

func escanear(fila interface{ Scan(...any) error }) (*domain.Sugerencia, error) {
	var s domain.Sugerencia
	var tipo, estado string
	if err := fila.Scan(
		&s.ID, &s.UsuarioID, &tipo, &s.Texto, &s.Pantalla, &s.Version,
		&estado, &s.Respuesta, &s.RespondidaPor, &s.RespondidaEn, &s.CreadaEn,
	); err != nil {
		return nil, err
	}
	s.Tipo = domain.Tipo(tipo)
	s.Estado = domain.Estado(estado)
	return &s, nil
}

func (r *PostgresRepo) Crear(ctx context.Context, s *domain.Sugerencia) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO sugerencia (id, usuario_id, tipo, texto, pantalla, version, estado, creada_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, s.ID, s.UsuarioID, string(s.Tipo), s.Texto,
		nullSiVacio(s.Pantalla), nullSiVacio(s.Version), string(s.Estado), s.CreadaEn)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("insertando sugerencia: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarPorID(ctx context.Context, id string) (*domain.Sugerencia, error) {
	fila := r.db.QueryRow(ctx, `SELECT `+columnas+` FROM sugerencia WHERE id = $1`, id)
	s, err := escanear(fila)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrSugerenciaNoExist
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("buscando sugerencia: %w", err)
	}
	return s, nil
}

func (r *PostgresRepo) Guardar(ctx context.Context, s *domain.Sugerencia) error {
	_, err := r.db.Exec(ctx, `
		UPDATE sugerencia
		   SET estado = $2, respuesta = $3, respondida_por = $4, respondida_en = $5
		 WHERE id = $1
	`, s.ID, string(s.Estado), nullSiVacio(s.Respuesta), s.RespondidaPor, s.RespondidaEn)
	if err != nil {
		return fmt.Errorf("guardando sugerencia: %w", err)
	}
	return nil
}

func (r *PostgresRepo) ListarTodas(ctx context.Context, soloAbiertas bool, p paginacion.Pagina) ([]*domain.Sugerencia, int, error) {
	// El filtro va como parámetro y no concatenando SQL: una sola consulta
	// preparada sirve para los dos casos.
	var total int
	if err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM sugerencia WHERE ($1 = false OR estado = 'ABIERTA')`,
		soloAbiertas).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("contando sugerencias: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT `+columnas+` FROM sugerencia
		 WHERE ($1 = false OR estado = 'ABIERTA')
		 ORDER BY creada_en DESC
		 LIMIT $2 OFFSET $3
	`, soloAbiertas, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listando sugerencias: %w", err)
	}
	defer rows.Close()

	resultado, err := escanearTodas(rows)
	return resultado, total, err
}

func (r *PostgresRepo) ListarDeUsuario(ctx context.Context, usuarioID string, p paginacion.Pagina) ([]*domain.Sugerencia, int, error) {
	var total int
	if err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM sugerencia WHERE usuario_id = $1`, usuarioID).Scan(&total); err != nil {
		if esIDInvalido(err) {
			return nil, 0, application.ErrIDInvalido
		}
		return nil, 0, fmt.Errorf("contando sugerencias del usuario: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT `+columnas+` FROM sugerencia
		 WHERE usuario_id = $1
		 ORDER BY creada_en DESC
		 LIMIT $2 OFFSET $3
	`, usuarioID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listando sugerencias del usuario: %w", err)
	}
	defer rows.Close()

	resultado, err := escanearTodas(rows)
	return resultado, total, err
}

func (r *PostgresRepo) ContarAbiertas(ctx context.Context) (int, error) {
	var n int
	if err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM sugerencia WHERE estado = 'ABIERTA'`).Scan(&n); err != nil {
		return 0, fmt.Errorf("contando sugerencias abiertas: %w", err)
	}
	return n, nil
}

func escanearTodas(rows pgx.Rows) ([]*domain.Sugerencia, error) {
	var resultado []*domain.Sugerencia
	for rows.Next() {
		s, err := escanear(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando sugerencia: %w", err)
		}
		resultado = append(resultado, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterando sugerencias: %w", err)
	}
	return resultado, nil
}

// nullSiVacio deja NULL en vez de cadena vacía: en la base, "no se sabe de
// qué pantalla vino" y "vino de una pantalla que se llama ”" no son lo
// mismo.
func nullSiVacio(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── Adaptador hacia auth ───────────────────────────────────────────────

// UsuarioPostgres resuelve nombre y mail leyendo la tabla `usuario`.
//
// Consulta directa y no una llamada al servicio de auth: son dos columnas
// de una fila, y meter una dependencia entre módulos por eso ataría el
// arranque de uno al del otro (mismo criterio que ListadorAdmins).
type UsuarioPostgres struct {
	db *pgxpool.Pool
}

func NewUsuarioPostgres(db *pgxpool.Pool) *UsuarioPostgres { return &UsuarioPostgres{db: db} }

func (u *UsuarioPostgres) NombreYEmail(ctx context.Context, usuarioID string) (string, string, error) {
	var nombre, apellido, email string
	err := u.db.QueryRow(ctx,
		`SELECT nombre, apellido, email FROM usuario WHERE id = $1`, usuarioID).
		Scan(&nombre, &apellido, &email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", fmt.Errorf("no se encontró el usuario %s", usuarioID)
		}
		if esIDInvalido(err) {
			return "", "", application.ErrIDInvalido
		}
		return "", "", fmt.Errorf("buscando el usuario: %w", err)
	}
	return nombre + " " + apellido, email, nil
}
