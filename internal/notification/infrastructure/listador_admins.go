package infrastructure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/notification/application"
	"github.com/ramiro/sgrc/internal/notification/domain"
)

var _ application.ListadorAdmins = (*ListadorAdminsPostgres)(nil)

// ListadorAdminsPostgres implementa el puerto hacia auth — consulta usuario
// directamente, sin importar internal/auth (mismo criterio que
// ValidadorUsuarioPostgres de academic).
type ListadorAdminsPostgres struct {
	pool *pgxpool.Pool
}

func NewListadorAdminsPostgres(pool *pgxpool.Pool) *ListadorAdminsPostgres {
	return &ListadorAdminsPostgres{pool: pool}
}

func (l *ListadorAdminsPostgres) IDsDeAdminsAprobados(ctx context.Context) ([]string, error) {
	return l.columnaDeAdminsAprobados(ctx, "id")
}

// EmailsDeAdminsSuscriptos son los Admin aprobados que reciben esa categoría
// por correo (RF-05.13). El LEFT JOIN es lo que hace de filtro, y el COALESCE
// resuelve a quien nunca abrió el panel: sin fila vale el valor por defecto de
// la categoría, que lo dice el dominio y no esta consulta.
func (l *ListadorAdminsPostgres) EmailsDeAdminsSuscriptos(ctx context.Context, categoria domain.CategoriaEmail) ([]string, error) {
	rows, err := l.pool.Query(ctx, `
		SELECT u.email
		FROM usuario u
		LEFT JOIN preferencia_email p ON p.usuario_id = u.id AND p.categoria = $1
		WHERE u.rol = 'ADMIN' AND u.estado = 'APROBADA' AND COALESCE(p.activa, $2)
	`, string(categoria), categoria.ActivaPorDefecto())
	if err != nil {
		return nil, fmt.Errorf("listando admins suscriptos a %s: %w", categoria, err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("escaneando email de admin suscripto: %w", err)
		}
		emails = append(emails, email)
	}
	return emails, errorDeFilas(rows)
}

// columnaDeAdminsAprobados deja el filtro de "Admin activo" en un solo lugar:
// el de arriba lo repite con un JOIN encima, así que si mañana cambia qué
// cuenta como Admin activo, hay que tocar los dos.
func (l *ListadorAdminsPostgres) columnaDeAdminsAprobados(ctx context.Context, columna string) ([]string, error) {
	rows, err := l.pool.Query(ctx,
		`SELECT `+columna+` FROM usuario WHERE rol = 'ADMIN' AND estado = 'APROBADA'`)
	if err != nil {
		return nil, fmt.Errorf("listando admins aprobados: %w", err)
	}
	defer rows.Close()

	var valores []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("escaneando %s de admin: %w", columna, err)
		}
		valores = append(valores, v)
	}
	return valores, errorDeFilas(rows)
}
