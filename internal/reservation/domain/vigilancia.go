package domain

import "time"

// Las reglas de tiempo del barrido (RF-08.10 a RF-08.13): cuándo hay que
// recordar una reserva, cuándo liberarla porque nadie la retiró, y cuándo
// avisarle al docente siguiente que una máquina no volvió.
//
// Viven en el dominio y no en el job porque son decisiones de negocio —"a
// los cuarenta minutos ya no es tuya"— y porque así se pueden probar sin
// base, sin reloj y sin correo. El job solo pregunta.

// Valores por defecto. Los tres son configurables por entorno; acá están
// para documentar de dónde salen y para que los tests no dependan del .env.
const (
	// GraciaDeRetiroPorDefecto: cuarenta minutos. Es lo que la escuela venía
	// tolerando en la práctica antes de que existiera el sistema.
	GraciaDeRetiroPorDefecto = 40 * time.Minute
	// AntelacionDelRecordatorio: una hora antes. Alcanza para que el docente
	// llegue, avise, o modifique la reserva antes de que empiece.
	AntelacionDelRecordatorio = time.Hour
	// DemoraDelAvisoDeNoRetiroPorDefecto: quince minutos después de que
	// empezó la clase. Es el momento en que el aviso todavía sirve para
	// algo — quedan veinticinco minutos para ir, cambiar la máquina o
	// cancelar. Mandarlo junto con la liberación era contarle al docente un
	// hecho consumado.
	DemoraDelAvisoDeNoRetiroPorDefecto = 15 * time.Minute
	// GraciaTrasEntregaParcialPorDefecto: quince minutos desde que el Admin
	// anotó la entrega. Es un plazo distinto y más corto que el de arriba
	// porque responde a otra pregunta: ahí el sistema no sabe si el docente
	// va a venir, acá ya vino y eligió qué se llevaba.
	GraciaTrasEntregaParcialPorDefecto = 15 * time.Minute
	// DemoraParaReclamarPorDefecto: diez minutos después de la hora en que
	// la máquina tenía que volver. Es un margen para el que está guardando
	// las cosas, no una tolerancia de verdad.
	DemoraParaReclamarPorDefecto = 10 * time.Minute
)

// CorrespondeRecordar dice si ya es hora de mandarle al docente el aviso de
// "en un rato tenés reserva".
//
// El límite de arriba es que la clase no haya TERMINADO, no que no haya
// empezado. Si el proceso estuvo caído, el recordatorio sale tarde en vez de
// perderse: a las 8:10 todavía sirve saber que hay una reserva a las 8 y
// que a las 8:40 se libera. Después de que terminó ya no le sirve a nadie.
func CorrespondeRecordar(fecha time.Time, horaInicio, horaFin time.Duration, antelacion time.Duration, ahora time.Time) bool {
	if YaTermino(fecha, horaFin, ahora) {
		return false
	}
	return !horaDePared(ahora, horaDelDia(ahora)).Before(horaDePared(fecha, horaInicio).Add(-antelacion))
}

// CorrespondeLiberar dice si una reserva confirmada ya puede dejar de
// bloquear el horario porque nadie vino a buscar la máquina.
//
// La reserva que YA TERMINÓ no se libera, y eso resuelve solo el caso de una
// gracia más larga que la clase: con cuarenta minutos de gracia y una hora
// de clase, se libera a los cuarenta; con una clase de media hora, no se
// libera nunca — liberar los últimos minutos no le sirve a nadie, y el job
// de vencimiento (RF-04.9) la va a marcar FINALIZADA igual.
//
// Que la máquina esté retirada o no NO se decide acá: el dominio no ve los
// préstamos. Lo pregunta quien llama.
func CorrespondeLiberar(fecha time.Time, horaInicio, horaFin time.Duration, gracia time.Duration, ahora time.Time) bool {
	if YaTermino(fecha, horaFin, ahora) {
		return false
	}
	return !horaDePared(ahora, horaDelDia(ahora)).Before(horaDePared(fecha, horaInicio).Add(gracia))
}

