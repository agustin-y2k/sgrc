// Package infrastructure implementa application.Repo de auditoria contra
// PostgreSQL real (pgx).
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/auditoria/application"
	"github.com/ramiro/sgrc/internal/auditoria/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// codigoTextoInvalido: SQLSTATE 22P02 — "invalid input syntax for type X".
const codigoTextoInvalido = "22P02"

var _ application.Repo = (*PostgresRepo)(nil)

type PostgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

func esIDInvalido(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoTextoInvalido
}

// condiciones arma el WHERE una sola vez para las dos consultas —la del conteo
// y la de la página—, porque si se separan cuentan cosas distintas y el
// paginador empieza a mostrar páginas vacías al final.
//
// Cada filtro se aplica con el patrón `$n IS NULL OR columna = $n`: así hay UNA
// sola consulta preparada que sirve para todas las combinaciones, en vez de
// concatenar SQL según qué campos vinieron.
const condiciones = `
	WHERE ($1::text IS NULL OR a.accion = $1)
	  AND ($2::text IS NULL OR a.entidad = $2)
	  AND ($3::uuid IS NULL OR a.entidad_id = $3)
	  AND ($4::uuid IS NULL OR a.usuario_id = $4)
	  AND ($5::timestamptz IS NULL OR a.creado_en >= $5)
	  AND ($6::timestamptz IS NULL OR a.creado_en < $6)`

// Listar devuelve la página pedida y el total sin paginar.
//
// El JOIN con `usuario` es un LEFT JOIN y NO puede ser otra cosa: `audit_log`
// no tiene clave foránea a propósito, para que lo que hizo una cuenta sobreviva
// a su eliminación (RF-01.9). Con un INNER JOIN, eliminar una cuenta borraría
// de la vista todo lo que esa cuenta hizo — que es exactamente lo que el
// registro existe para impedir.
func (r *PostgresRepo) Listar(ctx context.Context, f domain.Filtro, p paginacion.Pagina) ([]domain.Entrada, int, error) {
	args := []any{
		nullSiVacio(f.Accion), nullSiVacio(f.Entidad), nullSiVacio(f.EntidadID),
		nullSiVacio(f.UsuarioID), f.Desde, f.Hasta,
	}

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log a`+condiciones, args...).Scan(&total); err != nil {
		if esIDInvalido(err) {
			return nil, 0, application.ErrIDInvalido
		}
		return nil, 0, fmt.Errorf("contando entradas de auditoría: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT a.id, a.usuario_id,
		       COALESCE(u.nombre || ' ' || u.apellido, ''),
		       a.accion, a.entidad, COALESCE(a.entidad_id::text, ''),
		       a.detalle, COALESCE(host(a.ip_origen), ''), a.creado_en
		  FROM audit_log a
		  LEFT JOIN usuario u ON u.id = a.usuario_id`+condiciones+`
		 ORDER BY a.creado_en DESC, a.id
		 LIMIT $7 OFFSET $8`,
		append(args, p.Limit(), p.Offset())...)
	if err != nil {
		if esIDInvalido(err) {
			return nil, 0, application.ErrIDInvalido
		}
		return nil, 0, fmt.Errorf("listando entradas de auditoría: %w", err)
	}
	defer rows.Close()

	var resultado []domain.Entrada
	for rows.Next() {
		var e domain.Entrada
		if err := rows.Scan(&e.ID, &e.UsuarioID, &e.ActorNombre, &e.Accion,
			&e.Entidad, &e.EntidadID, &e.Detalle, &e.IPOrigen, &e.CreadoEn); err != nil {
			return nil, 0, fmt.Errorf("escaneando entrada de auditoría: %w", err)
		}
		resultado = append(resultado, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterando entradas de auditoría: %w", err)
	}
	return resultado, total, nil
}

func (r *PostgresRepo) AccionesEnUso(ctx context.Context) ([]string, error) {
	return r.valoresDistintos(ctx, `SELECT DISTINCT accion FROM audit_log ORDER BY accion`)
}

func (r *PostgresRepo) EntidadesEnUso(ctx context.Context) ([]string, error) {
	return r.valoresDistintos(ctx, `SELECT DISTINCT entidad FROM audit_log ORDER BY entidad`)
}

func (r *PostgresRepo) valoresDistintos(ctx context.Context, consulta string) ([]string, error) {
	rows, err := r.pool.Query(ctx, consulta)
	if err != nil {
		return nil, fmt.Errorf("leyendo los valores en uso: %w", err)
	}
	defer rows.Close()

	// Vacío y no nil: el selector de la pantalla lo recorre sin preguntar.
	valores := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("escaneando valor: %w", err)
		}
		valores = append(valores, v)
	}
	return valores, rows.Err()
}

// nullSiVacio manda NULL en vez de cadena vacía, que es lo que hace que el
// patrón `$n IS NULL OR ...` desactive el filtro en vez de buscar "".
func nullSiVacio(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
