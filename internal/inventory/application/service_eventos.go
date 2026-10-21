package application

import (
	"context"
	"time"
)

func (s *Service) CrearLicencias(ctx context.Context, params NuevaLicenciaParams) (*ResultadoAltaMasiva, error) {
	var r0 *ResultadoAltaMasiva
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionCrearLicencias(ctx, params)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
func (s *Service) RenovarLicencias(ctx context.Context, ids []string, renovadaEl *time.Time, porUsuario string) (*ResultadoRenovacion, error) {
	var r0 *ResultadoRenovacion
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionRenovarLicencias(ctx, ids, renovadaEl, porUsuario)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
func (s *Service) EditarLicencia(ctx context.Context, licenciaID string, params EditarLicenciaParams) error {
	return s.repo.Coordinar(ctx, true, func(ctx context.Context) error { return s.operacionEditarLicencia(ctx, licenciaID, params) })
}
func (s *Service) BorrarLicencia(ctx context.Context, licenciaID string) error {
	return s.repo.Coordinar(ctx, true, func(ctx context.Context) error { return s.operacionBorrarLicencia(ctx, licenciaID) })
}