// PuedeLlegarALiberarse dice si a esta reserva le puede llegar a correr el
// plazo de gracia, o sea si la clase sigue en curso cuando ese plazo se
// cumple.
//
// Existe porque el aviso de RF-08.20 sale ANTES que la liberación y tiene que
// saber de antemano lo que CorrespondeLiberar descubre sobre la marcha: una
// clase más corta que la gracia no se libera nunca. Sin esta pregunta, a esa
// reserva le llegaría un correo anunciándole algo que no va a pasar.
func PuedeLlegarALiberarse(horaInicio, horaFin, gracia time.Duration) bool {
	return horaFin > horaInicio+gracia
}

// CorrespondeAvisarNoRetiro dice si ya es hora de avisarle al docente que
// todavía no vino a buscar las máquinas y que pasada la gracia quedan libres
// (RF-08.20).
//
// La otra mitad de la condición —que efectivamente no se haya retirado
// ninguna— la pone quien llama, que es quien ve los préstamos.
//
// Igual que el recordatorio, el límite de arriba es que la clase no haya
// terminado y no que no haya empezado: si el proceso estuvo caído, el aviso
// sale tarde en vez de perderse.
func CorrespondeAvisarNoRetiro(fecha time.Time, horaInicio, horaFin, demora, gracia time.Duration, ahora time.Time) bool {
	if !PuedeLlegarALiberarse(horaInicio, horaFin, gracia) {
		return false
	}
	if YaTermino(fecha, horaFin, ahora) {
		return false
	}
	return !horaDePared(ahora, horaDelDia(ahora)).Before(horaDePared(fecha, horaInicio).Add(demora))
}

// CorrespondeLiberarTrasEntregaParcial es el plazo corto: el docente vino, se
// llevó una parte, y lo que dejó deja de estar guardado a su nombre pasados
// unos minutos desde esa entrega (RF-08.10).
//
// Cuenta desde la entrega y no desde el inicio de la clase, y eso **reemplaza**
// al plazo largo en vez de competir con él. En la práctica cae antes; si el
// Admin anota la entrega sobre el final de la gracia, cae después, y está
// bien que así sea: el docente estuvo en el mostrador recién, y darle sus
// quince minutos completos para volver por el resto es la razón de ser del
// plazo.
//
// El tope sigue siendo el mismo que el del plazo largo — una clase terminada
// no se libera, la marca FINALIZADA el barrido de vencimiento.
func CorrespondeLiberarTrasEntregaParcial(fecha time.Time, horaFin time.Duration, entregadoEn time.Time, gracia time.Duration, ahora time.Time) bool {
	if YaTermino(fecha, horaFin, ahora) {
		return false
	}
	return !ahora.Before(entregadoEn.Add(gracia))
}

// CorrespondeAvisarEquipoNoDisponible resuelve la regla del docente siguiente:
// el aviso sale en max(momento de la detección, inicio de su reserva − una
// hora).
//
// No son dos reglas con una excepción, es una sola cuenta. Y lo importante
// es lo que NO hace: si la máquina vuelve antes de que llegue ese momento,
// el aviso no sale nunca. En el caso más común —alguien se demora quince
// minutos y devuelve— el docente de tres horas después no se entera de nada,
// que es exactamente lo que se pidió al diseñarlo.
//
// Cuando la reserva es contigua o falta menos de una hora, esto devuelve
// true apenas se detecta la demora. Hay que ser honesto sobre eso: el mail
// llega tarde igual, el docente ya está yendo al laboratorio. Lo que
// resuelve ese caso de verdad es el reclamo al Admin, que sale a los diez
// minutos y lo arregla en persona.
func CorrespondeAvisarEquipoNoDisponible(fecha time.Time, horaInicio, horaFin time.Duration, antelacion time.Duration, ahora time.Time) bool {
	// Misma ventana que el recordatorio: desde una hora antes y mientras la
	// clase no haya terminado. La otra mitad de la condición —que la máquina
	// esté demorada AHORA— la pone quien llama, y es la que hace que el
	// aviso desaparezca solo si la PC vuelve a tiempo.
	return CorrespondeRecordar(fecha, horaInicio, horaFin, antelacion, ahora)
}
