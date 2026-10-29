package infrastructure

import (
	"context"
	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/audit"
)

// Los bloqueos no tienen grupo padre. Se bloquea el lote completo por ID,
// incluidos sus estados terminales para reconocer una repetición sin sobrescribirlos.
func (r *PostgresRepo) BuscarBloqueoParaActualizar(ctx context.Context, operacionID string) ([]*domain.Reserva, error) {
	filas, err := r.conexion(ctx).Query(ctx, `SELECT `+columnasReserva+` FROM reserva WHERE tipo='BLOQUEO' AND bloqueo_operacion_id=$1 ORDER BY id FOR UPDATE`, operacionID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, err
	}
	defer filas.Close()
	var resultado []*domain.Reserva
	for filas.Next() {
		reserva, err := escanearReserva(filas)
		if err != nil {
			return nil, err
		}
		resultado = append(resultado, reserva)
	}
	return resultado, errorDeFilas(filas)
}

func (r *PostgresRepo) RegistrarLevantamientoDeBloqueo(ctx context.Context, p application.LevantarBloqueoParams, cantidad int) error {
	return audit.NewPostgresAuditor(r.pool).Registrar(ctx, audit.Entrada{
		UsuarioID: p.SolicitanteID, Accion: audit.BloqueoLevantado, Entidad: "bloqueo_operacion", EntidadID: &p.OperacionID,
		IPOrigen: p.IPOrigen, Detalle: map[string]any{"motivo": p.Motivo, "reservasCanceladas": cantidad},
	})
}
