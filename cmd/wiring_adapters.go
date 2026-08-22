// Adaptadores de composición: viven acá (no en internal/inventory ni en
// internal/auth) a propósito.
package main

import (
	"context"
	"errors"
	"time"

	availabilityapp "github.com/ramiro/sgrc/internal/availability/application"
	reportingapp "github.com/ramiro/sgrc/internal/reporting/application"
	reservationapp "github.com/ramiro/sgrc/internal/reservation/application"
)

// inventoryValidadorReservasAdapter satisface
// inventory/application.ValidadorReservas envolviendo
// reservation/application.Service — inventory/ nunca importa reservation/
// directamente.
type inventoryValidadorReservasAdapter struct {
	reservationSvc *reservationapp.Service
}

func (a *inventoryValidadorReservasAdapter) CancelarReservasFuturasDeEquipo(ctx context.Context, equipoID, motivo string) (int, int, error) {
	return a.reservationSvc.CancelarReservasFuturasDeEquipo(ctx, equipoID, motivo)
}

func (a *inventoryValidadorReservasAdapter) TieneReservasFuturas(ctx context.Context, equipoID string) (bool, error) {
	return a.reservationSvc.TieneReservasFuturasDeEquipo(ctx, equipoID)
}

func (a *inventoryValidadorReservasAdapter) EstaPrestado(ctx context.Context, equipoID string) (bool, error) {
	return a.reservationSvc.EstaPrestado(ctx, equipoID)
}

// authCanceladorReservasAdapter satisface
// auth/application.CanceladorReservasDeMateria envolviendo
// reservation/application.Service — auth/ nunca importa reservation/
// directamente.
type authCanceladorReservasAdapter struct {
	reservationSvc *reservationapp.Service
}

func (a *authCanceladorReservasAdapter) CancelarReservasFuturasDeMateria(ctx context.Context, materiaID, motivo string) (int, error) {
	return a.reservationSvc.CancelarReservasFuturasDeMateria(ctx, materiaID, motivo)
}

// academicArchivadorHistoricoAdapter satisface
// academic/application.ArchivadorHistorico envolviendo TANTO
// reporting/application.Service COMO reservation/application.Service —
// academic/ nunca importa ninguno de los dos directamente.
type academicArchivadorHistoricoAdapter struct {
	reportingSvc   *reportingapp.Service
	reservationSvc *reservationapp.Service
}

func (a *academicArchivadorHistoricoAdapter) GuardarSnapshotDeCiclo(ctx context.Context, cicloID string, anio int) error {
	return a.reportingSvc.ArchivarSnapshotDeCiclo(ctx, cicloID, anio)
}

func (a *academicArchivadorHistoricoAdapter) EliminarReservasDeCiclo(ctx context.Context, cicloID string) error {
	_, _, err := a.reservationSvc.EliminarReservasDeCiclo(ctx, cicloID)
	return err
}

// reservationValidadorJornadaAdapter satisface
// reservation/application.ValidadorJornada envolviendo
// availability/application.Service — reservation/ nunca importa availability/
// directamente.
type reservationValidadorJornadaAdapter struct {
	availabilitySvc *availabilityapp.Service
}

func (a *reservationValidadorJornadaAdapter) PermiteReserva(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) (bool, error) {
	return a.availabilitySvc.PermiteReserva(ctx, fecha, horaInicio, horaFin)
}

func (a *reservationValidadorJornadaAdapter) CierreDeLaJornada(ctx context.Context, fecha time.Time) (reservationapp.CierreDeJornada, error) {
	declarada, abre, fin, err := a.availabilitySvc.CierreDeLaJornada(ctx, fecha)
	if err != nil {
		return reservationapp.CierreDeJornada{}, err
	}
	return reservationapp.CierreDeJornada{Declarada: declarada, Abre: abre, Fin: fin}, nil
}

// availabilityReservasAdapter satisface
// availability/application.ReservasDeLaInstitucion envolviendo
// reservation/application.Service.
//
// Es la flecha inversa de reservationValidadorJornadaAdapter, y no es un
// error: reservation le pregunta a availability si un horario entra en la
// jornada, y availability le pregunta a reservation qué quedaría afuera si la
// jornada cambiara. Son dos preguntas distintas y ninguna se contesta con la
// otra. El ciclo existe solo en tiempo de ejecución; en compilación cada
// módulo depende únicamente de un puerto propio, y las dos puntas se atan
// acá.
type availabilityReservasAdapter struct {
	reservationSvc *reservationapp.Service
}

// errSinCablear protege el único momento frágil del arranque: entre que se
// construye este adaptador y que se le asigna reservationSvc hay treinta
// líneas de main.go, y quien las reordene rompería el cambio de jornada con un
// panic a los seis meses. Con esto falla el request, no el proceso, y el
// mensaje dice exactamente qué pasó.
var errSinCablear = errors.New("el adaptador hacia reservation todavía no está cableado (ver el orden de construcción en main.go)")

func (a *availabilityReservasAdapter) listo() error {
	if a == nil || a.reservationSvc == nil {
		return errSinCablear
	}
	return nil
}

func (a *availabilityReservasAdapter) ReservasFuturas(ctx context.Context, desde time.Time) ([]availabilityapp.ReservaFutura, error) {
	if err := a.listo(); err != nil {
		return nil, err
	}
	detalladas, err := a.reservationSvc.ReservasFuturas(ctx, desde)
	if err != nil {
		return nil, err
	}
	futuras := make([]availabilityapp.ReservaFutura, len(detalladas))
	for i, d := range detalladas {
		futuras[i] = availabilityapp.ReservaFutura{
			ID:         d.Reserva.ID,
			Fecha:      d.Reserva.Fecha,
			HoraInicio: d.Reserva.HoraInicio,
			HoraFin:    d.Reserva.HoraFin,
			Equipo:     d.Etiqueta,
			Materia:    d.MateriaNombre,
			Docente:    textoDe(d.Reserva.NombreDocenteSnapshot),
		}
	}
	return futuras, nil
}

func (a *availabilityReservasAdapter) PrestamosAbiertos(ctx context.Context) ([]availabilityapp.PrestamoAbierto, error) {
	if err := a.listo(); err != nil {
		return nil, err
	}
	abiertos, err := a.reservationSvc.ListarPrestamosAbiertos(ctx)
	if err != nil {
		return nil, err
	}
	prestamos := make([]availabilityapp.PrestamoAbierto, len(abiertos))
	for i, p := range abiertos {
		prestamos[i] = availabilityapp.PrestamoAbierto{
			ID:                 p.Prestamo.ID,
			Equipo:             p.Etiqueta,
			Quien:              p.Prestamo.EntregadoANombre,
			DevolucionEstimada: p.Prestamo.DevolucionEstimada,
		}
	}
	return prestamos, nil
}

func (a *availabilityReservasAdapter) CancelarReservas(ctx context.Context, reservaIDs []string, motivo string) (int, error) {
	if err := a.listo(); err != nil {
		return 0, err
	}
	return a.reservationSvc.CancelarReservasPorIDs(ctx, reservaIDs, motivo)
}

// textoDe desreferencia un texto opcional. El nombre del docente es nulo en
// los bloqueos administrativos, que igual no llegan hasta acá.
func textoDe(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
