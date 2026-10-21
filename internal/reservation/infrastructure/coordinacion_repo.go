package infrastructure

import (
	"context"
	"fmt"

	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepo) exigirTransaccion(ctx context.Context) error {
	if _, ok := r.conexion(ctx).(pgx.Tx); !ok {
		return fmt.Errorf("los bloqueos de reservas requieren una transacción")
	}
	return nil
}

func (r *PostgresRepo) BuscarGruposParaActualizar(ctx context.Context, ids []string) (map[string]*domain.ReservaGrupo, error) {
	if err := r.exigirTransaccion(ctx); err != nil {
		return nil, err
	}
	grupos := make(map[string]*domain.ReservaGrupo, len(ids))
	if len(ids) == 0 {
		return grupos, nil
	}
	rows, err := r.conexion(ctx).Query(ctx, `SELECT `+columnasGrupo+` FROM reserva_grupo WHERE id=ANY($1) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, err
	}
	for rows.Next() {
		g, err := escanearReservaGrupo(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		grupos[g.ID] = g
	}
	err = errorDeFilas(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Consumir y cerrar antes de ejecutar la siguiente consulta sobre la conexión.
	rows, err = r.conexion(ctx).Query(ctx, `SELECT id FROM reserva WHERE reserva_grupo_id=ANY($1) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
	}
	err = errorDeFilas(rows)
	rows.Close()
	return grupos, err
}
