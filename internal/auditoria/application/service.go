package application

import (
	"context"

	"github.com/ramiro/sgrc/internal/auditoria/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// Service implementa la consulta del registro de auditoría.
//
// Es deliberadamente delgado: lo único que decide es que el filtro sea válido.
// No hay reglas de negocio que aplicar sobre un registro que, por definición,
// ya está escrito y no se toca.
type Service struct {
	repo Repo
}

func NewService(repo Repo) *Service {
	return &Service{repo: repo}
}

// Listar devuelve las entradas que cumplen el filtro, de la más reciente a la
// más vieja, y el total sin paginar.
func (s *Service) Listar(ctx context.Context, f domain.Filtro, p paginacion.Pagina) ([]domain.Entrada, int, error) {
	if err := f.Validar(); err != nil {
		return nil, 0, err
	}
	return s.repo.Listar(ctx, f, p)
}

// OpcionesDeFiltro son los valores que existen HOY en el registro, para armar
// los dos selectores de la pantalla.
type OpcionesDeFiltro struct {
	Acciones  []string
	Entidades []string
}

func (s *Service) OpcionesDeFiltro(ctx context.Context) (*OpcionesDeFiltro, error) {
	acciones, err := s.repo.AccionesEnUso(ctx)
	if err != nil {
		return nil, err
	}
	entidades, err := s.repo.EntidadesEnUso(ctx)
	if err != nil {
		return nil, err
	}
	return &OpcionesDeFiltro{Acciones: acciones, Entidades: entidades}, nil
}
