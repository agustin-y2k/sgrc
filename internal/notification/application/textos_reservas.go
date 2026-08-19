package application

import (
	"fmt"
	"strings"
	"time"

	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// Los textos del barrido de reservas y entregas (RF-08.10 a RF-08.13).

// horaDelDia formatea una hora de pared ("08:00"). Los horarios de las
// reservas son TIME sin zona: la hora de la escuela, no un instante.
func horaDelDia(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// listaDeEquipos enumera "PC 1, PC 2 y Proyector Epson".
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
	// igual va a recibir un mensaje por esta clase, mandarle dos es exactamente
	// el bombardeo que se quiso evitar.
	if len(a.EquiposSinDevolver) > 0 {
		cuerpo += fmt.Sprintf("\n\nUna cosa: %s todavía no volvió al laboratorio. "+
			"Puede que vuelva antes de tu clase, pero si llegás y no está, avisale a un "+
			"administrador o cambiá esa máquina por otra desde el sistema.",
			listaDeEquipos(a.EquiposSinDevolver))
	}

	// La regla de los cuarenta minutos se explica en cada recordatorio, no una
	// sola vez al principio: es lo que hace que liberar una reserva no se sienta
	// como que el sistema se la quitó de prepo.
	cuerpo += fmt.Sprintf("\n\nSi vas a llegar tarde o no vas a poder ir, modificá o anulá la "+
		"reserva: pasados %d minutos del horario de inicio, las máquinas que no hayas "+
		"retirado quedan libres para que las use otro docente.", a.MinutosDeGracia)
	cuerpo += m.enlace("Podés hacerlo desde:")
	cuerpo += firma
	return asunto, cuerpo
}

// ══════════════════════════════════════════════════════════════════ Una PC
// de tu reserva no volvió
// ══════════════════════════════════════════════════════════════════

func mensajeDeEquipoNoDisponible(a eventbus.EquipoNoDisponibleParaReserva) string {
	return fmt.Sprintf("%s de tu reserva de las %s no volvió al laboratorio todavía",
		listaDeEquipos(a.Equipos), horaDelDia(a.HoraInicio))
}

func (m *Mensajero) textoDeEquipoNoDisponible(a eventbus.EquipoNoDisponibleParaReserva) (asunto, cuerpo string) {
	asunto = "Una computadora de tu reserva puede no estar"

	cuerpo = saludo(a.Nombre) +
		fmt.Sprintf("%s de tu reserva de hoy a las %s (%s) todavía no volvió al "+
			"laboratorio: la tiene otra persona y ya se pasó de la hora de devolución.",
			listaDeEquipos(a.Equipos), horaDelDia(a.HoraInicio), a.MateriaNombre)

	// No se promete nada que el sistema no sepa: puede volver en cinco minutos,
	// y decir "no va a estar" sería hacerle cambiar la reserva al pedo.
	cuerpo += "\n\nPuede que vuelva antes de tu clase. Si preferís no arriesgarte, " +
		"desde el sistema podés cambiar esa máquina por otra que esté libre."
	cuerpo += m.enlace("Tus reservas están en:")
	cuerpo += firma
	return asunto, cuerpo
}

// ══════════════════════════════════════════════════════════════════ Alguien
// te pide un equipo que tenés reservado
// ══════════════════════════════════════════════════════════════════ Es el
// único aviso de esta familia que NO anuncia un cambio.

func mensajeDePedidoDeLiberacion(a eventbus.PedidoDeLiberacion) string {
	return fmt.Sprintf("%s necesita %s, que tenés reservada el %s de %s a %s. "+
		"Tu reserva sigue como está: la decisión es tuya",
		a.SolicitanteNombre, a.Etiqueta, formatearFecha(a.Fecha),
		horaDelDia(a.HoraInicio), horaDelDia(a.HoraFin))
}

func (m *Mensajero) textoDePedidoDeLiberacion(a eventbus.PedidoDeLiberacion) (asunto, cuerpo string) {
	asunto = fmt.Sprintf("%s necesita una computadora que tenés reservada", a.SolicitanteNombre)

	cuerpo = saludo(a.Nombre) +
		fmt.Sprintf("%s necesita %s para el %s de %s a %s, que es la franja en la que la "+
			"tenés reservada%s.",
			a.SolicitanteNombre, a.Etiqueta, formatearFecha(a.Fecha),
			horaDelDia(a.HoraInicio), horaDelDia(a.HoraFin), paraLaMateria(a.MateriaNombre))

	if a.Mensaje != "" {
		// Va tal cual y entre comillas: es la parte del pedido que explica para qué
		// la necesita, y reformularla sería ponerle palabras en la boca a quien
		// pidió.
		cuerpo += fmt.Sprintf("\n\nTe dejó este mensaje:\n\n  «%s»", a.Mensaje)
	}

	// Lo primero que hay que despejar: no se le sacó nada.
	cuerpo += "\n\nTu reserva no cambió y nadie la va a cambiar por vos: esto es un pedido, " +
		"no un aviso de cancelación. Si podés arreglarte sin esa máquina, entrá al sistema y " +
		"cambiala por otra libre o cancelá esa reserva, y queda disponible. Si la necesitás, " +
		"no tenés que hacer nada."
	cuerpo += m.enlace("Tus reservas están en:")
	cuerpo += firma
	return asunto, cuerpo
}

// paraLaMateria arma el " para Matemáticas" del medio de la frase, o nada si
// la reserva no tiene materia a la vista.
func paraLaMateria(nombre string) string {
	if strings.TrimSpace(nombre) == "" {
		return ""
	}
	return " para " + nombre
}

// ══════════════════════════════════════════════════════════════════ Todavía
// no retiraste las máquinas
// ══════════════════════════════════════════════════════════════════ Este
// aviso sale mientras todavía se puede hacer algo, no cuando la reserva ya se
// liberó.

func mensajeDeReservaSinRetirar(a eventbus.ReservaSinRetirar) string {
	return fmt.Sprintf("Todavía no retiraste %s de tu reserva de las %s para %s: a los %d "+
		"minutos del horario de inicio quedan libres para otro docente",
		listaDeEquipos(a.Equipos), horaDelDia(a.HoraInicio), a.MateriaNombre, a.MinutosDeGracia)
}

func (m *Mensajero) textoDeReservaSinRetirar(a eventbus.ReservaSinRetirar) (asunto, cuerpo string) {
	asunto = "Todavía no retiraste tus computadoras"

	cuerpo = saludo(a.Nombre) +
		fmt.Sprintf("Tu clase de %s empezó a las %s y todavía nadie pasó a buscar %s.",
			a.MateriaNombre, horaDelDia(a.HoraInicio), listaDeEquipos(a.Equipos))

	cuerpo += fmt.Sprintf("\n\nA los %d minutos del horario de inicio, lo que no se haya "+
		"retirado queda libre para que lo use otro docente. Si vas en camino no hace falta "+
		"que hagas nada; si no vas a poder ir, o si preferís cambiar alguna máquina por otra, "+
		"desde el sistema podés hacerlo ahora.", a.MinutosDeGracia)

	// Liberar no es prohibir, y decirlo acá evita el llamado del docente que
	// llegó tarde y cree que ya no puede usar nada.
	cuerpo += "\n\nY si llegás más tarde y las computadoras siguen en el laboratorio, " +
		"pedíselas a un administrador: te las entrega igual. Lo único que cambia es que " +
		"dejan de estar guardadas a tu nombre."
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
		// "desde las" es EntregadoEn y "tenía que volver a las" es DebioVolverA.
		// Iban las dos con DebioVolverA, así que el correo repetía la misma hora y
		// afirmaba que la máquina había salido justo cuando tenía que estar de
		// vuelta.
		fmt.Fprintf(&sb, ": la tiene %s desde las %s, tenía que volver a las %s (%s de demora)\n",
			p.Quien, p.EntregadoEn.Format("15:04"), p.DebioVolverA.Format("15:04"),
			textoDeDemora(p.MinutosDeDemora))
	}

	cuerpo = sb.String()
	cuerpo += m.enlace("Podés ver qué hay afuera desde:")
	cuerpo += firma
	return asunto, cuerpo
}

