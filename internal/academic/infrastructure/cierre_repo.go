package infrastructure

import (
	"context"
	"errors"

	"github.com/agustin-y2k/sgrc/internal/academic/application"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepo) EnTransaccion(ctx context.Context, fn func(context.Context) error) error {
	return pgtransaccion.Ejecutar(ctx, r.pool, fn)
}

func (r *PostgresRepo) CoordinarAsignaciones(ctx context.Context, fn func(context.Context) error) error {
	return pgtransaccion.Coordinar(ctx, r.pool, true, fn)
}

func (r *PostgresRepo) EnCierre(ctx context.Context, cicloID string, fn func(context.Context) error) error {
	return pgtransaccion.Ejecutar(ctx, r.pool, func(ctx context.Context) error {
		var id string
		err := pgtransaccion.Con(ctx, r.pool).QueryRow(ctx, `SELECT id FROM ciclo_lectivo WHERE id=$1 FOR UPDATE`, cicloID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrCicloNoEncontrado
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		if err != nil {
			return err
		}
		return fn(ctx)
	})
}
func (r *PostgresRepo) BuscarCierre(ctx context.Context, cicloID string) (*application.CierreCompletado, error) {
	var cierre application.CierreCompletado
	err := pgtransaccion.Con(ctx, r.pool).QueryRow(ctx, `SELECT anio_destino,nuevo_ciclo_id,cursos_clonados,materias_clonadas FROM cierre_ciclo WHERE ciclo_id=$1`, cicloID).Scan(&cierre.AnioDestino, &cierre.Resultado.NuevoCicloID, &cierre.Resultado.CursosClonados, &cierre.Resultado.MateriasClonadas)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cierre, nil
}
func (r *PostgresRepo) GuardarCierre(ctx context.Context, cicloID string, cierre *application.CierreCompletado) error {
	_, err := pgtransaccion.Con(ctx, r.pool).Exec(ctx, `INSERT INTO cierre_ciclo(ciclo_id,anio_destino,nuevo_ciclo_id,cursos_clonados,materias_clonadas) VALUES($1,$2,$3,$4,$5)`, cicloID, cierre.AnioDestino, cierre.Resultado.NuevoCicloID, cierre.Resultado.CursosClonados, cierre.Resultado.MateriasClonadas)
	return err
}
