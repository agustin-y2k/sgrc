package application

import (
	"context"
	"fmt"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func (s *Service) verificarPeriodoDelCiclo(ctx context.Context, materiaID string, desde, hasta, ultima time.Time, inicio, fin time.Duration) error {
	anio, err := s.validadorMateria.AnioLectivoDeMateria(ctx, materiaID)
	if err != nil {
		return fmt.Errorf("verificando el año del ciclo de la materia: %w", err)
	}
	if desde.Year() != anio || hasta.Year() != anio {
		return fmt.Errorf("%w: ciclo %d, del 01/01/%d al 31/12/%d", domain.ErrReservaFueraDelCiclo, anio, anio, anio)
	}
	return domain.ValidarReservaEnCiclo(anio, ultima, inicio, fin)
}
