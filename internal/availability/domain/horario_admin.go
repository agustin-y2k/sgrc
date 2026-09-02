// Package domain contiene las entidades y reglas de negocio puras de
// availability — sin dependencias de infraestructura (sin *sql.DB, sin
// fiber.Ctx).
package domain

import (
	"errors"
	"fmt"
	"time"
)

// DiaSemana — mismo concepto que en reservation (ReglaRecurrencia), pero
// declarado acá también a propósito: cada paquete tiene su propio tipo de
// dominio, nunca se importa el domain/ de otro paquete (ver
// docs/06-arquitectura.md §3).
type DiaSemana string

// Los siete días.
const (
	Lunes     DiaSemana = "LUNES"
	Martes    DiaSemana = "MARTES"
	Miercoles DiaSemana = "MIERCOLES"
	Jueves    DiaSemana = "JUEVES"
	Viernes   DiaSemana = "VIERNES"
	Sabado    DiaSemana = "SABADO"
	Domingo   DiaSemana = "DOMINGO"
)

var ErrDiaSemanaInvalido = errors.New("día de la semana inválido")

func ParseDiaSemana(s string) (DiaSemana, error) {
	switch DiaSemana(s) {
	case Lunes, Martes, Miercoles, Jueves, Viernes, Sabado, Domingo:
		return DiaSemana(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrDiaSemanaInvalido, s)
	}
}

// goWeekdayADiaSemana traduce el time.Weekday de Go a nuestro enum, para
// poder comparar "ahora" contra los bloques cargados.
var goWeekdayADiaSemana = map[time.Weekday]DiaSemana{
	time.Monday:    Lunes,
	time.Tuesday:   Martes,
	time.Wednesday: Miercoles,
	time.Thursday:  Jueves,
	time.Friday:    Viernes,
	time.Saturday:  Sabado,
	time.Sunday:    Domingo,
}

// DiaYHoraDe traduce un instante real a los dos componentes que necesita el
// cálculo de disponibilidad (DisponibleAhora): el día de semana y el offset
// desde medianoche.
func DiaYHoraDe(ahora time.Time) (DiaSemana, time.Duration) {
	dia := goWeekdayADiaSemana[ahora.Weekday()]
	hora := time.Duration(ahora.Hour())*time.Hour + time.Duration(ahora.Minute())*time.Minute + time.Duration(ahora.Second())*time.Second
	return dia, hora
}

// FechaSolo descarta la hora de un time.Time, quedándose solo con el día —
// mismo criterio que reservation usa para Fecha (columna DATE, no TIMESTAMP):
// la hora es irrelevante para identificar "la excepción de hoy".
func FechaSolo(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Lo comparten el horario de los Admin y la jornada de la institución, con
// una diferencia que conviene tener presente: la jornada puede cruzar la
// medianoche (20:00–01:00) y el horario de un Admin no, porque el cálculo de
// "¿hay alguien ahora?" es de un solo día.
var ErrRangoHorarioInvalido = errors.New("la hora de fin no puede ser igual a la de inicio")

// ErrBloqueSolapado: dos bloques del mismo día que se pisan.
var ErrBloqueSolapado = errors.New("ese horario se pisa con otro bloque del mismo día")

// BloqueHorario es un tramo del patrón semanal recurrente de presencia de un
// Admin en el laboratorio (RF-07.1).
//
// No restringe ninguna operación que haga una persona —reservar, entregar y
// aprobar funcionan igual esté quien esté— pero desde la 1.18.0 **sí decide si
// el barrido automático actúa** (RF-07.6, ver mostrador.go). Un horario mal
// cargado ya no es solo una pantalla que miente.
type BloqueHorario struct {
	ID         string
	UsuarioID  string
	DiaSemana  DiaSemana
	HoraInicio time.Duration
	HoraFin    time.Duration
}

func NuevoBloqueHorario(id, usuarioID string, diaSemana DiaSemana, horaInicio, horaFin time.Duration) (*BloqueHorario, error) {
	if horaFin <= horaInicio {
		return nil, ErrRangoHorarioInvalido
	}
	return &BloqueHorario{
		ID: id, UsuarioID: usuarioID, DiaSemana: diaSemana,
		HoraInicio: horaInicio, HoraFin: horaFin,
	}, nil
}

// Cubre indica si este bloque incluye el día y la hora dados — rango
// [HoraInicio, HoraFin): el límite exacto de fin NO cuenta como cubierto
// (mismo criterio que el resto del sistema para rangos horarios, ver
// docs/07-modelo-datos.md).
func (b *BloqueHorario) Cubre(dia DiaSemana, horaActual time.Duration) bool {
	return b.DiaSemana == dia && horaActual >= b.HoraInicio && horaActual < b.HoraFin
}

// SeSolapaCon dice si dos bloques del mismo día pisan aunque sea un minuto.
func (b *BloqueHorario) SeSolapaCon(otro *BloqueHorario) bool {
	if b.DiaSemana != otro.DiaSemana {
		return false
	}
	return b.HoraInicio < otro.HoraFin && otro.HoraInicio < b.HoraFin
}

// PrimeroQueSeSolapa devuelve el bloque de la lista que pisa a este, o nil.
func (b *BloqueHorario) PrimeroQueSeSolapa(otros []*BloqueHorario) *BloqueHorario {
	for _, otro := range otros {
		if otro.ID == b.ID {
			continue
		}
		if b.SeSolapaCon(otro) {
			return otro
		}
	}
	return nil
}
