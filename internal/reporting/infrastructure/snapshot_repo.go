package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agustin-y2k/sgrc/internal/reporting/application"
	"github.com/agustin-y2k/sgrc/internal/reporting/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
	"github.com/jackc/pgx/v5"
)

func identidadHistoricaDe(h *domain.HistoricoUsoDocente) string {
	if h.DocenteIDHistorico != "" {
		return h.DocenteIDHistorico
	}
	if h.UsuarioID != nil {
		return *h.UsuarioID
	}
	return h.ID
}
func (r *PostgresRepo) EnSnapshot(ctx context.Context, cicloID string, fn func(context.Context) error) error {
	return pgtransaccion.Coordinar(ctx, r.pool, true, func(ctx context.Context) error {
		var id string
		err := pgtransaccion.Con(ctx, r.pool).QueryRow(ctx, `SELECT id FROM ciclo_lectivo WHERE id=$1 FOR UPDATE`, cicloID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrIDInvalido
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
func (r *PostgresRepo) SnapshotSellado(ctx context.Context, cicloID string) (bool, error) {
	var sellado bool
	err := pgtransaccion.Con(ctx, r.pool).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM snapshot_ciclo WHERE ciclo_id=$1)`, cicloID).Scan(&sellado)
	return sellado, err
}
func (r *PostgresRepo) BorrarHistoricosNoSellados(ctx context.Context, anio int) error {
	for _, tabla := range []string{"historico_uso_equipo", "historico_uso_docente"} {
		if _, err := pgtransaccion.Con(ctx, r.pool).Exec(ctx, `DELETE FROM `+tabla+` WHERE anio=$1 AND NOT EXISTS(SELECT 1 FROM snapshot_ciclo WHERE anio=$1)`, anio); err != nil {
			return err
		}
	}
	return nil
}
func (r *PostgresRepo) SellarSnapshot(ctx context.Context, cicloID string, anio int) error {
	actividad, err := r.CalcularActividad(ctx, cicloID, nil, nil)
	if err != nil {
		return err
	}
	datos, err := json.Marshal(actividad)
	if err != nil {
		return err
	}
	tag, err := pgtransaccion.Con(ctx, r.pool).Exec(ctx, `INSERT INTO snapshot_ciclo(ciclo_id,anio,actividad) SELECT id,anio,$3 FROM ciclo_lectivo WHERE id=$1 AND anio=$2`, cicloID, anio, datos)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("el año del snapshot no coincide con el ciclo")
	}
	return nil
}

func (r *PostgresRepo) TotalesUsoDeCiclo(ctx context.Context, cicloID string) (domain.TotalesUso, error) {
	var t domain.TotalesUso
	err := pgtransaccion.Con(ctx, r.pool).QueryRow(ctx, `
  WITH fuente AS (
   SELECT r.tipo, `+expresionMinutosDe("r")+` AS minutos FROM reserva r
   WHERE `+criterioReservaComputable("r")+` AND (
    r.materia_id IN (SELECT materia_id FROM contenedor_de_materia WHERE ciclo_lectivo_id=$1)
    OR (r.tipo='BLOQUEO' AND EXTRACT(YEAR FROM r.fecha)=(SELECT anio FROM ciclo_lectivo WHERE id=$1))
   )
  )
  SELECT COUNT(*),COALESCE(SUM(minutos),0),COUNT(*) FILTER(WHERE tipo='NORMAL'),
   COALESCE(SUM(minutos) FILTER(WHERE tipo='NORMAL'),0) FROM fuente`, cicloID).
		Scan(&t.ReservasEquipos, &t.MinutosEquipos, &t.ReservasDocentes, &t.MinutosDocentes)
	return t, err
}
