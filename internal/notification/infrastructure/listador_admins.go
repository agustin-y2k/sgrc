package infrastructure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/notification/application"
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

// EmailsDeAdminsAprobados es la misma consulta con otra columna: a quién le
// llega la copia por correo del aviso.
func (l *ListadorAdminsPostgres) EmailsDeAdminsAprobados(ctx context.Context) ([]string, error) {
	return l.columnaDeAdminsAprobados(ctx, "email")
}

// columnaDeAdminsAprobados evita duplicar la consulta y, sobre todo, evita
// que las dos versiones del filtro se separen con el tiempo: si mañana cambia
// qué cuenta como "Admin activo", tiene que cambiar para el aviso interno y
// para el correo a la vez.
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
