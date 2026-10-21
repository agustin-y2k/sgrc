package application

import (
	"context"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/agustin-y2k/sgrc/internal/sugerencias/domain"
)

func (s *Service) Escribir(ctx context.Context, usuarioID, tipo, asunto, texto, pantalla, version string) (*domain.Sugerencia, error) {
	var r0 *domain.Sugerencia
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionEscribir(ctx, usuarioID, tipo, asunto, texto, pantalla, version)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
func (s *Service) Responder(ctx context.Context, sugerenciaID, autorID string, deAdmin bool, texto string) (*domain.Sugerencia, error) {
	var r0 *domain.Sugerencia
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionResponder(ctx, sugerenciaID, autorID, deAdmin, texto)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
func (s *Service) MarcarResuelta(ctx context.Context, sugerenciaID string) (*domain.Sugerencia, error) {
	var r0 *domain.Sugerencia
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionMarcarResuelta(ctx, sugerenciaID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
