// Adaptadores de composición: viven acá (no en internal/inventory ni en
// internal/auth) a propósito.
package main

import (
	"context"
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
