// Package domain contiene las entidades y reglas de negocio puras de
// availability — sin dependencias de infraestructura (sin *sql.DB, sin
// fiber.Ctx). Horario de disponibilidad de Admins, puramente informativo
// (RF-07). Ver docs/03-diagrama-clases.md para las entidades y
// docs/01-requisitos.md para el detalle funcional.
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

// La semana lectiva de la escuela es de lunes a viernes: si no hay clase,
// publicar un horario de presencia ese día no le sirve a nadie. SABADO
// estaba acá de antes de esa decisión —reservation ya lo había sacado en la
// migración 002— así que availability aceptaba un día que el resto del
// sistema no reconoce, y el frontend tenía que esquivar esas filas al
// mostrarlas. La migración 005 pone el mismo límite en la base.
const (
	Lunes     DiaSemana = "LUNES"
	Martes    DiaSemana = "MARTES"
	Miercoles DiaSemana = "MIERCOLES"
	Jueves    DiaSemana = "JUEVES"
	Viernes   DiaSemana = "VIERNES"
)

var ErrDiaSemanaInvalido = errors.New("día de la semana inválido")

func ParseDiaSemana(s string) (DiaSemana, error) {
	switch DiaSemana(s) {
	case Lunes, Martes, Miercoles, Jueves, Viernes:
		return DiaSemana(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrDiaSemanaInvalido, s)
	}
}

// goWeekdayADiaSemana traduce el time.Weekday de Go a nuestro enum, para
// poder comparar "ahora" contra los bloques cargados. El fin de semana
// queda sin mapeo a propósito: no son días hábiles del sistema, así que un
// "ahora" en sábado o domingo nunca matchea ningún bloque semanal cargado —
// DiaYHoraDe devuelve DiaSemana("") esos días, y DisponibleAhora responde
// que no hay nadie disponible, que es la verdad.
var goWeekdayADiaSemana = map[time.Weekday]DiaSemana{
	time.Monday:    Lunes,
	time.Tuesday:   Martes,
	time.Wednesday: Miercoles,
	time.Thursday:  Jueves,
	time.Friday:    Viernes,
}

// DiaYHoraDe traduce un instante real a los dos componentes que necesita
// el cálculo de disponibilidad (DisponibleAhora): el día de semana y el
// offset desde medianoche.
func DiaYHoraDe(ahora time.Time) (DiaSemana, time.Duration) {
	dia := goWeekdayADiaSemana[ahora.Weekday()]
	hora := time.Duration(ahora.Hour())*time.Hour + time.Duration(ahora.Minute())*time.Minute + time.Duration(ahora.Second())*time.Second
	return dia, hora
}

// FechaSolo descarta la hora de un time.Time, quedándose solo con el día
// — mismo criterio que reservation usa para Fecha (columna DATE, no
// TIMESTAMP): la hora es irrelevante para identificar "la excepción de
// hoy".
func FechaSolo(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

var ErrRangoHorarioInvalido = errors.New("la hora de fin debe ser posterior a la hora de inicio")

// BloqueHorario es un tramo del patrón semanal recurrente de presencia de
// un Admin en el laboratorio (RF-07.1) — puramente informativo, sin efecto
// sobre permisos ni reservas.
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
