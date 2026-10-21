package application

import (
	"context"
	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
)

func (s *Service) CrearCuentaDeEquipo(ctx context.Context, equipoID string, datos domain.DatosDeCuenta, password string) (*domain.CuentaDeEquipo, error) {
	var resultado *domain.CuentaDeEquipo
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		var err error
		resultado, err = s.operacionCrearCuentaDeEquipo(ctx, equipoID, datos, password)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) EditarCuentaDeEquipo(ctx context.Context, cuentaID string, params EditarCuentaParams) (*domain.CuentaDeEquipo, error) {
	var resultado *domain.CuentaDeEquipo
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		var err error
		resultado, err = s.operacionEditarCuentaDeEquipo(ctx, cuentaID, params)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) BorrarCuentaDeEquipo(ctx context.Context, cuentaID string) error {
	return s.repo.Coordinar(ctx, true, func(ctx context.Context) error { return s.operacionBorrarCuentaDeEquipo(ctx, cuentaID) })
}

func (s *Service) CrearIncidencia(ctx context.Context, equipoID, reportadoPor, descripcion, categoria string, gravedad domain.Gravedad) (*domain.Incidencia, error) {
	var resultado *domain.Incidencia
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		var err error
		resultado, err = s.operacionCrearIncidencia(ctx, equipoID, reportadoPor, descripcion, categoria, gravedad)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) EditarIncidencia(ctx context.Context, incidenciaID string, params EditarIncidenciaParams) error {
	return s.repo.Coordinar(ctx, true, func(ctx context.Context) error { return s.operacionEditarIncidencia(ctx, incidenciaID, params) })
}
