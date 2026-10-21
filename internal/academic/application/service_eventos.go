package application

import (
	"context"
	"github.com/agustin-y2k/sgrc/internal/academic/domain"
)

func (s *Service) PedirMateria(ctx context.Context, usuarioID string, materiaID *string, curso, materia, motivo string) (*domain.PedidoDeMateria, error) {
	var r0 *domain.PedidoDeMateria
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionPedirMateria(ctx, usuarioID, materiaID, curso, materia, motivo)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
