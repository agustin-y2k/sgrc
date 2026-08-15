package domain

import (
	"errors"
	"fmt"
	"time"
)

// DiaSemana — mismo concepto que en horario_admin (availability), pero
// declarado acá también a propósito: cada paquete tiene su propio tipo de
// dominio, nunca se importa el domain/ de otro paquete (ver
// docs/06-arquitectura.md §3).
type DiaSemana string

// La semana tiene siete días y el sistema no supone cuáles usa la
// institución. Antes el enum frenaba en viernes y una regla fija rechazaba
// sábados y domingos, lo cual dejaba afuera a dos clases de escuela que
// existen y son mayoría en algunos lugares: las de jornada extendida o
// albergue, que dictan el fin de semana, y las nocturnas, cuyo horario no se
// parece al de la mañana.
//
// Qué días abre cada institución dejó de ser una constante del código y pasó
// a ser un dato que el Admin declara (ver JornadaInstitucional en
// availability). Sin jornada declarada no hay restricción: el sistema no
// inventa un calendario que nadie le dijo.
//
// Los feriados y el receso NO se modelan — se resuelven a mano (ver
// docs/01-requisitos.md).
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

// diaSemanaAGoWeekday traduce nuestro enum al time.Weekday de Go, para
// poder comparar contra fechas reales al generar ocurrencias.
var diaSemanaAGoWeekday = map[DiaSemana]time.Weekday{
	Lunes:     time.Monday,
	Martes:    time.Tuesday,
	Miercoles: time.Wednesday,
	Jueves:    time.Thursday,
	Viernes:   time.Friday,
	Sabado:    time.Saturday,
	Domingo:   time.Sunday,
}

var ErrRangoFechasInvalido = errors.New("la fecha de fin debe ser igual o posterior a la fecha de inicio")

// ReglaRecurrencia es el patrón de una reserva recurrente (RF-04.2) — se
// materializa en N ReservaGrupo, uno por cada ocurrencia del día de semana
// elegido dentro del rango de fechas.
type ReglaRecurrencia struct {
	ID          string
	MateriaID   string
	CreadoPor   string
	DiaSemana   DiaSemana
	HoraInicio  time.Duration
	HoraFin     time.Duration
	FechaInicio time.Time
	FechaFin    time.Time
}

func NuevaReglaRecurrencia(id, materiaID, creadoPor string, diaSemana DiaSemana, horaInicio, horaFin time.Duration, fechaInicio, fechaFin time.Time) (*ReglaRecurrencia, error) {
	if horaFin == horaInicio {
		return nil, ErrRangoHorarioInvalido
	}
	if fechaFin.Before(fechaInicio) {
		return nil, ErrRangoFechasInvalido
	}
	return &ReglaRecurrencia{
		ID: id, MateriaID: materiaID, CreadoPor: creadoPor, DiaSemana: diaSemana,
		HoraInicio: horaInicio, HoraFin: horaFin, FechaInicio: fechaInicio, FechaFin: fechaFin,
	}, nil
}

// GenerarFechas devuelve todas las fechas entre FechaInicio y FechaFin
// (ambas inclusive) que caen en el día de semana de esta regla — una por
// cada ReservaGrupo que application/ debe materializar (RF-04.2).
func (r *ReglaRecurrencia) GenerarFechas() []time.Time {
	objetivo := diaSemanaAGoWeekday[r.DiaSemana]
	var fechas []time.Time

	for d := r.FechaInicio; !d.After(r.FechaFin); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == objetivo {
			fechas = append(fechas, d)
		}
	}
	return fechas
}
