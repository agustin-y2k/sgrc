package application

import (
	"context"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

func (s *Service) CancelarReserva(ctx context.Context, reservaID, solicitanteID string, esAdmin bool, motivo string) error {
	return eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		return s.operacionCancelarReserva(ctx, reservaID, solicitanteID, esAdmin, motivo)
	})
}

func (s *Service) CancelarOcurrenciaRecurrente(ctx context.Context, grupoID, solicitanteID string, esAdmin bool, motivo string, soloEsta bool) (int, error) {
	var resultado int
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		resultado, err = s.operacionCancelarOcurrenciaRecurrente(ctx, grupoID, solicitanteID, esAdmin, motivo, soloEsta)
		return err
	})
	if err != nil {
		return 0, err
	}
	return resultado, nil
}

func (s *Service) CancelarReservasFuturasDeEquipo(ctx context.Context, equipoID, motivo string) (int, int, error) {
	var canceladas, afectados int
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		canceladas, afectados, err = s.operacionCancelarReservasFuturasDeEquipo(ctx, equipoID, motivo)
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	return canceladas, afectados, nil
}

func (s *Service) CancelarReservasFuturasDeMateria(ctx context.Context, materiaID, motivo string) (int, error) {
	var resultado int
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		resultado, err = s.operacionCancelarReservasFuturasDeMateria(ctx, materiaID, motivo)
		return err
	})
	if err != nil {
		return 0, err
	}
	return resultado, nil
}

func (s *Service) CancelarReservasPorIDs(ctx context.Context, ids []string, motivo string) (int, error) {
	var resultado int
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		resultado, err = s.operacionCancelarReservasPorIDs(ctx, ids, motivo)
		return err
	})
	if err != nil {
		return 0, err
	}
	return resultado, nil
}
