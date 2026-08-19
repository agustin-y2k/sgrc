package application

import (
	"context"
	"fmt"

	"github.com/ramiro/sgrc/internal/auth/domain"
)

// La foto de perfil (ver domain.FotoDePerfil).
//
// Es de cada quien sobre su propia cuenta: no hay forma de subirle una foto
// a otro ni de borrarle la suya. Un Admin puede VER la de cualquiera —
// aparece al lado del nombre en las listas—, que es distinto de poder
// cambiarla.

// GuardarMiFoto reemplaza la foto propia. Si ya había una, se pisa: no hay
// historial de fotos, y guardarlo sería juntar imágenes viejas de gente que
// justamente quiso cambiarlas.
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

// BuscarFoto la devuelve para mostrarla. La puede pedir cualquier usuario
// autenticado: las fotos se ven al lado del nombre en las pantallas
// compartidas, igual que el nombre.
func (s *Service) BuscarFoto(ctx context.Context, usuarioID string) (*domain.FotoDePerfil, error) {
	return s.repo.BuscarFoto(ctx, usuarioID)
}

// EliminarMiFoto vuelve a las iniciales.
func (s *Service) EliminarMiFoto(ctx context.Context, usuarioID string) error {
	return s.repo.EliminarFoto(ctx, usuarioID)
}
