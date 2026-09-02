package application

import (
	"fmt"
	"strings"

	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// Los textos de la bandeja de soporte y de los pedidos para dictar una
// materia.

// ══════════════════════════════════════════════════════════════════ El buzón
// ══════════════════════════════════════════════════════════════════

// maxEnElAviso: cuánto del mensaje entra en el aviso de la campana.
const maxEnElAviso = 200

func recortar(texto string) string {
	r := []rune(strings.TrimSpace(texto))
	if len(r) <= maxEnElAviso {
		return string(r)
	}
	return strings.TrimSpace(string(r[:maxEnElAviso])) + "…"
}

func quienODefecto(nombre string) string {
	if strings.TrimSpace(nombre) == "" {
		return "Alguien"
	}
	return nombre
}

// queHizo describe la acción en los términos de la persona y no del sistema:
// "avisó que algo no anda" y no "reportó un problema".
func queHizo(tipo string) string {
	switch tipo {
	case "AYUDA":
		return "pidió ayuda"
	case "PROBLEMA":
		return "avisó que algo no anda"
	default:
		return "dejó una sugerencia"
	}
}

func mensajeDeSugerencia(a eventbus.SugerenciaNueva) string {
	base := fmt.Sprintf("%s %s: «%s»", quienODefecto(a.Quien), queHizo(a.Tipo), recortar(a.Asunto))
	if a.Pantalla != "" {
		base += fmt.Sprintf(" (desde %s)", a.Pantalla)
	}
	return base
}

// mensajeDeSeguimiento: quien preguntó volvió a escribir en su hilo. Se
// distingue del primero porque para quien lee no es lo mismo atender algo
// nuevo que retomar una conversación que ya había contestado.
func mensajeDeSeguimiento(a eventbus.SugerenciaSeguimiento) string {
	return fmt.Sprintf("%s escribió de nuevo sobre «%s»: %s",
		quienODefecto(a.Quien), recortar(a.Asunto), recortar(a.Texto))
}

func mensajeDeRespuestaASugerencia(a eventbus.SugerenciaRespondida) string {
	return fmt.Sprintf("Te contestaron sobre «%s»: %s",
		recortar(a.Asunto), recortar(a.Respuesta))
}

// asuntoDelHilo pone adelante de qué se trata y después lo que escribió la
// persona. El prefijo importa en una bandeja llena: "Pedido de ayuda" ordena
// solo, y el asunto suelto se pierde entre los mails de la escuela.
func asuntoDelHilo(tipo, asunto string) string {
	var prefijo string
	switch tipo {
	case "AYUDA":
		prefijo = "Pedido de ayuda"
	case "PROBLEMA":
		prefijo = "Algo no anda"
	default:
		prefijo = "Sugerencia"
	}
	if asunto = strings.TrimSpace(asunto); asunto == "" {
		return prefijo
	}
	return prefijo + ": " + asunto
}

func (m *Mensajero) textoDeSugerencia(a eventbus.SugerenciaNueva) (asunto, cuerpo string) {
	asunto = asuntoDelHilo(a.Tipo, a.Asunto)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s desde el sistema:\n\n", quienODefecto(a.Quien), queHizo(a.Tipo))
	// El texto va COMPLETO en el correo, a diferencia del aviso: acá no hay un
	// renglón que respetar, y quien lee necesita el detalle para poder hacer
	// algo con esto.
	fmt.Fprintf(&sb, "  %s\n", strings.ReplaceAll(strings.TrimSpace(a.Texto), "\n", "\n  "))
	if a.Pantalla != "" {
		fmt.Fprintf(&sb, "\nLo escribió desde: %s\n", a.Pantalla)
	}

	cuerpo = sb.String()
	cuerpo += m.enlace("Podés contestarle desde:")
	cuerpo += firma
	return asunto, cuerpo
}

// textoDeSeguimiento es el correo a los Admin cuando quien preguntó vuelve a
// escribir. No repite el mensaje inicial: lo que hace falta para retomar es
// el asunto y lo último que dijo.
func (m *Mensajero) textoDeSeguimiento(a eventbus.SugerenciaSeguimiento) (asunto, cuerpo string) {
	asunto = asuntoDelHilo(a.Tipo, a.Asunto)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s escribió de nuevo en una conversación abierta:\n\n", quienODefecto(a.Quien))
	fmt.Fprintf(&sb, "  %s\n", strings.ReplaceAll(strings.TrimSpace(a.Texto), "\n", "\n  "))

	cuerpo = sb.String()
	cuerpo += m.enlace("Podés seguirla desde:")
	cuerpo += firma
	return asunto, cuerpo
}

func (m *Mensajero) textoDeRespuestaASugerencia(a eventbus.SugerenciaRespondida) (asunto, cuerpo string) {
	asunto = "Te contestaron: " + strings.TrimSpace(a.Asunto)

	cuerpo = saludo(a.Nombre) +
		fmt.Sprintf("Sobre «%s», te contestaron:\n\n  %s\n",
			strings.TrimSpace(a.Asunto),
			strings.ReplaceAll(strings.TrimSpace(a.Respuesta), "\n", "\n  "))
	// Que se puede seguir la conversación adentro es la mitad del punto de
	// tener esto en el sistema: sin decirlo, la gente contesta el mail.
	cuerpo += "\n\nSi te quedó algo por preguntar, podés seguir la conversación " +
		"desde el sistema; no hace falta que respondas este correo."
	cuerpo += m.enlace("Entrá a Notificaciones desde:")
	cuerpo += firma
	return asunto, cuerpo
}

// ══════════════════════════════════════════════════════════════════ Pedidos
// para dictar una materia
// ══════════════════════════════════════════════════════════════════

// nombreDeLaMateria arma "Programación de 1°A" cuando se sabe el curso, y
// solo el nombre cuando no.
func nombreDeLaMateria(materia, curso string) string {
	if strings.TrimSpace(curso) == "" {
		return materia
	}
	return fmt.Sprintf("%s de %s", materia, curso)
}

func mensajeDePedidoDeMateria(a eventbus.PedidoDeMateriaNuevo) string {
	base := fmt.Sprintf("%s pide dictar %s", quienODefecto(a.Nombre),
		nombreDeLaMateria(a.MateriaNombre, a.CursoNombre))
	if a.EsMateriaNueva {
		// Que la materia no exista cambia lo que hay que hacer: no es aprobar un
		// permiso, es crear algo.
		base += " (esa materia todavía no existe en el sistema)"
	}
	return base + fmt.Sprintf(". Motivo: «%s»", recortar(a.Motivo))
}

func mensajeDePedidoResuelto(a eventbus.PedidoDeMateriaResuelto) string {
	if a.Aprobado {
		return fmt.Sprintf("Ya podés reservar computadoras para %s. %s",
			a.MateriaNombre, strings.TrimSpace(a.Respuesta))
	}
	return fmt.Sprintf("No se aprobó tu pedido para dictar %s: %s",
		a.MateriaNombre, strings.TrimSpace(a.Respuesta))
}

func (m *Mensajero) textoDePedidoDeMateria(a eventbus.PedidoDeMateriaNuevo) (asunto, cuerpo string) {
	asunto = fmt.Sprintf("%s pide dictar %s", quienODefecto(a.Nombre), a.MateriaNombre)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s pidió poder reservar computadoras para %s.\n\n",
		quienODefecto(a.Nombre), nombreDeLaMateria(a.MateriaNombre, a.CursoNombre))
	fmt.Fprintf(&sb, "Lo explica así:\n\n  %s\n",
		strings.ReplaceAll(strings.TrimSpace(a.Motivo), "\n", "\n  "))

	if a.EsMateriaNueva {
		sb.WriteString("\nEsa materia todavía no existe en el sistema: si aprobás el pedido, " +
			"se crea con ese nombre.\n")
	}
	if len(a.DocentesActuales) > 0 {
		// Con quién hablar antes de decidir.
		nombres := make([]string, 0, len(a.DocentesActuales))
		for _, d := range a.DocentesActuales {
			nombres = append(nombres, d.Nombre)
		}
		fmt.Fprintf(&sb, "\nHoy esa materia la dan: %s. También recibieron este aviso.\n",
			strings.Join(nombres, ", "))
	}

	cuerpo = sb.String()
	cuerpo += m.enlace("Se resuelve desde:")
	cuerpo += firma
	return asunto, cuerpo
}

