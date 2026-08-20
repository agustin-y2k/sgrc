// Package infrastructure implementa application.Repo contra PostgreSQL
// real (pgx). Es la única capa de internal/auth que conoce SQL.
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/auth/application"
	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// textoOpcional convierte "" a NULL: la columna es nullable y guardar la
// cadena vacía haría que "no lo declaró" y "lo dejó en blanco" se vean
// distinto en la base sin serlo.
func textoOpcional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func textoDe(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// codigoViolacionUnica es el código de error de Postgres para una violación
// de constraint UNIQUE (email duplicado, en nuestro caso).
const codigoViolacionUnica = "23505"

// codigoTextoInvalido: SQLSTATE 22P02 — "invalid input syntax for type X".
const codigoTextoInvalido = "22P02"

// PostgresRepo implementa application.Repo.
var _ application.Repo = (*PostgresRepo)(nil)

// consultor es el subconjunto de pgx que usan las consultas de este paquete —
// lo satisfacen tanto *pgxpool.Pool como pgx.Tx, que es lo que permite reusar
// los mismos métodos dentro y fuera de una transacción.
type consultor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type PostgresRepo struct {
	db consultor
	// pool es nil cuando el repo ya está atado a una transacción.
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{db: pool, pool: pool}
}

// EnTransaccion corre fn dentro de una única transacción.
func (r *PostgresRepo) EnTransaccion(ctx context.Context, fn func(application.Repo) error) error {
	if r.pool == nil {
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
// devuelve el error de sintaxis inmediatamente — a veces aparece recién acá,
// después del loop.
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

const columnasUsuario = `id, nombre, apellido, email, password_hash, debe_cambiar_password, rol, estado, fecha_registro, fecha_aprobacion, aprobado_por, curso_solicitado, materia_solicitada, rol_solicitado, cargo_solicitado, google_sub, version_sesion`

// BuscarPorEmail compara contra lower(email) y no contra la columna pelada.
func (r *PostgresRepo) BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error) {
	row := r.db.QueryRow(ctx, `SELECT `+columnasUsuario+` FROM usuario WHERE lower(email) = lower($1)`, email)
	return escanearUsuario(row)
}

func (r *PostgresRepo) BuscarPorID(ctx context.Context, id string) (*domain.Usuario, error) {
	row := r.db.QueryRow(ctx, `SELECT `+columnasUsuario+` FROM usuario WHERE id = $1`, id)
	return escanearUsuario(row)
}

// BuscarPorGoogleSub resuelve el vínculo con una cuenta de Google.
func (r *PostgresRepo) BuscarPorGoogleSub(ctx context.Context, sub string) (*domain.Usuario, error) {
	if sub == "" {
		return nil, application.ErrUsuarioNoEncontrado
	}
	row := r.db.QueryRow(ctx,
		`SELECT `+columnasUsuario+` FROM usuario WHERE google_sub IS NOT NULL AND google_sub = $1`, sub)
	return escanearUsuario(row)
}

// filaUsuario son los destinos de un Scan sobre columnasUsuario, en ese mismo
// orden.
type filaUsuario struct {
	u                 domain.Usuario
	rolStr, estadoStr string
	// Las columnas nullable se escanean a *string y recién después se
	// traducen a "" — pgx no puede escribir un NULL en un string pelado.
	passwordHash, curso, materia, rolSolicitado, cargoSolicitado, googleSub *string
}

func (f *filaUsuario) destinos() []any {
	return []any{
		&f.u.ID, &f.u.Nombre, &f.u.Apellido, &f.u.Email, &f.passwordHash,
		&f.u.DebeCambiarPassword, &f.rolStr, &f.estadoStr, &f.u.FechaRegistro,
		&f.u.FechaAprobacion, &f.u.AprobadoPor, &f.curso, &f.materia, &f.rolSolicitado,
		&f.cargoSolicitado, &f.googleSub,
		&f.u.VersionSesion,
	}
}

// usuario arma la entidad con lo escaneado, validando contra el dominio lo
// que en la base es un VARCHAR suelto.
func (f *filaUsuario) usuario() (*domain.Usuario, error) {
	rol, err := domain.ParseRol(f.rolStr)
	if err != nil {
		return nil, fmt.Errorf("rol inválido en la base para usuario %s: %w", f.u.ID, err)
	}
	estado, err := domain.ParseEstado(f.estadoStr)
	if err != nil {
		return nil, fmt.Errorf("estado inválido en la base para usuario %s: %w", f.u.ID, err)
	}
	f.u.Rol = rol
	f.u.Estado = estado
	f.u.PasswordHash = textoDe(f.passwordHash)
	f.u.CursoSolicitado = textoDe(f.curso)
	f.u.MateriaSolicitada = textoDe(f.materia)
	f.u.RolSolicitado = textoDe(f.rolSolicitado)
	f.u.CargoSolicitado = textoDe(f.cargoSolicitado)
	f.u.GoogleSub = textoDe(f.googleSub)

	return &f.u, nil
}

// escanearUsuarioConTotal es escanearUsuario más la columna que agrega
// COUNT(*) OVER() al listado paginado.
func escanearUsuarioConTotal(row pgx.Row, total *int) (*domain.Usuario, error) {
	var f filaUsuario
	if err := row.Scan(append(f.destinos(), total)...); err != nil {
		return nil, fmt.Errorf("escaneando usuario: %w", err)
	}
	return f.usuario()
}

// escanearUsuario centraliza el mapeo fila→entidad, incluyendo la traducción
// de "no encontrado" al error de negocio que application/ espera (nunca dejar
// que pgx.ErrNoRows se filtre hacia arriba tal cual).
func escanearUsuario(row pgx.Row) (*domain.Usuario, error) {
	var f filaUsuario
	if err := row.Scan(f.destinos()...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrUsuarioNoEncontrado
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando usuario: %w", err)
	}
	return f.usuario()
}

func (r *PostgresRepo) Crear(ctx context.Context, u *domain.Usuario) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO usuario (id, nombre, apellido, email, password_hash, debe_cambiar_password, rol, estado, fecha_registro, curso_solicitado, materia_solicitada, rol_solicitado, cargo_solicitado, google_sub)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, u.ID, u.Nombre, u.Apellido, u.Email, textoOpcional(u.PasswordHash), u.DebeCambiarPassword,
		string(u.Rol), string(u.Estado), u.FechaRegistro,
		textoOpcional(u.CursoSolicitado), textoOpcional(u.MateriaSolicitada),
		textoOpcional(u.RolSolicitado), textoOpcional(u.CargoSolicitado),
		textoOpcional(u.GoogleSub))

	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrEmailYaRegistrado
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("insertando usuario: %w", err)
	}
	return nil
}

