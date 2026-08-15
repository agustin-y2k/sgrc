package domain

import (
	"errors"
	"sort"
	"time"
)

// La jornada institucional es el horario en que la escuela está abierta:
// qué días y entre qué horas se puede usar el laboratorio.
//
// No confundirla con BloqueHorario, que está en este mismo paquete y se le
// parece mucho. Son dos cosas distintas y por eso no comparten tabla:
//
//   - BloqueHorario es de UN Admin: "yo estoy los martes de 8 a 12". Es
//     informativo, sirve para que un docente sepa a quién buscar, y no
//     habilita ni impide nada.
//   - BloqueJornada es de LA INSTITUCIÓN: "la escuela abre los martes de 7 a
//     22". Es normativo: una reserva fuera de la jornada se rechaza.
//
// Antes esto no existía y la jornada estaba hardcodeada como "lunes a
// viernes", lo cual dejaba afuera a las escuelas de jornada extendida o
// albergue —que dictan el fin de semana— y no decía nada de las horas. Ahora
// cada institución declara la suya.
//
// Varios bloques por día a propósito: una escuela con turno mañana y turno
// noche puede declarar 07:00–12:00 y 18:00–23:00, y dejar el mediodía
// afuera. Con un solo rango por día habría que abrir de 7 a 23 y perder esa
// distinción.
type BloqueJornada struct {
	ID         string
	DiaSemana  DiaSemana
	HoraInicio time.Duration
	HoraFin    time.Duration
}

// ErrBloqueJornadaSolapado: dos bloques del mismo día que se pisan. Mismo
// criterio que el horario de los Admin — se rechaza en vez de fusionarlos,
// porque fusionar silenciosamente hace que la pantalla muestre algo distinto
// de lo que la persona cargó.
var ErrBloqueJornadaSolapado = errors.New("ese bloque se superpone con otro del mismo día")

// La jornada también cruza la medianoche: una nocturna abre de 20:00 a
// 01:00. Misma regla que las reservas —hora_fin menor que hora_inicio
// significa "termina al día siguiente"— y por la misma razón: si la jornada
// no pudiera expresarlo, una escuela que dicta hasta la una de la mañana
// tendría que declarar 20:00–23:59 y sus propias clases nocturnas quedarían
// fuera de su propio horario.
func NuevoBloqueJornada(id string, dia DiaSemana, horaInicio, horaFin time.Duration) (*BloqueJornada, error) {
	if horaFin == horaInicio {
		return nil, ErrRangoHorarioInvalido
	}
	return &BloqueJornada{ID: id, DiaSemana: dia, HoraInicio: horaInicio, HoraFin: horaFin}, nil
}

// FinRelativo es la hora de fin medida desde la medianoche del día que nombra
// al tramo, así que pasa de las 24 horas cuando cruza: 20:00–01:00 es
// [20h, 25h). Comparar hora_fin cruda daría 01:00 y pondría el fin antes del
// inicio.
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
//
// Recibe TODOS los bloques de la jornada, no solo los del día, y eso es
// deliberado: hay que distinguir dos situaciones que se ven parecidas y
// significan lo contrario.
//
//   - Jornada vacía (ningún bloque, ningún día) = la institución todavía no
//     declaró nada. No hay restricción. Es lo único honesto que puede hacer
//     un sistema al que no le dijeron en qué ámbito lo instalaron: inventar
//     un lunes-a-viernes por defecto es volver a suponer.
//   - Jornada declarada pero sin bloques ESE día = la escuela no abre ese
//     día. Se rechaza.
//
// Si solo recibiera los bloques del día, las dos serían una lista vacía.
func PermiteReserva(jornada []*BloqueJornada, dia DiaSemana, horaInicio, horaFin time.Duration) bool {
	if len(jornada) == 0 {
		return true
	}

	tramos := tramosDelDia(jornada, dia)
	if len(tramos) == 0 {
		return false
	}

	// La reserva tiene que entrar ENTERA en un tramo. No alcanza con que
	// empiece dentro de uno: una reserva de 11:00 a 19:00 contra una jornada
	// de 07:00–12:00 y 18:00–23:00 pediría el laboratorio durante seis horas
	// en que la escuela está cerrada.
	//
	// Los dos extremos se miden desde la misma medianoche, así que pasan de
	// las 24 horas cuando cruzan. Una clase de 22:00 a 01:00 es [22h, 25h) y
	// entra en una jornada de 20:00 a 02:00, que es [20h, 26h).
	finReserva := horaInicio + duracionDe(horaInicio, horaFin)
	for _, t := range tramos {
		if horaInicio >= t.desde && finReserva <= t.hasta {
			return true
		}
	}
	return false
}

// duracionDe es el gemelo de reservation/domain.DuracionDe. Está duplicado a
// propósito: cada paquete tiene su propio dominio y ninguno importa el del
// otro (docs/06-arquitectura.md §3). Son ocho líneas y la alternativa era un
// paquete compartido de aritmética horaria, que acopla los dos dominios para
// ahorrar menos de lo que cuesta.
func duracionDe(horaInicio, horaFin time.Duration) time.Duration {
	if horaFin < horaInicio {
		return 24*time.Hour - horaInicio + horaFin
	}
	return horaFin - horaInicio
}

// tramo es un intervalo del día en horas desde la medianoche, con el fin
// pudiendo pasar de 24: un tramo nocturno de 20:00 a 01:00 es [20h, 25h).
//
// Es un tipo aparte y no un BloqueJornada porque un tramo fusionado no es un
// bloque: no tiene ID, no está en la base y nadie lo cargó. Reusar la
// estructura invitaba a devolverlo por la API como si fuera uno.
type tramo struct {
	desde, hasta time.Duration
}

// tramosDelDia devuelve los tramos de ese día, ordenados y con los contiguos
// fusionados.
//
// La fusión importa para el caso de borde: una escuela que carga 07:00–12:00
// y 12:00–18:00 —dos turnos que se tocan— describe un día abierto de 7 a 18,
// y una reserva de 11:00 a 13:00 tiene que poder hacerse. Sin fusionar, no
// entraría entera en ninguno de los dos y se rechazaría sin motivo visible.
//
// Todo se mide en tiempo relativo a la medianoche del día del tramo, y por
// eso el fin puede pasar de 24 horas. Comparar las horas crudas rompía con
// los tramos nocturnos: 01:00 es menor que casi cualquier inicio, así que un
// tramo de 20:00 a 01:00 se leía al revés y no fusionaba ni contenía nada.
//
// Los solapados no se contemplan acá porque se rechazan al cargarlos
// (ErrBloqueJornadaSolapado); si alguno entrara igual escribiendo directo en
// la base, este mismo código lo absorbe sin romperse.
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
