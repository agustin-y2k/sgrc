package domain

import "time"

// ¿Hay alguien atendiendo el mostrador? (RF-07.6)
//
// Hasta la 1.18.0 el horario de los Admin era puramente informativo: servía
// para que un docente supiera a quién buscar. Ahora además es la condición de
// la que depende el barrido, y la razón es que el barrido no puede distinguir
// por su cuenta dos situaciones que en la base se ven idénticas:
//
//   - nadie vino a buscar las máquinas reservadas, y
//   - vinieron, se las llevaron, y no había ningún Admin para registrarlo.
//
// En el segundo caso todo lo que el barrido concluye es falso, y el
// perjudicado es el docente, que hizo todo bien. Si el que cubre el mostrador
// —un directivo, un preceptor— prefiere anotar en papel, el sistema no tiene
// por qué castigar a nadie por eso: se queda quieto, como si estuviera
// apagado, hasta que vuelva a haber alguien operándolo.

// Tramo es un rango horario efectivo dentro de un día, medido desde la
// medianoche.
type Tramo struct {
	Desde time.Duration
	Hasta time.Duration
}

// TramosDelDia son los tramos que un Admin efectivamente atiende ese día:
// la excepción si cargó una —que pisa por completo el patrón semanal, misma
// regla que DisponibleAhora— o sus bloques semanales si no.
//
// Devuelve nil cuando ese Admin no atiende nada ese día, que es lo que
// distingue "declaró que no viene" de "no declaró nada".
func TramosDelDia(bloques []*BloqueHorario, excepcionDelDia *Excepcion, dia DiaSemana) []Tramo {
	if excepcionDelDia != nil {
		if excepcionDelDia.Tipo == NoDisponible {
			return nil
		}
		return []Tramo{{Desde: *excepcionDelDia.HoraInicio, Hasta: *excepcionDelDia.HoraFin}}
	}

	var tramos []Tramo
	for _, b := range bloques {
		if b.DiaSemana == dia {
			tramos = append(tramos, Tramo{Desde: b.HoraInicio, Hasta: b.HoraFin})
		}
	}
	return tramos
}

// CubreLaHora dice si alguno de esos tramos incluye ese instante del día.
// Rango [Desde, Hasta), igual que BloqueHorario.Cubre.
func CubreLaHora(tramos []Tramo, hora time.Duration) bool {
	for _, t := range tramos {
		if hora >= t.Desde && hora < t.Hasta {
			return true
		}
	}
	return false
}

// DeclaroHorario dice si esa persona tiene algún bloque semanal cargado, sin
// importar de qué día.
//
// Es lo que separa "el mostrador no está atendido ahora" de "en esta escuela
// nadie declaró nunca sus horarios". Sin esta distinción, el día que se
// despliega esta versión el barrido entero se apagaría solo y en silencio en
// cualquier instalación que todavía no cargó los horarios — que es
// exactamente el modo de falla que un sistema no puede tener.
func DeclaroHorario(bloques []*BloqueHorario) bool {
	return len(bloques) > 0
}
