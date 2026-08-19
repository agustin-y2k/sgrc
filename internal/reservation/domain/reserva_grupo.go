// Package domain contiene las entidades y reglas de negocio puras de
// reservation — ReservaGrupo, Reserva y ReglaRecurrencia.
package domain

import (
	"errors"
	"fmt"
	"time"
)

// EstadoReservaGrupo (RF-04, docs/05-diagramas-estado.md).
type EstadoReservaGrupo string

const (
	GrupoConfirmada            EstadoReservaGrupo = "CONFIRMADA"
	GrupoParcialmenteCancelada EstadoReservaGrupo = "PARCIALMENTE_CANCELADA"
	GrupoCancelada             EstadoReservaGrupo = "CANCELADA"
	GrupoFinalizada            EstadoReservaGrupo = "FINALIZADA"
	// GrupoNoRetirado: NINGUNA de sus PCs se retiró.
	GrupoNoRetirado EstadoReservaGrupo = "NO_RETIRADA"
)

var ErrEstadoGrupoInvalido = errors.New("estado de reserva grupo inválido")

func ParseEstadoReservaGrupo(s string) (EstadoReservaGrupo, error) {
	switch EstadoReservaGrupo(s) {
	case GrupoConfirmada, GrupoParcialmenteCancelada, GrupoCancelada, GrupoFinalizada, GrupoNoRetirado:
		return EstadoReservaGrupo(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrEstadoGrupoInvalido, s)
	}
}

// PuedeTransicionarA: CONFIRMADA puede pasar a cualquiera de los otros tres
// (cancelación parcial de alguna PC, cancelación total, o finalización por el
// paso del tiempo).
func (e EstadoReservaGrupo) PuedeTransicionarA(nuevo EstadoReservaGrupo) bool {
	switch e {
	case GrupoConfirmada:
		return nuevo == GrupoParcialmenteCancelada || nuevo == GrupoCancelada ||
			nuevo == GrupoFinalizada || nuevo == GrupoNoRetirado
	case GrupoParcialmenteCancelada:
		return nuevo == GrupoCancelada || nuevo == GrupoFinalizada
	case GrupoCancelada, GrupoFinalizada, GrupoNoRetirado:
		return false
	default:
		return false
	}
}

var ErrTransicionGrupoInvalida = errors.New("transición de estado de reserva grupo inválida")

// ErrRangoHorarioInvalido: horaFin debe ser posterior a horaInicio — se
// valida en dominio para las dos entidades (ReservaGrupo y Reserva) y
// ReglaRecurrencia, así que se declara acá una sola vez y se reusa.
var ErrRangoHorarioInvalido = errors.New("la hora de fin no puede ser igual a la de inicio")

// ReservaGrupo es la reserva tal como la percibe el docente — materia, fecha,
// horario.
type ReservaGrupo struct {
	ID                    string
	MateriaID             string
	CreadoPor             *string
	NombreDocenteSnapshot string
	Fecha                 time.Time
	HoraInicio            time.Duration // offset desde medianoche, ver reserva.go
	HoraFin               time.Duration
	Estado                EstadoReservaGrupo
	ReglaRecurrenciaID    *string
	CreadaEn              time.Time
}

func NuevoReservaGrupo(id, materiaID string, creadoPor *string, nombreDocenteSnapshot string, fecha time.Time, horaInicio, horaFin time.Duration, reglaRecurrenciaID *string, ahora time.Time) (*ReservaGrupo, error) {
	if horaFin == horaInicio {
		return nil, ErrRangoHorarioInvalido
	}
	return &ReservaGrupo{
		ID:                    id,
		MateriaID:             materiaID,
		CreadoPor:             creadoPor,
		NombreDocenteSnapshot: nombreDocenteSnapshot,
		Fecha:                 fecha,
		HoraInicio:            horaInicio,
		HoraFin:               horaFin,
		Estado:                GrupoConfirmada,
		ReglaRecurrenciaID:    reglaRecurrenciaID,
		CreadaEn:              ahora,
	}, nil
}

func (g *ReservaGrupo) CambiarEstado(nuevo EstadoReservaGrupo) error {
	if !g.Estado.PuedeTransicionarA(nuevo) {
		return fmt.Errorf("%w: de %s a %s", ErrTransicionGrupoInvalida, g.Estado, nuevo)
	}
	g.Estado = nuevo
	return nil
}
