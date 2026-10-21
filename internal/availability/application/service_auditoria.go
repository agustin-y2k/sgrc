package application

import (
	"context"
	"github.com/agustin-y2k/sgrc/internal/availability/domain"
	"time"
)

func (s *Service) AgregarBloque(ctx context.Context, usuarioID string, dia domain.DiaSemana, horaInicio, horaFin time.Duration) (*domain.BloqueHorario, error) {
	var resultado *domain.BloqueHorario
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		var err error
		resultado, err = s.operacionAgregarBloque(ctx, usuarioID, dia, horaInicio, horaFin)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) EditarBloque(ctx context.Context, id, usuarioID string, dia *domain.DiaSemana, horaInicio, horaFin *time.Duration) (*domain.BloqueHorario, error) {
	var resultado *domain.BloqueHorario
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		var err error
		resultado, err = s.operacionEditarBloque(ctx, id, usuarioID, dia, horaInicio, horaFin)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) EliminarBloque(ctx context.Context, id, usuarioID string) error {
	return s.repo.Coordinar(ctx, true, func(ctx context.Context) error { return s.operacionEliminarBloque(ctx, id, usuarioID) })
}

func (s *Service) CargarExcepcion(ctx context.Context, usuarioID string, fecha time.Time, tipo domain.TipoExcepcion, horaInicio, horaFin *time.Duration, motivo *string) (*domain.Excepcion, error) {
	var resultado *domain.Excepcion
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		var err error
		resultado, err = s.operacionCargarExcepcion(ctx, usuarioID, fecha, tipo, horaInicio, horaFin, motivo)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}
