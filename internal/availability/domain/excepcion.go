package domain

import (
	"errors"
	"fmt"
	"time"
)

// TipoExcepcion — RF-07.4/07.5.
type TipoExcepcion string

const (
	NoDisponible      TipoExcepcion = "NO_DISPONIBLE"
	HorarioModificado TipoExcepcion = "HORARIO_MODIFICADO"
)

var ErrTipoExcepcionInvalido = errors.New("tipo de excepción inválido")

func ParseTipoExcepcion(s string) (TipoExcepcion, error) {
	switch TipoExcepcion(s) {
	case NoDisponible, HorarioModificado:
		return TipoExcepcion(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrTipoExcepcionInvalido, s)
	}
}

// ErrExcepcionIncoherente: NO_DISPONIBLE no lleva horario, HORARIO_MODIFICADO
// lo requiere completo — mismo chk_excepcion_horario_coherente de la base
// (docs/07-modelo-datos.md), validado también acá para devolver 400 antes
// de golpear la constraint.
var ErrExcepcionIncoherente = errors.New("horaInicio/horaFin no coinciden con el tipo de excepción")

// Excepcion cubre tanto una excepción planificada para una fecha puntual
// (RF-07.4: horario distinto o ausencia total ese día) como el atajo
// "marcarme no disponible ahora" (RF-07.5) — son la misma fila, con
// Fecha=hoy en el segundo caso. UNIQUE(usuario_id, fecha) se garantiza vía
// upsert en el repo, no acá.
type Excepcion struct {
	ID         string
	UsuarioID  string
	Fecha      time.Time
	Tipo       TipoExcepcion
	HoraInicio *time.Duration
	HoraFin    *time.Duration
	Motivo     *string
}

func NuevaExcepcion(id, usuarioID string, fecha time.Time, tipo TipoExcepcion, horaInicio, horaFin *time.Duration, motivo *string) (*Excepcion, error) {
	switch tipo {
	case NoDisponible:
		if horaInicio != nil || horaFin != nil {
			return nil, ErrExcepcionIncoherente
		}
	case HorarioModificado:
		if horaInicio == nil || horaFin == nil {
			return nil, ErrExcepcionIncoherente
		}
		if *horaFin <= *horaInicio {
			return nil, ErrRangoHorarioInvalido
		}
	default:
		return nil, ErrTipoExcepcionInvalido
	}

	return &Excepcion{
		ID: id, UsuarioID: usuarioID, Fecha: FechaSolo(fecha), Tipo: tipo,
		HoraInicio: horaInicio, HoraFin: horaFin, Motivo: motivo,
	}, nil
}

// DisponibleAhora resuelve si, asumiendo que ESTA excepción rige para hoy
// (siempre pisa el patrón semanal — ver docs/07-modelo-datos.md §2), el
// admin cuenta como disponible a la hora indicada. Rango [HoraInicio,
// HoraFin) para HORARIO_MODIFICADO, igual criterio que BloqueHorario.Cubre.
func (e *Excepcion) DisponibleAhora(horaActual time.Duration) bool {
	if e.Tipo == NoDisponible {
		return false
	}
	return horaActual >= *e.HoraInicio && horaActual < *e.HoraFin
}
