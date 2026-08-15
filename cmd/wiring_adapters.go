// Adaptadores de composición: viven acá (no en internal/inventory ni en
// internal/auth) a propósito. Son el único lugar del proyecto donde está
// permitido que un paquete de dominio "conozca" a otro — cmd/ es la raíz
// de composición, arma el grafo de dependencias completo, y ningún otro
// paquete debe replicar este conocimiento (ver docs/06-arquitectura.md §3).
//
// Existen porque las cascadas de cancelación (RF-02.8, RF-03.8/03.9) son
// ACCIONES con reglas de negocio reales (una reserva ya cancelada no se
// puede volver a cancelar, el ReservaGrupo padre recalcula su estado,
// etc.) — reimplementar esa máquina de estados con SQL crudo en cada
// paquete consumidor sería duplicar lógica con riesgo real de que
// diverja. Los validadores de solo LECTURA (ValidadorUsuario,
// ValidadorMateria, ValidadorEquipo, etc.) sí van directo por SQL en cada
// paquete, porque ahí no hay ninguna regla de negocio que valga la pena
// centralizar — la diferencia es lectura vs. acción, no una excepción
// arbitraria a la regla de "no importar entre paquetes".
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
// reservation/application.Service — inventory/ nunca importa
// reservation/ directamente.
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
// directamente. Usado por la cascada de DarDeBaja (RF-02.8).
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
//
// Los dos pasos quedan expuestos por separado a propósito: el orden en que
// se ejecutan es una regla de negocio (el borrado va último porque es
// irreversible) y vive en academic/application.ArchivarYClonar, no
// escondida acá adentro.
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
// availability/application.Service — reservation/ nunca importa
// availability/ directamente.
//
// Une las dos mitades del cambio que sacó "lunes a viernes" del código:
// availability sabe qué días y horas declaró la institución, reservation
// necesita saber si una reserva entra ahí, y ninguno de los dos tiene por
// qué conocer al otro.
type reservationValidadorJornadaAdapter struct {
	availabilitySvc *availabilityapp.Service
}

func (a *reservationValidadorJornadaAdapter) PermiteReserva(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) (bool, error) {
	return a.availabilitySvc.PermiteReserva(ctx, fecha, horaInicio, horaFin)
}
