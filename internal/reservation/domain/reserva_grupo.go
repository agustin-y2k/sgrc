// Package domain contiene las entidades y reglas de negocio puras de
// reservation — ReservaGrupo, Reserva y ReglaRecurrencia. Ver
// docs/03-diagrama-clases.md, docs/05-diagramas-estado.md y
// docs/01-requisitos.md RF-04 para el detalle funcional completo.
package domain

import (
	"errors"
	"fmt"
	"time"
)

// EstadoReservaGrupo (RF-04, docs/05-diagramas-estado.md). Un ReservaGrupo
// es lo que el docente percibe como "mi reserva" — una materia, fecha y
// horario, que por debajo puede involucrar varios equipos (una Reserva cada
// una).
type EstadoReservaGrupo string

const (
	GrupoConfirmada            EstadoReservaGrupo = "CONFIRMADA"
	GrupoParcialmenteCancelada EstadoReservaGrupo = "PARCIALMENTE_CANCELADA"
	GrupoCancelada             EstadoReservaGrupo = "CANCELADA"
	GrupoFinalizada            EstadoReservaGrupo = "FINALIZADA"
	// GrupoNoRetirado: NINGUNA de sus PCs se retiró. Si el docente vino y se
	// llevó tres de cinco, el grupo NO pasa por acá: vino a dar la clase, y
	// lo que pasó con las otras dos máquinas se ve fila por fila.
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

// PuedeTransicionarA: CONFIRMADA puede pasar a cualquiera de los otros
// tres (cancelación parcial de alguna PC, cancelación total, o
// finalización por el paso del tiempo). PARCIALMENTE_CANCELADA puede
// terminar de cancelarse del todo, o finalizar (las PCs que sí se usaron
// llegaron a su horario). CANCELADA y FINALIZADA son terminales.
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
// El mensaje ya no dice "posterior": una clase nocturna de 22:00 a 01:00
// tiene una hora de fin ANTERIOR, y eso es válido — significa que termina al
// día siguiente. Lo único que se rechaza es que sean iguales.
var ErrRangoHorarioInvalido = errors.New("la hora de fin no puede ser igual a la de inicio")

// ReservaGrupo es la reserva tal como la percibe el docente — materia,
// fecha, horario. Por debajo, una o más Reserva (una por PC) son las que
// realmente ocupan el recurso físico.
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
