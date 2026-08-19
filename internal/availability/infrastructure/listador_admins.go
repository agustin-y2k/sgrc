package infrastructure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/availability/application"
)

var _ application.ListadorAdmins = (*ListadorAdminsPostgres)(nil)

// ListadorAdminsPostgres implementa el puerto hacia auth — consulta usuario
// directamente, sin importar internal/auth (mismo criterio que
// ListadorAdminsPostgres de notification, aunque acá también trae
// nombre/apellido para el DTO de disponibilidad, RF-07.2).
type ListadorAdminsPostgres struct {
	pool *pgxpool.Pool
}

func NewListadorAdminsPostgres(pool *pgxpool.Pool) *ListadorAdminsPostgres {
	return &ListadorAdminsPostgres{pool: pool}
}

func (l *ListadorAdminsPostgres) AdminsAprobados(ctx context.Context) ([]application.AdminInfo, error) {
	rows, err := l.pool.Query(ctx, `SELECT id, nombre, apellido FROM usuario WHERE rol = 'ADMIN' AND estado = 'APROBADA'`)
	if err != nil {
		return nil, fmt.Errorf("listando admins aprobados: %w", err)
	}
	defer rows.Close()

	var admins []application.AdminInfo
	for rows.Next() {
		var a application.AdminInfo
		if err := rows.Scan(&a.ID, &a.Nombre, &a.Apellido); err != nil {
			return nil, fmt.Errorf("escaneando admin: %w", err)
		}
		admins = append(admins, a)
	}
	return admins, errorDeFilas(rows)
}
