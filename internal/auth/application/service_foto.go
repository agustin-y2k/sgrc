package application

import (
	"context"
	"fmt"

	"github.com/ramiro/sgrc/internal/auth/domain"
)

// La foto de perfil (ver domain.FotoDePerfil).

// GuardarMiFoto reemplaza la foto propia.
func (s *Service) GuardarMiFoto(ctx context.Context, usuarioID string, contenido []byte) (*domain.FotoDePerfil, error) {
	f, err := domain.NuevaFotoDePerfil(usuarioID, contenido, s.ahora())
	if err != nil {
		return nil, err
	}
	if err := s.repo.GuardarFoto(ctx, f); err != nil {
		return nil, fmt.Errorf("guardando la foto: %w", err)
	}
	return f, nil
}

// BuscarFoto la devuelve para mostrarla.
func (s *Service) BuscarFoto(ctx context.Context, usuarioID string) (*domain.FotoDePerfil, error) {
	return s.repo.BuscarFoto(ctx, usuarioID)
}

// EliminarMiFoto vuelve a las iniciales.
func (s *Service) EliminarMiFoto(ctx context.Context, usuarioID string) error {
	return s.repo.EliminarFoto(ctx, usuarioID)
}