func (m *Mensajero) textoDePedidoResuelto(a eventbus.PedidoDeMateriaResuelto) (asunto, cuerpo string) {
	if a.Aprobado {
		asunto = fmt.Sprintf("Ya podés reservar para %s", a.MateriaNombre)
		cuerpo = saludo(a.Nombre) +
			fmt.Sprintf("Se aprobó tu pedido: ya podés reservar computadoras para %s.\n", a.MateriaNombre)
		if strings.TrimSpace(a.Respuesta) != "" {
			cuerpo += fmt.Sprintf("\nTe dejaron dicho: %s\n", strings.TrimSpace(a.Respuesta))
		}
		cuerpo += m.enlace("Podés reservar desde:")
		cuerpo += firma
		return asunto, cuerpo
	}

	asunto = fmt.Sprintf("Sobre tu pedido para dictar %s", a.MateriaNombre)
	cuerpo = saludo(a.Nombre) +
		fmt.Sprintf("No se aprobó tu pedido para dictar %s.\n", a.MateriaNombre)
	if strings.TrimSpace(a.Respuesta) != "" {
		// El motivo va siempre que exista: un rechazo sin explicación manda a
		// la persona a preguntar por qué, y esa conversación empieza mal.
		cuerpo += fmt.Sprintf("\nEl motivo: %s\n", strings.TrimSpace(a.Respuesta))
	}
	cuerpo += firma
	return asunto, cuerpo
}