func (r *PostgresRepo) Guardar(ctx context.Context, u *domain.Usuario) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE usuario SET
			nombre = $2, apellido = $3, email = $4, password_hash = $5,
			debe_cambiar_password = $6, rol = $7, estado = $8,
			fecha_aprobacion = $9, aprobado_por = $10, google_sub = $11,
			version_sesion = $12
		WHERE id = $1
	`, u.ID, u.Nombre, u.Apellido, u.Email, textoOpcional(u.PasswordHash), u.DebeCambiarPassword,
		string(u.Rol), string(u.Estado), u.FechaAprobacion, u.AprobadoPor,
		textoOpcional(u.GoogleSub), u.VersionSesion)

	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrEmailYaRegistrado
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando usuario: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrUsuarioNoEncontrado
	}
	return nil
}

// Listar devuelve usuarios filtrados por estado/rol (nil = sin ese filtro).
func (r *PostgresRepo) Listar(ctx context.Context, filtroEstado *domain.Estado, filtroRol *domain.Rol, pagina paginacion.Pagina) ([]*domain.Usuario, int, error) {
	desde := ` FROM usuario WHERE 1=1`
	args := []any{}

	if filtroEstado != nil {
		args = append(args, string(*filtroEstado))
		desde += fmt.Sprintf(" AND estado = $%d", len(args))
	}
	if filtroRol != nil {
		args = append(args, string(*filtroRol))
		desde += fmt.Sprintf(" AND rol = $%d", len(args))
	}

	// El desempate por id es lo que hace que la paginación sea estable:
	// fecha_registro sola empata entre las cuentas sembradas en el mismo
	// segundo, y ahí una misma persona puede aparecer en dos páginas.
	query := `SELECT ` + columnasUsuario + `, COUNT(*) OVER() AS total` + desde +
		" ORDER BY fecha_registro DESC, id"

	argsPagina := append(append([]any{}, args...), pagina.Limit(), pagina.Offset())
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(argsPagina)-1, len(argsPagina))

	rows, err := r.db.Query(ctx, query, argsPagina...)
	if err != nil {
		return nil, 0, fmt.Errorf("listando usuarios: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Usuario
	total := 0
	for rows.Next() {
		u, err := escanearUsuarioConTotal(rows, &total)
		if err != nil {
			return nil, 0, fmt.Errorf("escaneando fila de la lista: %w", err)
		}
		resultado = append(resultado, u)
	}
	if err := errorDeFilas(rows); err != nil {
		return nil, 0, err
	}

	// Una página más allá del final no deja ninguna fila de la que leer COUNT(*)
	// OVER(), y el total en 0 haría que la pantalla dijera que no hay usuarios
	// teniendo la primera página llena.
	if len(resultado) == 0 && pagina.Offset() > 0 {
		if err := r.db.QueryRow(ctx, "SELECT COUNT(*)"+desde, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("contando usuarios: %w", err)
		}
	}

	return resultado, total, nil
}

// ContarAdminsAprobados bloquea las filas que cuenta (FOR UPDATE).
func (r *PostgresRepo) ContarAdminsAprobados(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT 1 FROM usuario WHERE rol = 'ADMIN' AND estado = 'APROBADA' FOR UPDATE
		) AS admins_bloqueados
	`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contando admins aprobados: %w", err)
	}
	return n, nil
}

func (r *PostgresRepo) Eliminar(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM usuario WHERE id = $1`, id)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("eliminando usuario: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrUsuarioNoEncontrado
	}
	return nil
}
