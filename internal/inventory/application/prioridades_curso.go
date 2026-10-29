package application

import (
	"context"
	"errors"

	"github.com/agustin-y2k/sgrc/internal/shared/prioridadcurso"
)

var ErrConflictoPrioridadCurso = errors.New("ya existe una prioridad exacta distinta para el curso de destino; revisá las prioridades antes de conservarlas")

func (s *Service) PrioridadesDeCurso(ctx context.Context, cursoID string, destino prioridadcurso.Alcance) ([]prioridadcurso.Marca, error) {
	return s.repo.PrioridadesDeCurso(ctx, cursoID, destino)
}

// ConservarPrioridadesDeCurso comparte la transacción del cambio académico.
// Las reglas generales mantienen su alcance. Las exactas siguen al curso,
// duplicándose cuando otro curso, incluso de un ciclo anterior, aún las usa.
func (s *Service) ConservarPrioridadesDeCurso(ctx context.Context, cursoID string, destino prioridadcurso.Alcance) error {
	return s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		marcas, err := s.repo.PrioridadesDeCurso(ctx, cursoID, destino)
		if err != nil {
			return err
		}
		for _, m := range marcas {
			if !m.Exacta || !m.Antes || m.Despues {
				continue
			}
			if m.DestinoID != "" && m.DestinoPrioridad != m.Prioridad {
				return ErrConflictoPrioridadCurso
			}
			p, err := s.repo.BuscarPreferenciaPorID(ctx, m.ID)
			if err != nil {
				return err
			}
			if m.DestinoID != "" {
				if !m.Compartida {
					if err := s.repo.BorrarPreferencia(ctx, p.ID); err != nil {
						return err
					}
				}
				continue
			}
			// Se trabaja sobre una copia para no modificar el valor leído del repositorio.
			nueva := *p
			nueva.Anio = &destino.Anio
			nueva.Modalidad, nueva.Division = nil, nil
			if destino.Modalidad != "" {
				nueva.Modalidad = &destino.Modalidad
			}
			if destino.Division != "" {
				nueva.Division = &destino.Division
			}
			if m.Compartida {
				nueva.ID = s.nuevoID()
				creada, err := s.repo.CrearPreferencia(ctx, &nueva)
				if err != nil {
					return err
				}
				if !creada {
					return ErrConflictoPrioridadCurso
				}
			} else if err := s.repo.GuardarPreferencia(ctx, &nueva); err != nil {
				return err
			}
		}
		return nil
	})
}
