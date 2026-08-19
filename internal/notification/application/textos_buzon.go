package application

import (
	"fmt"
	"strings"

	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// Los textos del buzón de sugerencias y de los pedidos para dictar una
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

func mensajeDeSugerencia(a eventbus.SugerenciaNueva) string {
	// "avisó que algo no anda" y no "reportó un problema": lo primero es lo
	// que la persona hizo, lo segundo es cómo lo llamaría un sistema.
	que := "dejó una sugerencia"
	if a.Tipo == "PROBLEMA" {
		que = "avisó que algo no anda"
	}

	base := fmt.Sprintf("%s %s: «%s»", quienODefecto(a.Quien), que, recortar(a.Texto))
	if a.Pantalla != "" {
		base += fmt.Sprintf(" (desde %s)", a.Pantalla)
	}
	return base
}

func mensajeDeRespuestaASugerencia(a eventbus.SugerenciaRespondida) string {
	return fmt.Sprintf("Te contestaron lo que escribiste («%s»): %s",
		recortar(a.TextoOriginal), recortar(a.Respuesta))
}

func (m *Mensajero) textoDeSugerencia(a eventbus.SugerenciaNueva) (asunto, cuerpo string) {
	if a.Tipo == "PROBLEMA" {
		asunto = "Alguien avisó que algo del sistema no anda"
	} else {
		asunto = "Llegó una sugerencia"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s escribió desde el sistema:\n\n", quienODefecto(a.Quien))
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

func (m *Mensajero) textoDeRespuestaASugerencia(a eventbus.SugerenciaRespondida) (asunto, cuerpo string) {
	asunto = "Te contestaron lo que escribiste"

	cuerpo = saludo(a.Nombre) +
		fmt.Sprintf("Sobre lo que escribiste:\n\n  %s\n\nTe contestaron:\n\n  %s\n",
			strings.TrimSpace(a.TextoOriginal), strings.TrimSpace(a.Respuesta))
	cuerpo += m.enlace("Podés ver todo lo que escribiste en:")
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

// mensajeDePedidoParaElTitular es lo que le llega a quien YA dicta esa
// materia.
func mensajeDePedidoParaElTitular(a eventbus.PedidoDeMateriaNuevo) string {
	return fmt.Sprintf(
		"%s pidió dictar %s, que también das vos. Lo resuelve el equipo de administración; "+
			"si tenés algo que decir al respecto, habla con ellos.",
		quienODefecto(a.Nombre), nombreDeLaMateria(a.MateriaNombre, a.CursoNombre))
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

func (m *Mensajero) textoDePedidoParaElTitular(a eventbus.PedidoDeMateriaNuevo) (asunto, cuerpo string) {
	asunto = fmt.Sprintf("Alguien pidió dictar %s", a.MateriaNombre)

	cuerpo = fmt.Sprintf(
		"%s pidió poder reservar computadoras para %s, que también das vos.\n\n"+
			"No hay nada que tengas que hacer: lo resuelve el equipo de administración. "+
			"Te llega para que no te enteres tarde — si tenés algo que decir, hablalo con ellos.\n",
		quienODefecto(a.Nombre), nombreDeLaMateria(a.MateriaNombre, a.CursoNombre))
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
