package application

import (
	"fmt"
	"strings"
	"time"

	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// Los textos del barrido de reservas y entregas (RF-08.10 a RF-08.13).
//
// Son los únicos avisos del sistema que nacen de un reloj, y eso cambia cómo
// hay que escribirlos: quien los recibe no acaba de hacer nada, así que el
// mensaje tiene que explicar solo por qué le está llegando. Un "tu reserva
// fue liberada" sin decir que pasaron cuarenta minutos se lee como un error
// del sistema.

// horaDelDia formatea una hora de pared ("08:00"). Los horarios de las
// reservas son TIME sin zona: la hora de la escuela, no un instante.
func horaDelDia(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// listaDeEquipos enumera "PC 1, PC 2 y Proyector Epson". La conjunción final
// no es un capricho: sin ella se lee como una tabla, y esto es una frase.
//
// Recibe etiquetas ya resueltas y no números: desde la 015 lo que se reserva
// puede no tener número, y "PC 0" es lo que sale de formatear uno que no
// existe.
func listaDeEquipos(nombres []string) string {
	switch len(nombres) {
	case 0:
		return ""
	case 1:
		return nombres[0]
	default:
		return strings.Join(nombres[:len(nombres)-1], ", ") + " y " + nombres[len(nombres)-1]
	}
}

// ══════════════════════════════════════════════════════════════════
// Recordatorio de reserva
// ══════════════════════════════════════════════════════════════════

func mensajeDeRecordatorio(a eventbus.RecordatorioDeReserva) string {
	base := fmt.Sprintf("Hoy a las %s tenés reservada%s %s para %s",
		horaDelDia(a.HoraInicio), plural(len(a.Equipos), "", "s"),
		listaDeEquipos(a.Equipos), a.MateriaNombre)

	if len(a.EquiposSinDevolver) > 0 {
		base += fmt.Sprintf(". Ojo: %s no volvió al laboratorio todavía",
			listaDeEquipos(a.EquiposSinDevolver))
	}
	return base
}

func (m *Mensajero) textoDeRecordatorio(a eventbus.RecordatorioDeReserva) (asunto, cuerpo string) {
	asunto = fmt.Sprintf("Tenés reserva a las %s", horaDelDia(a.HoraInicio))

	cuerpo = saludo(a.Nombre) +
		fmt.Sprintf("Hoy a las %s tenés reservada%s %s para %s.",
			horaDelDia(a.HoraInicio), plural(len(a.Equipos), "", "s"),
			listaDeEquipos(a.Equipos), a.MateriaNombre)

	// La advertencia va acá adentro y no en un correo aparte: si el docente
	// igual va a recibir un mensaje por esta clase, mandarle dos es
	// exactamente el bombardeo que se quiso evitar.
	if len(a.EquiposSinDevolver) > 0 {
		cuerpo += fmt.Sprintf("\n\nUna cosa: %s todavía no volvió al laboratorio. "+
			"Puede que vuelva antes de tu clase, pero si llegás y no está, avisale a un "+
			"administrador o cambiá esa máquina por otra desde el sistema.",
			listaDeEquipos(a.EquiposSinDevolver))
	}

	// La regla de los cuarenta minutos se explica en cada recordatorio, no
	// una sola vez al principio: es lo que hace que liberar una reserva no
	// se sienta como que el sistema se la quitó de prepo.
	cuerpo += fmt.Sprintf("\n\nSi vas a llegar tarde o no vas a poder ir, modificá o anulá la "+
		"reserva: pasados %d minutos del horario de inicio, las máquinas que no hayas "+
		"retirado quedan libres para que las use otro docente.", a.MinutosDeGracia)
	cuerpo += m.enlace("Podés hacerlo desde:")
	cuerpo += firma
	return asunto, cuerpo
}

// ══════════════════════════════════════════════════════════════════
// Una PC de tu reserva no volvió
// ══════════════════════════════════════════════════════════════════

func mensajeDeEquipoNoDisponible(a eventbus.EquipoNoDisponibleParaReserva) string {
	return fmt.Sprintf("%s de tu reserva de las %s no volvió al laboratorio todavía",
		listaDeEquipos(a.Equipos), horaDelDia(a.HoraInicio))
}

func (m *Mensajero) textoDePCNoDisponible(a eventbus.EquipoNoDisponibleParaReserva) (asunto, cuerpo string) {
	asunto = "Una computadora de tu reserva puede no estar"

	cuerpo = saludo(a.Nombre) +
		fmt.Sprintf("%s de tu reserva de hoy a las %s (%s) todavía no volvió al "+
			"laboratorio: la tiene otra persona y ya se pasó de la hora de devolución.",
			listaDeEquipos(a.Equipos), horaDelDia(a.HoraInicio), a.MateriaNombre)

	// No se promete nada que el sistema no sepa: puede volver en cinco
	// minutos, y decir "no va a estar" sería hacerle cambiar la reserva al
	// pedo.
	cuerpo += "\n\nPuede que vuelva antes de tu clase. Si preferís no arriesgarte, " +
		"desde el sistema podés cambiar esa máquina por otra que esté libre."
	cuerpo += m.enlace("Tus reservas están en:")
	cuerpo += firma
	return asunto, cuerpo
}

// ══════════════════════════════════════════════════════════════════
// Tu reserva se liberó
// ══════════════════════════════════════════════════════════════════

func mensajeDeReservasLiberadas(a eventbus.ReservasLiberadas) string {
	if a.TodaLaReserva {
		return fmt.Sprintf("Tu reserva de las %s para %s quedó libre: pasaron %d minutos y no "+
			"se retiró ninguna computadora",
			horaDelDia(a.HoraInicio), a.MateriaNombre, a.MinutosDeGracia)
	}
	return fmt.Sprintf("%s de tu reserva de las %s quedaron libres: pasaron %d minutos y no se retiraron",
		listaDeEquipos(a.Equipos), horaDelDia(a.HoraInicio), a.MinutosDeGracia)
}

func (m *Mensajero) textoDeReservasLiberadas(a eventbus.ReservasLiberadas) (asunto, cuerpo string) {
	asunto = "Tu reserva quedó libre"

	if a.TodaLaReserva {
		cuerpo = saludo(a.Nombre) +
			fmt.Sprintf("Tu reserva de hoy a las %s para %s (%s) quedó libre: pasaron %d "+
				"minutos del horario de inicio y no se retiró ninguna computadora, así que "+
				"volvieron a estar disponibles para el resto.",
				horaDelDia(a.HoraInicio), a.MateriaNombre, listaDeEquipos(a.Equipos),
				a.MinutosDeGracia)
	} else {
		cuerpo = saludo(a.Nombre) +
			fmt.Sprintf("%s de tu reserva de hoy a las %s para %s quedaron libres: pasaron %d "+
				"minutos y no se retiraron, así que volvieron a estar disponibles para el resto.",
				listaDeEquipos(a.Equipos), horaDelDia(a.HoraInicio), a.MateriaNombre,
				a.MinutosDeGracia)
	}

	// Liberar no es prohibir, y decirlo importa: sin esta línea el docente
	// que llegó tarde asume que ya no puede usarlas y se va.
	cuerpo += "\n\nEsto no quiere decir que no las puedas usar: si todavía están en el " +
		"laboratorio, pedíselas a un administrador y te las entrega igual. Lo único que " +
		"pasó es que dejaron de estar guardadas a tu nombre."
	cuerpo += m.enlace("Tus reservas están en:")
	cuerpo += firma
	return asunto, cuerpo
}

// ══════════════════════════════════════════════════════════════════
// Devoluciones demoradas (a los Admin y a quien la tiene)
// ══════════════════════════════════════════════════════════════════

func mensajeDePrestamosDemorados(a eventbus.PrestamosDemorados) string {
	if len(a.Prestamos) == 1 {
		p := a.Prestamos[0]
		return fmt.Sprintf("%s no volvió al laboratorio: la tiene %s y tenía que devolverla a las %s",
			p.Etiqueta, p.Quien, p.DebioVolverA.Format("15:04"))
	}
	return fmt.Sprintf("%d computadoras no volvieron al laboratorio a horario", len(a.Prestamos))
}

func (m *Mensajero) textoDeDemoraParaAdmins(a eventbus.PrestamosDemorados) (asunto, cuerpo string) {
	if len(a.Prestamos) == 1 {
		asunto = "Una computadora no volvió a horario"
	} else {
		asunto = fmt.Sprintf("%d computadoras no volvieron a horario", len(a.Prestamos))
	}

	var sb strings.Builder
	sb.WriteString("Estas computadoras tenían que estar de vuelta y no volvieron:\n\n")
	for _, p := range a.Prestamos {
		fmt.Fprintf(&sb, "  - %s", p.Etiqueta)
		if p.CarroNombre != "" {
			fmt.Fprintf(&sb, " (%s)", p.CarroNombre)
		}
		fmt.Fprintf(&sb, ": la tiene %s desde las %s, tenía que volver a las %s (%s de demora)\n",
			p.Quien, p.DebioVolverA.Format("15:04"), p.DebioVolverA.Format("15:04"),
			textoDeDemora(p.MinutosDeDemora))
	}

	cuerpo = sb.String()
	cuerpo += m.enlace("Podés ver qué hay afuera desde:")
	cuerpo += firma
	return asunto, cuerpo
}

// textoDeDemoraParaQuienLaTiene es un recordatorio, no un reclamo. Quien la
// tiene puede estar dando clase, y el tono importa: esto lo lee un colega,
// no un deudor.
func (m *Mensajero) textoDeDemoraParaQuienLaTiene(p eventbus.PrestamoDemorado) (asunto, cuerpo string) {
	asunto = fmt.Sprintf("Acordate de devolver la %s", p.Etiqueta)

	cuerpo = saludo(p.Quien) +
		fmt.Sprintf("La %s tenía que volver al laboratorio a las %s. Si ya la devolviste, "+
			"puede que todavía no la hayan registrado y podés ignorar este mensaje.",
			p.Etiqueta, p.DebioVolverA.Format("15:04"))
	cuerpo += "\n\nSi la seguís necesitando, avisale a un administrador: puede haber alguien " +
		"esperándola para la próxima clase."
	cuerpo += firma
	return asunto, cuerpo
}

// textoDeDemora: "25 minutos", "2 h 10 min".
func textoDeDemora(minutos int) string {
	if minutos < 60 {
		return fmt.Sprintf("%d minutos", minutos)
	}
	horas, resto := minutos/60, minutos%60
	if resto == 0 {
		return fmt.Sprintf("%d h", horas)
	}
	return fmt.Sprintf("%d h %d min", horas, resto)
}

// ══════════════════════════════════════════════════════════════════
// El corte de fin de jornada
// ══════════════════════════════════════════════════════════════════

func mensajeDeCierre(a eventbus.EquiposSinDevolverAlCierre) string {
	if len(a.Equipos) == 1 {
		p := a.Equipos[0]
		return fmt.Sprintf("%s quedó fuera del laboratorio al cierre: la tiene %s",
			p.Etiqueta, p.Quien)
	}
	return fmt.Sprintf("%d computadoras quedaron fuera del laboratorio al cierre", len(a.Equipos))
}

func (m *Mensajero) textoDeCierreParaAdmins(a eventbus.EquiposSinDevolverAlCierre) (asunto, cuerpo string) {
	asunto = "Computadoras que quedaron afuera"

	var sb strings.Builder
	sb.WriteString("Al cerrar la jornada, estas computadoras siguen fuera del laboratorio:\n\n")
	for _, p := range a.Equipos {
		fmt.Fprintf(&sb, "  - %s", p.Etiqueta)
		if p.CarroNombre != "" {
			fmt.Fprintf(&sb, " (%s)", p.CarroNombre)
		}
		fmt.Fprintf(&sb, ": la tiene %s desde las %s", p.Quien, p.DesdeCuando.Format("15:04"))
		// A quién le va a faltar mañana es el dato accionable: sin él, la
		// lista es una constatación y con él es una tarea.
		if p.ProximoNombre != "" {
			fmt.Fprintf(&sb, "; la tiene reservada %s el %s a las %s",
				p.ProximoNombre, formatearFecha(p.ProximaFecha), horaDelDia(p.ProximaHora))
		}
		sb.WriteString("\n")
	}

	cuerpo = sb.String()
	cuerpo += m.enlace("Podés ver el detalle desde:")
	cuerpo += firma
	return asunto, cuerpo
}

func (m *Mensajero) textoDeCierreParaElProximo(p eventbus.EquipoSinDevolverAlCierre) (asunto, cuerpo string) {
	asunto = "Una computadora de tu reserva puede no estar"

	cuerpo = saludo(p.ProximoNombre) +
		fmt.Sprintf("La %s, que tenés reservada para el %s a las %s, quedó fuera del "+
			"laboratorio al cerrar hoy: la tiene %s.",
			p.Etiqueta, formatearFecha(p.ProximaFecha),
			horaDelDia(p.ProximaHora), p.Quien)
	cuerpo += "\n\nPuede que la devuelvan antes de tu clase. Si preferís no arriesgarte, " +
		"desde el sistema podés cambiarla por otra que esté libre."
	cuerpo += m.enlace("Tus reservas están en:")
	cuerpo += firma
	return asunto, cuerpo
}
