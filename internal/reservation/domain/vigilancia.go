package domain

import "time"

// Las reglas de tiempo del barrido (RF-08.10, RF-08.11, RF-08.13): cuándo hay
// que recordar una reserva y cuándo liberarla porque nadie la retiró.

// Valores por defecto. Los dos configurables lo son por entorno; acá están
// para documentar de dónde salen y para que los tests no dependan del .env.
const (
	// GraciaDeRetiroPorDefecto: cuarenta minutos. Es lo que la escuela venía
	// tolerando en la práctica antes de que existiera el sistema.
	GraciaDeRetiroPorDefecto = 40 * time.Minute
	// AntelacionDelRecordatorio: una hora antes. Alcanza para que el docente
	// llegue, avise, o modifique la reserva antes de que empiece.
	AntelacionDelRecordatorio = time.Hour
	// GraciaTrasEntregaParcialPorDefecto: quince minutos desde que el Admin
	// anotó la entrega.
	GraciaTrasEntregaParcialPorDefecto = 15 * time.Minute
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

// CorrespondeLiberarTrasEntregaParcial es el plazo corto: el docente vino, se
// llevó una parte, y lo que dejó deja de estar guardado a su nombre pasados
// unos minutos desde esa entrega (RF-08.10).
func CorrespondeLiberarTrasEntregaParcial(fecha time.Time, horaInicio, horaFin time.Duration, entregadoEn time.Time, gracia time.Duration, ahora time.Time) bool {
	if YaTermino(fecha, horaInicio, horaFin, ahora) {
		return false
	}
	return !ahora.Before(entregadoEn.Add(gracia))
}
