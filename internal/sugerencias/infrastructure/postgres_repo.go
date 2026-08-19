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
const columnas = `id, usuario_id, tipo, asunto, coalesce(pantalla, ''), coalesce(version, ''),
	estado, creada_en, ultima_actividad_en`

func escanear(fila interface{ Scan(...any) error }) (*domain.Sugerencia, error) {
	var s domain.Sugerencia
	var tipo, estado string
	if err := fila.Scan(
		&s.ID, &s.UsuarioID, &tipo, &s.Asunto, &s.Pantalla, &s.Version,
		&estado, &s.CreadaEn, &s.UltimaActividadEn,
	); err != nil {
		return nil, err
	}
	s.Tipo = domain.Tipo(tipo)
	s.Estado = domain.Estado(estado)
	return &s, nil
}

// Crear escribe el hilo y su primer mensaje en una transacción: un hilo sin
// mensajes no es nada que alguien pueda leer ni contestar.
func (r *PostgresRepo) Crear(ctx context.Context, s *domain.Sugerencia) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciando transacción: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op si ya hizo Commit

	if _, err := tx.Exec(ctx, `
		INSERT INTO sugerencia (id, usuario_id, tipo, asunto, pantalla, version, estado, creada_en, ultima_actividad_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, s.ID, s.UsuarioID, string(s.Tipo), s.Asunto,
		nullSiVacio(s.Pantalla), nullSiVacio(s.Version), string(s.Estado),
		s.CreadaEn, s.UltimaActividadEn); err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("insertando sugerencia: %w", err)
	}

	for _, m := range s.Mensajes {
		if err := insertarMensaje(ctx, tx, s.ID, m); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmando la sugerencia: %w", err)
	}
	return nil
}

// AgregarMensaje suma una intervención y mueve el hilo al tope de la bandeja,
// también en una transacción: un mensaje que no actualiza la actividad queda
// enterrado abajo y nadie lo contesta.
func (r *PostgresRepo) AgregarMensaje(ctx context.Context, s *domain.Sugerencia, m domain.Mensaje) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciando transacción: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op si ya hizo Commit

	if err := insertarMensaje(ctx, tx, s.ID, m); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE sugerencia SET estado = $2, ultima_actividad_en = $3 WHERE id = $1`,
		s.ID, string(s.Estado), s.UltimaActividadEn); err != nil {
		return fmt.Errorf("actualizando el hilo: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmando el mensaje: %w", err)
	}
	return nil
}

func insertarMensaje(ctx context.Context, tx pgx.Tx, sugerenciaID string, m domain.Mensaje) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO sugerencia_mensaje (id, sugerencia_id, autor_id, de_admin, texto, escrito_en)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, m.ID, sugerenciaID, m.AutorID, m.DeAdmin, m.Texto, m.EscritoEn); err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("insertando mensaje: %w", err)
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

	porHilo, err := r.mensajesDe(ctx, []string{s.ID})
	if err != nil {
		return nil, err
	}
	s.Mensajes = porHilo[s.ID]
	return s, nil
}

// GuardarEstado escribe solo lo que cambia al resolver o reabrir: los
// mensajes no se editan nunca.
func (r *PostgresRepo) GuardarEstado(ctx context.Context, s *domain.Sugerencia) error {
	_, err := r.db.Exec(ctx,
		`UPDATE sugerencia SET estado = $2, ultima_actividad_en = $3 WHERE id = $1`,
		s.ID, string(s.Estado), s.UltimaActividadEn)
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
		 ORDER BY ultima_actividad_en DESC
		 LIMIT $2 OFFSET $3
	`, soloAbiertas, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listando sugerencias: %w", err)
	}
	defer rows.Close()

	resultado, err := r.escanearConMensajes(ctx, rows)
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
		 ORDER BY ultima_actividad_en DESC
		 LIMIT $2 OFFSET $3
	`, usuarioID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listando sugerencias del usuario: %w", err)
	}
	defer rows.Close()

	resultado, err := r.escanearConMensajes(ctx, rows)
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

// escanearConMensajes trae los hilos de la página y DESPUÉS todos sus
// mensajes en una sola consulta más. Con una consulta por hilo, una bandeja
// de veinte serían veintiuna idas a la base para dibujar una lista.
func (r *PostgresRepo) escanearConMensajes(ctx context.Context, rows pgx.Rows) ([]*domain.Sugerencia, error) {
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
	if len(resultado) == 0 {
		return resultado, nil
	}

	ids := make([]string, len(resultado))
	for i, s := range resultado {
		ids[i] = s.ID
	}
	porHilo, err := r.mensajesDe(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, s := range resultado {
		s.Mensajes = porHilo[s.ID]
	}
	return resultado, nil
}

func (r *PostgresRepo) mensajesDe(ctx context.Context, sugerenciaIDs []string) (map[string][]domain.Mensaje, error) {
	rows, err := r.db.Query(ctx, `
		SELECT sugerencia_id, id, autor_id, de_admin, texto, escrito_en
		  FROM sugerencia_mensaje
		 WHERE sugerencia_id = ANY($1)
		 ORDER BY escrito_en, id
	`, sugerenciaIDs)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("leyendo los mensajes: %w", err)
	}
	defer rows.Close()

	porHilo := make(map[string][]domain.Mensaje, len(sugerenciaIDs))
	for rows.Next() {
		var hiloID string
		var m domain.Mensaje
		if err := rows.Scan(&hiloID, &m.ID, &m.AutorID, &m.DeAdmin, &m.Texto, &m.EscritoEn); err != nil {
			return nil, fmt.Errorf("escaneando mensaje: %w", err)
		}
		porHilo[hiloID] = append(porHilo[hiloID], m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterando mensajes: %w", err)
	}
	return porHilo, nil
}

// nullSiVacio deja NULL en vez de cadena vacía: en la base, "no se sabe de
// qué pantalla vino" y "vino de una pantalla que se llama ”" no son lo mismo.
func nullSiVacio(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── Adaptador hacia auth ───────────────────────────────────────────────

// UsuarioPostgres resuelve nombre y mail leyendo la tabla `usuario`.
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
