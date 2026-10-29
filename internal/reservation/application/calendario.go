package application

import (
	"context"
	"errors"
)

var ErrRangoCalendario = errors.New("el calendario requiere desde y hasta, con un máximo de 42 días")

// ListarCalendario comparte ocupación sin ampliar los permisos de gestión.
// Se pagina por clases completas, manteniendo los equipos cancelados del grupo.
func (s *Service) ListarCalendario(ctx context.Context, f FiltroReservas, solicitanteID string, soloMias bool) ([]ReservaDetallada, int, error) {
	if solicitanteID == "" {
		return nil, 0, ErrNoEsTuReserva
	}
	if f.Desde == nil || f.Hasta == nil || f.Hasta.Before(*f.Desde) || !f.Hasta.Before(f.Desde.AddDate(0, 0, 42)) {
		return nil, 0, ErrRangoCalendario
	}
	f.PorClases = true
	// También se consultan las nocturnas que comenzaron el día anterior.
	desde := f.Desde.AddDate(0, 0, -1)
	f.Desde = &desde
	f.CreadoPor = nil
	if soloMias {
		f.CreadoPor = &solicitanteID
	}
	return s.repo.ListarReservas(ctx, f)
}
