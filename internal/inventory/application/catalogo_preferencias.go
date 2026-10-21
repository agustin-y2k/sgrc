package application

import "context"

// MateriaParaPreferencia conserva la relación académica del ciclo activo.
type MateriaParaPreferencia struct {
	MateriaID        string
	MateriaNombre    string
	ContenedorID     string
	ContenedorNombre string
	Tipo             string
	Modalidad        *string
	Anio             *int
	Division         *string
}

func (s *Service) CatalogoDePrioridades(ctx context.Context) ([]MateriaParaPreferencia, error) {
	return s.repo.CatalogoDePrioridades(ctx)
}
