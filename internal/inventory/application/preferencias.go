package application

import (
	"context"
	"fmt"

	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
)

// Marcas de preferencia de materia por equipo (RF-03.21).

// NuevaPreferenciaParams es una marca aplicada a varios equipos de una vez.
type NuevaPreferenciaParams struct {
	EquipoIDs     []string
	MateriaNombre string
	// Ejes del alcance: opcionales en reglas generales; exactos en curso concreto.
	Modalidad   *string
	Anio        *int
	Division    *string
	Prioridad   int
	CursoExacto bool
}

// ResultadoAltaDePreferencias separa lo creado de lo que ya estaba.
type ResultadoAltaDePreferencias struct {
	Creadas          []*domain.PreferenciaDeEquipo
	EquiposQueYaTeni []string
}

// MarcarPreferencia crea la marca en cada equipo del lote.
func (s *Service) MarcarPreferencia(ctx context.Context, params NuevaPreferenciaParams) (*ResultadoAltaDePreferencias, error) {
	var resultado *ResultadoAltaDePreferencias
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		var err error
		resultado, err = s.marcarPreferenciaCoordinada(ctx, params)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) marcarPreferenciaCoordinada(ctx context.Context, params NuevaPreferenciaParams) (*ResultadoAltaDePreferencias, error) {
	if len(params.EquipoIDs) == 0 {
		return nil, domain.ErrSinEquiposParaPreferi
	}

	validada, err := domain.NuevaPreferencia("", "", params.MateriaNombre, params.Modalidad, params.Anio, params.Division, params.Prioridad, params.CursoExacto)
	if err != nil {
		return nil, err
	}
	existe, err := s.repo.ExisteAlcancePreferencia(ctx, validada)
	if err != nil {
		return nil, err
	}
	if !existe {
		return nil, ErrAlcancePreferenciaInexistente
	}
	resultado := &ResultadoAltaDePreferencias{}

	// El lote entero en una transacción: marcar veinte máquinas y que se
	// marquen doce deja el inventario a medio camino, y la pantalla no dice
	// cuáles quedaron sin marcar.
	err = s.repo.EnTransaccion(ctx, func(repo Repo) error {
		resultado = &ResultadoAltaDePreferencias{}

		for _, equipoID := range params.EquipoIDs {
			p, err := domain.NuevaPreferencia(s.nuevoID(), equipoID, params.MateriaNombre,
				params.Modalidad, params.Anio, params.Division, params.Prioridad, params.CursoExacto)
			if err != nil {
				// La materia, el alcance y la prioridad son los mismos para todo el
				// lote: si no validan, no validan para ninguno y seguir intentando
				// con las demás máquinas no puede cambiar nada.
				return err
			}

			creada, err := repo.CrearPreferencia(ctx, p)
			if err != nil {
				return fmt.Errorf("marcando la preferencia en el equipo %s: %w", equipoID, err)
			}
			if !creada {
				// Ya la tenía: se informa aparte y no voltea el lote.
				resultado.EquiposQueYaTeni = append(resultado.EquiposQueYaTeni, equipoID)
				continue
			}
			resultado.Creadas = append(resultado.Creadas, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

// EditarPreferencia cambia el alcance y la prioridad de una marca existente.
func (s *Service) EditarPreferencia(ctx context.Context, id string, modalidad *string, anio *int,
	division *string, prioridad int, cursoExacto ...bool) (*domain.PreferenciaDeEquipo, error) {
	var resultado *domain.PreferenciaDeEquipo
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		var err error
		resultado, err = s.editarPreferenciaCoordinada(ctx, id, modalidad, anio, division, prioridad, cursoExacto...)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) editarPreferenciaCoordinada(ctx context.Context, id string, modalidad *string, anio *int,
	division *string, prioridad int, cursoExacto ...bool) (*domain.PreferenciaDeEquipo, error) {
	actual, err := s.repo.BuscarPreferenciaPorID(ctx, id)
	if err != nil {
		return nil, err
	}

	exacto := actual.CursoExacto
	if len(cursoExacto) > 0 {
		exacto = cursoExacto[0]
	}
	// Se reconstruye por el constructor en vez de asignar campos sueltos: así
	// el alcance editado pasa por las mismas validaciones y los mismos
	// recortes que el nuevo.
	editada, err := domain.NuevaPreferencia(actual.ID, actual.EquipoID, actual.MateriaNombre,
		modalidad, anio, division, prioridad, exacto)
	if err != nil {
		return nil, err
	}

	existe, err := s.repo.ExisteAlcancePreferencia(ctx, editada)
	if err != nil {
		return nil, err
	}
	if !existe {
		return nil, ErrAlcancePreferenciaInexistente
	}

	if err := s.repo.GuardarPreferencia(ctx, editada); err != nil {
		return nil, err
	}
	return editada, nil
}

// BorrarPreferencia saca la marca. Le devuelve el equipo al orden neutral y
// no afecta ninguna reserva existente.
func (s *Service) BorrarPreferencia(ctx context.Context, id string) error {
	return s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		return s.repo.BorrarPreferencia(ctx, id)
	})
}

func (s *Service) ListarPreferenciasDeEquipo(ctx context.Context, equipoID string) ([]*domain.PreferenciaDeEquipo, error) {
	return s.repo.ListarPreferenciasPorEquipo(ctx, equipoID)
}

func (s *Service) ListarPreferenciasDeCarro(ctx context.Context, carroID string) ([]*domain.PreferenciaDeEquipo, error) {
	return s.repo.ListarPreferenciasPorCarro(ctx, carroID)
}

// NombresDeMateriaEnUso conserva el catálogo histórico por compatibilidad.
// La interfaz de prioridades utiliza CatalogoDePrioridades y sus relaciones.
func (s *Service) NombresDeMateriaEnUso(ctx context.Context) ([]string, error) {
	return s.repo.NombresDeMateriaEnUso(ctx)
}

// ListarPreferenciasHuerfanas: las marcas que ya no cruzan con ninguna materia
// vigente (RF-03.21).
//
// Aparecen al renombrar, borrar o archivar una materia, porque la marca se vincula por
// NOMBRE y no por referencia — un diseño deliberado, para que las marcas
// sobrevivan al clonado anual del ciclo, y cuya contracara es justamente ésta.
//
// Sin esta consulta las huérfanas son invisibles: siguen mostrándose en la
// ficha del equipo exactamente igual que una marca que sí aplica.
func (s *Service) ListarPreferenciasHuerfanas(ctx context.Context) ([]*PreferenciaHuerfana, error) {
	return s.repo.ListarPreferenciasHuerfanas(ctx)
}

// ContarMarcasDeMateria: cuántas marcas de preferencia apuntan a ese nombre de
// materia.
//
// Lo pregunta academic antes de renombrar una materia, para poder avisar en vez
// de romper en silencio. Es una lectura, no una regla: el renombre se hace
// igual, con el número o sin él.
func (s *Service) ContarMarcasDeMateria(ctx context.Context, materiaID, materiaNombre string) (int, error) {
	return s.repo.ContarPreferenciasQueDejarianDeAplicar(ctx, materiaID, materiaNombre)
}

// ObtenerPreferencia — ver el bloque «Obtener uno solo» de service.go.
func (s *Service) ObtenerPreferencia(ctx context.Context, id string) (*domain.PreferenciaDeEquipo, error) {
	return s.repo.BuscarPreferenciaPorID(ctx, id)
}
