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
// listaDeEquipos enumera equipos para leer dentro de una frase: "PC 1, PC 2 y
// PC 3 del Carro EDUTEC".
//
// Las etiquetas vienen con el carro adentro ("PC 1 del Carro EDUTEC") porque
// el número del zócalo se repite entre carros y un aviso que dice solo "PC 1"
// no le permite a nadie saber a dónde ir. Pero repetirlo en cada elemento hace
// ilegible el caso normal, que es una clase entera del mismo carro: "PC 1 del
// Carro 1, PC 5 del Carro 1, PC 11 del Carro 1, …" ocho veces.
//
// Por eso, si TODOS comparten el mismo carro, se dice una sola vez al final.
// Si están mezclados, cada uno se queda con el suyo — ahí la repetición es
// justamente la información.
func listaDeEquipos(nombres []string) string {
	switch len(nombres) {
	case 0:
		return ""
	case 1:
		return nombres[0]
	}

	cuerpo, comun := factorizarCarro(nombres)
	lista := strings.Join(cuerpo[:len(cuerpo)-1], ", ") + " y " + cuerpo[len(cuerpo)-1]
	return lista + comun
}

// factorizarCarro saca el carro repetido al final. Devuelve las etiquetas ya
// recortadas y el sufijo común (con su espacio adelante), o los nombres tal
// cual y "" si no hay uno que valga la pena factorizar.
//
// Se calcula por sufijo común más largo y no partiendo por " del ": un carro
// puede llamarse "Carro del Fondo", y ahí partir por la primera aparición
// dejaría "PC 1 del Carro" + "del Fondo".
func factorizarCarro(nombres []string) ([]string, string) {
	comun := sufijoComun(nombres)
	// Tiene que ser un carro entero —empieza en " del "— y tiene que quedar
	// algo antes en cada etiqueta: si un equipo se llamara igual que el
	// sufijo, recortarlo lo dejaría vacío.
	if !strings.HasPrefix(comun, " del ") {
		return nombres, ""
	}
	recortados := make([]string, len(nombres))
	for i, n := range nombres {
		recortados[i] = strings.TrimSuffix(n, comun)
		if recortados[i] == "" {
			return nombres, ""
		}
	}
	return recortados, comun
}

// Recorta de a un byte y no de a una runa: alcanza, porque el sufijo solo se
// acepta si empieza en " del " —ASCII— y eso garantiza que el corte cayó en un
// borde de runa. Un carro con acento en el nombre no lo rompe.
func sufijoComun(nombres []string) string {
	comun := nombres[0]
	for _, n := range nombres[1:] {
		for !strings.HasSuffix(n, comun) {
			comun = comun[1:]
			if comun == "" {
				return ""
			}
		}
	}
	return comun
}

// ══════════════════════════════════════════════════════════════════
// Recordatorio de reserva
// ══════════════════════════════════════════════════════════════════

// El recordatorio ya no tiene mensaje de campana: salió de ahí en la 1.18.0
// y quedó solo como correo (ver subscribers.go). Lo que sigue es ese correo.

func (m *Mensajero) textoDeRecordatorio(a eventbus.RecordatorioDeReserva) (asunto, cuerpo string) {
	asunto = fmt.Sprintf("Tenés reserva a las %s", horaDelDia(a.HoraInicio))

	cuerpo = saludo(a.Nombre) +
		fmt.Sprintf("Hoy a las %s tenés reservada%s %s para %s.",
			horaDelDia(a.HoraInicio), plural(len(a.Equipos), "", "s"),
			listaDeEquipos(a.Equipos), a.MateriaNombre)

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
