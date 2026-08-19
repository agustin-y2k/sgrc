package domain

import "time"

// Las reglas de tiempo del barrido (RF-08.10 a RF-08.13): cuándo hay que
// recordar una reserva, cuándo liberarla porque nadie la retiró, y cuándo
// avisarle al docente siguiente que una máquina no volvió.

// Valores por defecto. Los tres son configurables por entorno; acá están
// para documentar de dónde salen y para que los tests no dependan del .env.
const (
	// GraciaDeRetiroPorDefecto: cuarenta minutos. Es lo que la escuela venía
	// tolerando en la práctica antes de que existiera el sistema.
	GraciaDeRetiroPorDefecto = 40 * time.Minute
	// AntelacionDelRecordatorio: una hora antes. Alcanza para que el docente
	// llegue, avise, o modifique la reserva antes de que empiece.
	AntelacionDelRecordatorio = time.Hour
	// DemoraDelAvisoDeNoRetiroPorDefecto: quince minutos después de que empezó
	// la clase.
	DemoraDelAvisoDeNoRetiroPorDefecto = 15 * time.Minute
	// GraciaTrasEntregaParcialPorDefecto: quince minutos desde que el Admin
	// anotó la entrega.
	GraciaTrasEntregaParcialPorDefecto = 15 * time.Minute
	// DemoraParaReclamarPorDefecto: diez minutos después de la hora en que la
	// máquina tenía que volver.
	DemoraParaReclamarPorDefecto = 10 * time.Minute
)

// CorrespondeRecordar dice si ya es hora de mandarle al docente el aviso de
// "en un rato tenés reserva".
func CorrespondeRecordar(fecha time.Time, horaInicio, horaFin time.Duration, antelacion time.Duration, ahora time.Time) bool {
	if YaTermino(fecha, horaInicio, horaFin, ahora) {
		return false
	}
	return !horaDePared(ahora, horaDelDia(ahora)).Before(horaDePared(fecha, horaInicio).Add(-antelacion))
}

// CorrespondeLiberar dice si una reserva confirmada ya puede dejar de
// bloquear el horario porque nadie vino a buscar la máquina.
func CorrespondeLiberar(fecha time.Time, horaInicio, horaFin time.Duration, gracia time.Duration, ahora time.Time) bool {
	if YaTermino(fecha, horaInicio, horaFin, ahora) {
		return false
	}
	return !horaDePared(ahora, horaDelDia(ahora)).Before(horaDePared(fecha, horaInicio).Add(gracia))
}

// PuedeLlegarALiberarse dice si a esta reserva le puede llegar a correr el
// plazo de gracia, o sea si la clase sigue en curso cuando ese plazo se
// cumple.
func PuedeLlegarALiberarse(horaInicio, horaFin, gracia time.Duration) bool {
	// Sobre la duración real y no sobre las horas crudas: una clase nocturna de
	// 22:00 a 01:00 dura tres horas, pero `horaFin > horaInicio + gracia` daría
	// 01:00 > 22:40 = falso, y el sistema concluiría que es más corta que la
	// gracia y que nunca se libera.
	return DuracionDe(horaInicio, horaFin) > gracia
}

// CorrespondeAvisarNoRetiro dice si ya es hora de avisarle al docente que
// todavía no vino a buscar las máquinas y que pasada la gracia quedan libres
// (RF-08.20).
func CorrespondeAvisarNoRetiro(fecha time.Time, horaInicio, horaFin, demora, gracia time.Duration, ahora time.Time) bool {
	if !PuedeLlegarALiberarse(horaInicio, horaFin, gracia) {
		return false
	}
	if YaTermino(fecha, horaInicio, horaFin, ahora) {
		return false
	}
	return !horaDePared(ahora, horaDelDia(ahora)).Before(horaDePared(fecha, horaInicio).Add(demora))
}

// CorrespondeLiberarTrasEntregaParcial es el plazo corto: el docente vino, se
// llevó una parte, y lo que dejó deja de estar guardado a su nombre pasados
// unos minutos desde esa entrega (RF-08.10).
func CorrespondeLiberarTrasEntregaParcial(fecha time.Time, horaInicio, horaFin time.Duration, entregadoEn time.Time, gracia time.Duration, ahora time.Time) bool {
	if YaTermino(fecha, horaInicio, horaFin, ahora) {
		return false
	}
	return !ahora.Before(entregadoEn.Add(gracia))
}

// CorrespondeAvisarEquipoNoDisponible resuelve la regla del docente
// siguiente: el aviso sale en max(momento de la detección, inicio de su
// reserva − una hora).
func CorrespondeAvisarEquipoNoDisponible(fecha time.Time, horaInicio, horaFin time.Duration, antelacion time.Duration, ahora time.Time) bool {
	// Misma ventana que el recordatorio: desde una hora antes y mientras la
	// clase no haya terminado.
	return CorrespondeRecordar(fecha, horaInicio, horaFin, antelacion, ahora)
}