// textoDeDemoraParaQuienLaTiene es un recordatorio, no un reclamo.
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

// ══════════════════════════════════════════════════════════════════ El corte
// de fin de jornada
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

// ══════════════════════════════════════════════════════════════════
// Cancelaciones (RF-05.1/05.2/05.3)
// ══════════════════════════════════════════════════════════════════

// textoDeCancelacion cuenta qué computadoras se cancelaron y por qué.
//
// Lo que el texto no puede dejar pasar: una cancelación es POR MÁQUINA. Un
// bloqueo sobre dos PCs de una clase de seis no cancela la clase, y quien lee
// "se canceló tu reserva" a las once de la noche da por perdida una hora que
// todavía tiene. Por eso se nombran los equipos y se aclara qué sigue en pie.
func (m *Mensajero) textoDeCancelacion(a eventbus.CancelacionesDeUsuario) (asunto, cuerpo string) {
	equipos := equiposDeLasCanceladas(a.Reservas)
	fecha, unaSolaFecha := fechaUnica(a.Reservas)

	switch {
	case len(a.Reservas) == 1:
		r := a.Reservas[0]
		asunto = fmt.Sprintf("Se canceló %s de tu reserva del %s",
			etiquetaODefecto(r.Etiqueta), formatearFecha(r.Fecha))
		cuerpo = saludo(a.Nombre) + fmt.Sprintf(
			"Se canceló %s, que tenías reservada para el %s.",
			etiquetaODefecto(r.Etiqueta), formatearFecha(r.Fecha))
	case unaSolaFecha:
		asunto = fmt.Sprintf("Se cancelaron %d computadoras de tu reserva del %s",
			len(a.Reservas), formatearFecha(fecha))
		cuerpo = saludo(a.Nombre) + fmt.Sprintf(
			"Se cancelaron %d computadoras que tenías reservadas para el %s: %s.",
			len(a.Reservas), formatearFecha(fecha), equipos)
	default:
		asunto = fmt.Sprintf("Se cancelaron %d computadoras que tenías reservadas", len(a.Reservas))
		cuerpo = saludo(a.Nombre) + fmt.Sprintf(
			"Se cancelaron %d computadoras que tenías reservadas: %s.", len(a.Reservas), equipos)
	}

	// El motivo va en su propio renglón: es lo único de este correo que
	// escribió una persona, y lo primero que se busca al leerlo.
	if motivo := strings.TrimSpace(a.Motivo); motivo != "" {
		cuerpo += "\n\nMotivo: " + motivo
	}

	cuerpo += "\n\nSe cancelaron solo las computadoras que se nombran acá: si " +
		"para esa clase tenías otras, siguen reservadas. Podés reservar otra " +
		"máquina para la misma franja si queda alguna libre."
	cuerpo += m.enlace("Mirá cómo quedó tu reserva en:")
	cuerpo += firma
	return asunto, cuerpo
}
