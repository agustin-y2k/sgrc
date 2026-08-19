package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// Marcas de preferencia de materia por equipo (RF-03.21).

// NuevaPreferenciaParams es una marca aplicada a varios equipos de una vez.
type NuevaPreferenciaParams struct {
	EquipoIDs     []string
	MateriaNombre string
	Anio          *int
	Division      *string
	Prioridad     int
}

// ResultadoAltaDePreferencias separa lo creado de lo que ya estaba.
type ResultadoAltaDePreferencias struct {
	Creadas          []*domain.PreferenciaDeEquipo
	EquiposQueYaTeni []string
}

// MarcarPreferencia crea la marca en cada equipo del lote.
func (s *Service) MarcarPreferencia(ctx context.Context, params NuevaPreferenciaParams) (*ResultadoAltaDePreferencias, error) {
	if len(params.EquipoIDs) == 0 {
		return nil, domain.ErrSinEquiposParaPreferi
	}

	resultado := &ResultadoAltaDePreferencias{}
	for _, equipoID := range params.EquipoIDs {
		p, err := domain.NuevaPreferencia(s.nuevoID(), equipoID, params.MateriaNombre,
			params.Anio, params.Division, params.Prioridad)
		if err != nil {
			// La materia, el alcance y la prioridad son los mismos para todo el lote:
			// si no validan, no validan para ninguno y seguir intentando con las demás
			// máquinas no puede cambiar nada.
			return nil, err
		}

		if err := s.repo.CrearPreferencia(ctx, p); err != nil {
			if errors.Is(err, domain.ErrPreferenciaDuplicada) {
				resultado.EquiposQueYaTeni = append(resultado.EquiposQueYaTeni, equipoID)
				continue
			}
			return nil, fmt.Errorf("marcando la preferencia en el equipo %s: %w", equipoID, err)
		}
		resultado.Creadas = append(resultado.Creadas, p)
	}
	return resultado, nil
}

// EditarPreferencia cambia el alcance y la prioridad de una marca existente.
func (s *Service) EditarPreferencia(ctx context.Context, id string, anio *int, division *string, prioridad int) (*domain.PreferenciaDeEquipo, error) {
	actual, err := s.repo.BuscarPreferenciaPorID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Se reconstruye por el constructor en vez de asignar campos sueltos: así el
	// alcance editado pasa por las mismas validaciones que el nuevo (una
	// división sin año, por ejemplo).
	editada, err := domain.NuevaPreferencia(actual.ID, actual.EquipoID, actual.MateriaNombre,
		anio, division, prioridad)
	if err != nil {
		return nil, err
	}

	if err := s.repo.GuardarPreferencia(ctx, editada); err != nil {
		return nil, err
	}
	return editada, nil
}

// BorrarPreferencia saca la marca. Le devuelve el equipo al orden neutral y
// no afecta ninguna reserva existente.
func (s *Service) BorrarPreferencia(ctx context.Context, id string) error {
	return s.repo.BorrarPreferencia(ctx, id)
}

func (s *Service) ListarPreferenciasDeEquipo(ctx context.Context, equipoID string) ([]*domain.PreferenciaDeEquipo, error) {
	return s.repo.ListarPreferenciasPorEquipo(ctx, equipoID)
}

// NombresDeMateriaEnUso son las materias que el Admin puede elegir al
// marcar. Ver el puerto sobre por qué son nombres y no materias.
func (s *Service) NombresDeMateriaEnUso(ctx context.Context) ([]string, error) {
	return s.repo.NombresDeMateriaEnUso(ctx)
}
