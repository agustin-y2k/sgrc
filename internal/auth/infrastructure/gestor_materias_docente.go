package infrastructure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/auth/application"
)

var _ application.GestorMateriasDocente = (*GestorMateriasDocentePostgres)(nil)

// GestorMateriasDocentePostgres implementa el puerto hacia academic
// (docente_materia) — a propósito NO importa internal/academic, mismo
// criterio que ValidadorUsuarioPostgres de academic hacia auth. A
// diferencia del puerto hacia reservation (que es una acción con máquina
// de estados, resuelta en cmd/wiring_adapters.go envolviendo
// reservation.Service), estas tres operaciones son lecturas simples y un
// DELETE sin ninguna regla de negocio propia — van directo por SQL acá,
// igual que todos los demás validadores de solo lectura del proyecto.
type GestorMateriasDocentePostgres struct {
	pool *pgxpool.Pool
}

func NewGestorMateriasDocentePostgres(pool *pgxpool.Pool) *GestorMateriasDocentePostgres {
	return &GestorMateriasDocentePostgres{pool: pool}
}

func (g *GestorMateriasDocentePostgres) MateriasDeDocente(ctx context.Context, usuarioID string) ([]string, error) {
	rows, err := g.pool.Query(ctx, `SELECT materia_id FROM docente_materia WHERE usuario_id = $1`, usuarioID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando materias del docente: %w", err)
	}
	defer rows.Close()

	var materias []string
	for rows.Next() {
		var materiaID string
		if err := rows.Scan(&materiaID); err != nil {
			return nil, fmt.Errorf("escaneando materia_id: %w", err)
		}
		materias = append(materias, materiaID)
	}
	return materias, errorDeFilas(rows)
}

func (g *GestorMateriasDocentePostgres) QuedaOtroDocenteActivo(ctx context.Context, materiaID, usuarioIDExcluido string) (bool, error) {
	var existe bool
	err := g.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM docente_materia dm
			JOIN usuario u ON u.id = dm.usuario_id
			WHERE dm.materia_id = $1 AND dm.usuario_id != $2 AND u.estado = 'APROBADA'
		)
	`, materiaID, usuarioIDExcluido).Scan(&existe)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando otros docentes activos: %w", err)
	}
	return existe, nil
}

func (g *GestorMateriasDocentePostgres) RemoverAsignacionesDeDocente(ctx context.Context, usuarioID string) error {
	_, err := g.pool.Exec(ctx, `DELETE FROM docente_materia WHERE usuario_id = $1`, usuarioID)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("removiendo asignaciones del docente: %w", err)
	}
	return nil
}
