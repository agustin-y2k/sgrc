package domain

import (
	"errors"
	"sort"
	"time"
)

// La jornada institucional es el horario en que la escuela está abierta: qué
// días y entre qué horas se puede usar el laboratorio.
type BloqueJornada struct {
	ID         string
	DiaSemana  DiaSemana
	HoraInicio time.Duration
	HoraFin    time.Duration
}

// ErrBloqueJornadaSolapado: dos bloques del mismo día que se pisan.
var ErrBloqueJornadaSolapado = errors.New("ese bloque se superpone con otro del mismo día")

// La jornada también cruza la medianoche: una nocturna abre de 20:00 a 01:00.
// Misma regla que las reservas —hora_fin menor que hora_inicio significa
// "termina al día siguiente"— y por la misma razón: si la jornada no pudiera
// expresarlo, una escuela que dicta hasta la una de la mañana tendría que
// declarar 20:00–23:59 y sus propias clases nocturnas quedarían fuera de su
// propio horario.
func NuevoBloqueJornada(id string, dia DiaSemana, horaInicio, horaFin time.Duration) (*BloqueJornada, error) {
	if horaFin == horaInicio {
		return nil, ErrRangoHorarioInvalido
	}
	return &BloqueJornada{ID: id, DiaSemana: dia, HoraInicio: horaInicio, HoraFin: horaFin}, nil
}

// FinRelativo es la hora de fin medida desde la medianoche del día que nombra
// al tramo, así que pasa de las 24 horas cuando cruza: 20:00–01:00 es [20h,
// 25h).
func (b *BloqueJornada) FinRelativo() time.Duration {
	if b.HoraFin < b.HoraInicio {
		return b.HoraFin + 24*time.Hour
	}
	return b.HoraFin
}

// SolapaCon dice si dos tramos del mismo día se pisan. Tocarse no es
// pisarse: 07:00–12:00 y 12:00–18:00 son contiguos y válidos.
func (b *BloqueJornada) SolapaCon(otro *BloqueJornada) bool {
	if b.DiaSemana != otro.DiaSemana {
		return false
	}
	return b.HoraInicio < otro.FinRelativo() && otro.HoraInicio < b.FinRelativo()
}

// PermiteReserva dice si un bloque (día + rango horario) cae dentro de la
// jornada declarada por la institución.
func PermiteReserva(jornada []*BloqueJornada, dia DiaSemana, horaInicio, horaFin time.Duration) bool {
	if len(jornada) == 0 {
		return true
	}

	tramos := tramosDelDia(jornada, dia)
	if len(tramos) == 0 {
		return false
	}

	// La reserva tiene que entrar ENTERA en un tramo.
	finReserva := horaInicio + duracionDe(horaInicio, horaFin)
	for _, t := range tramos {
		if horaInicio >= t.desde && finReserva <= t.hasta {
			return true
		}
	}
	return false
}

// MomentoDentroDeLaJornada dice si un instante puntual cae en algún tramo
// abierto de ese día.
//
// Es la pregunta que hace falta para un préstamo: su devolución pactada es un
// momento, no un rango. Existe aparte de PermiteReserva —y no como una
// reserva de duración cero— porque un tramo de cero horas es justamente lo
// que NuevoBloqueJornada rechaza por inválido: apoyarse en que el cálculo de
// duración se porte bien en ese borde sería apoyarse en un accidente.
//
// Jornada vacía = sin restricción, igual que en PermiteReserva.
func MomentoDentroDeLaJornada(jornada []*BloqueJornada, dia DiaSemana, hora time.Duration) bool {
	if len(jornada) == 0 {
		return true
	}
	for _, t := range tramosDelDia(jornada, dia) {
		// Cerrado en los dos extremos: devolver justo a la hora de cierre es
		// devolver en horario.
		if hora >= t.desde && hora <= t.hasta {
			return true
		}
	}
	return false
}

// CierreDe dice a qué hora termina la jornada de ese día, medida desde la
// medianoche del día que nombra al tramo.
//
// Pasa de las 24 horas cuando el último tramo cruza: una nocturna que declara
// el lunes de 20:00 a 01:00 cierra su lunes a las 25h, o sea a la 01:00 del
// martes. Devolverlo así —y no como "01:00"— es lo que permite sumarle la
// hora de gracia sin tener que saber de qué día se está hablando.
//
// El segundo valor es false cuando ese día la escuela no abre. Con la jornada
// sin declarar también es false: no hay de dónde deducir un cierre, y suponer
// uno sería volver a inventar el calendario que este modelo vino a sacar.
func CierreDe(jornada []*BloqueJornada, dia DiaSemana) (time.Duration, bool) {
	tramos := tramosDelDia(jornada, dia)
	if len(tramos) == 0 {
		return 0, false
	}
	// Los tramos vienen ordenados y fusionados, así que el cierre es el fin
	// del último: una escuela de turno mañana y turno noche cierra cuando
	// termina la noche, no cuando termina la mañana.
	return tramos[len(tramos)-1].hasta, true
}

// duracionDe es el gemelo de reservation/domain.DuracionDe.
func duracionDe(horaInicio, horaFin time.Duration) time.Duration {
	if horaFin < horaInicio {
		return 24*time.Hour - horaInicio + horaFin
	}
	return horaFin - horaInicio
}

// tramo es un intervalo del día en horas desde la medianoche, con el fin
// pudiendo pasar de 24: un tramo nocturno de 20:00 a 01:00 es [20h, 25h).
type tramo struct {
	desde, hasta time.Duration
}

// tramosDelDia devuelve los tramos de ese día, ordenados y con los contiguos
// fusionados.
func tramosDelDia(jornada []*BloqueJornada, dia DiaSemana) []tramo {
	var delDia []tramo
	for _, b := range jornada {
		if b.DiaSemana == dia {
			delDia = append(delDia, tramo{desde: b.HoraInicio, hasta: b.FinRelativo()})
		}
	}
	if len(delDia) == 0 {
		return nil
	}

	sort.Slice(delDia, func(i, j int) bool { return delDia[i].desde < delDia[j].desde })

	fusionados := []tramo{delDia[0]}
	for _, t := range delDia[1:] {
		ultimo := &fusionados[len(fusionados)-1]
		if t.desde <= ultimo.hasta {
			if t.hasta > ultimo.hasta {
				ultimo.hasta = t.hasta
			}
			continue
		}
		fusionados = append(fusionados, t)
	}
	return fusionados
}
