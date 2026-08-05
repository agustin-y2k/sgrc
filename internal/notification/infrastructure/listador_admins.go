package infrastructure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/notification/application"
)

var _ application.ListadorAdmins = (*ListadorAdminsPostgres)(nil)

// ListadorAdminsPostgres implementa el puerto hacia auth — consulta
// usuario directamente, sin importar internal/auth (mismo criterio que
// ValidadorUsuarioPostgres de academic).
type ListadorAdminsPostgres struct {
	pool *pgxpool.Pool
}

func NewListadorAdminsPostgres(pool *pgxpool.Pool) *ListadorAdminsPostgres {
	return &ListadorAdminsPostgres{pool: pool}
}

func (l *ListadorAdminsPostgres) IDsDeAdminsAprobados(ctx context.Context) ([]string, error) {
	rows, err := l.pool.Query(ctx, `SELECT id FROM usuario WHERE rol = 'ADMIN' AND estado = 'APROBADA'`)
	if err != nil {
		return nil, fmt.Errorf("listando admins aprobados: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("escaneando id de admin: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, errorDeFilas(rows)
}
