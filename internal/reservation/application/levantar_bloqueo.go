package application

import (
	"context"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

type LevantarBloqueoParams struct {
	OperacionID   string
	SolicitanteID string
	EsAdmin       bool
	Motivo        string
	IPOrigen      string
}
type ResultadoLevantarBloqueo struct {
	BloqueoOperacionID   string `json:"bloqueoOperacionId"`
	ReservasCanceladas   int    `json:"reservasCanceladas"`
	ReservasYaCanceladas int    `json:"reservasYaCanceladas"`
	ReservasNoVigentes   int    `json:"reservasNoVigentes"`
}

// LevantarBloqueo conserva el alcance original de la operación, aun cuando la
// pantalla sólo muestre parte del lote. Las repeticiones no reescriben su historia.
func (s *Service) LevantarBloqueo(ctx context.Context, p LevantarBloqueoParams) (*ResultadoLevantarBloqueo, error) {
	if !p.EsAdmin || p.SolicitanteID == "" {
		return nil, ErrNoEsTuReserva
	}
	var resultado *ResultadoLevantarBloqueo
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		return s.repo.Coordinar(ctx, false, func(ctx context.Context) error {
			return s.repo.EnTransaccion(ctx, func(repo Repo) error {
				resultado = &ResultadoLevantarBloqueo{BloqueoOperacionID: p.OperacionID}
				filas, err := repo.BuscarBloqueoParaActualizar(ctx, p.OperacionID)
				if err != nil {
					return err
				}
				if len(filas) == 0 {
					return ErrBloqueoNoEncontrado
				}
				// Validar los permisos de todo el lote antes de la primera escritura.
				for _, r := range filas {
					if err := puedeCancelar(r.CreadoPor, p.SolicitanteID, true, p.Motivo); err != nil {
						return err
					}
				}
				ahora := s.ahora()
				pendientes := make([]cancelacionPendiente, 0, len(filas))
				for _, r := range filas {
					switch r.Estado {
					case domain.ReservaCancelada:
						resultado.ReservasYaCanceladas++
						continue
					case domain.ReservaConfirmada:
						if err := r.Cancelar(&p.SolicitanteID, p.Motivo, ahora); err != nil {
							return err
						}
						if err := repo.GuardarReserva(ctx, r); err != nil {
							return err
						}
						pendientes = append(pendientes, cancelacionPendiente{reserva: r, motivo: p.Motivo})
						resultado.ReservasCanceladas++
					default:
						resultado.ReservasNoVigentes++
					}
				}
				if resultado.ReservasCanceladas > 0 {
					if err := repo.RegistrarLevantamientoDeBloqueo(ctx, p, resultado.ReservasCanceladas); err != nil {
						return err
					}
					s.publicarCancelaciones(ctx, pendientes)
				}
				return nil
			})
		})
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}
