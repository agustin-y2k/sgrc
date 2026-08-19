// Package http expone las rutas Fiber de notification — ver
// docs/08-api-spec.yaml para el contrato completo de cada endpoint.
package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

type notificacionResponse struct {
	ID        string  `json:"id"`
	ReservaID *string `json:"reservaId,omitempty"`
	Mensaje   string  `json:"mensaje"`
	// Tipo le permite a la interfaz ofrecer la acción que corresponde
	// (ej. "ir a aprobar") sin interpretar el texto del mensaje.
	Tipo     string     `json:"tipo"`
	Estado   string     `json:"estado"`
	CreadaEn time.Time  `json:"creadaEn"`
	LeidaEn  *time.Time `json:"leidaEn,omitempty"`
}

func toNotificacionResponse(n *domain.Notificacion) notificacionResponse {
	return notificacionResponse{
		ID: n.ID, ReservaID: n.ReservaID, Mensaje: n.Mensaje, Tipo: string(n.Tipo),
		Estado: string(n.Estado), CreadaEn: n.CreadaEn, LeidaEn: n.LeidaEn,
	}
}

// listarNotificacionesResponse reemplaza al fiber.Map suelto que devolvía
// este endpoint: con `meta` en juego conviene un tipo, que es además lo que
// los tests pueden deserializar sin adivinar la forma.
type listarNotificacionesResponse struct {
	Data []notificacionResponse `json:"data"`
	Meta paginacion.Meta        `json:"meta"`
}

// ── Preferencias de correo (RF-05.13) ───────────────────────────────────

type preferenciaEmailResponse struct {
	Categoria string `json:"categoria"`
	// Grupo separa "tus avisos" de "los de administración", que es como se
	// muestran: son dos listas con criterios distintos y mezclarlas obliga a
	// leer las catorce para encontrar la que uno busca.
	Grupo string `json:"grupo"`
	// Etiqueta y Descripcion viajan con cada casilla en vez de vivir en la
	// pantalla: la lista de categorías la define el backend, que es quien sabe
	// qué correos existen de verdad, y así no puede quedar una casilla que no
	// apaga nada ni un correo que ninguna casilla nombra.
	Etiqueta    string `json:"etiqueta"`
	Descripcion string `json:"descripcion"`
	Activa      bool   `json:"activa"`
	// Fija: sale siempre y no admite preferencia. Se muestra igual —tildada y
	// sin casilla que tocar— para que se vea que existe: una lista que oculta
	// lo que no se puede cambiar se lee como la lista completa de correos, y
	// no lo es.
	Fija bool `json:"fija"`
}

type preferenciasEmailResponse struct {
	Data []preferenciaEmailResponse `json:"data"`
}

type guardarPreferenciasEmailRequest struct {
	// Categorias es la selección COMPLETA: lo que no viene queda apagado.
	Categorias []string `json:"categorias"`
}

// textoDeCategoria describe cada correo en los términos de quien lo recibe —
// qué le va a llegar y cada cuánto—, porque eso es lo que decide si lo quiere.
var textoDeCategoria = map[domain.CategoriaEmail]struct{ etiqueta, descripcion string }{
	// ── De la cuenta, que no se apagan ───────────────────────────────────
	domain.CatRecuperacionDeCuenta: {
		"Recuperar tu contraseña",
		"El código para restablecerla, o el aviso de que tu cuenta entra con Google. No se puede desactivar: si perdés el acceso, este correo es el único camino de vuelta — no hay aviso en el sistema que puedas leer sin poder entrar.",
	},
	domain.CatCuentaAprobada: {
		"Te aprobaron la cuenta",
		"Cuando un administrador habilita tu cuenta. No se puede desactivar: hasta que llega no tenés forma de saber que ya podés entrar.",
	},

	// ── Personales ────────────────────────────────────────────────────────
	domain.CatSoporteRespondido: {
		"Te contestaron un pedido de ayuda",
		"Cuando el equipo de administración responde algo que preguntaste desde \"Pedir ayuda\". No se puede desactivar: pediste ayuda y estás esperando; un aviso que espera a que entres a mirar no sirve para eso.",
	},
	domain.CatReservaCancelada: {
		"Te cancelaron una computadora",
		"Cuando una computadora que tenías reservada se cancela: pasó a mantenimiento, se rompió, o se bloqueó la franja para un acto o una evaluación. Cancela solo las máquinas que nombra, no toda la clase.",
	},
	domain.CatEquipoNoDisponible: {
		"Una computadora tuya puede no estar",
		"Antes de tu clase, si una de las que reservaste no volvió al laboratorio, y al cierre del día si la que tenés para mañana quedó afuera. Es lo que te da tiempo a conseguir otra.",
	},
	domain.CatPedidoDeLiberacion: {
		"Alguien te pide una computadora que reservaste",
		"Otro docente necesita un equipo que tenés tomado. Tu reserva no cambia: decidís vos.",
	},
	domain.CatPedidoDeMateriaResuelto: {
		"Resolvieron tu pedido de materia",
		"Cuando aprueban o rechazan el pedido para dictar una materia que hiciste desde tu perfil.",
	},
	domain.CatPedidoSobreMiMateria: {
		"Alguien pidió dictar una materia tuya",
		"Cuando otro docente pide que lo asignen a una materia que vos ya das. No hay nada que tengas que hacer: es para que no te enteres tarde.",
	},
	domain.CatSugerenciaRespondida: {
		"Te contestaron lo que escribiste",
		"La respuesta a una sugerencia o a un problema que reportaste por el buzón.",
	},
	domain.CatRecordatorioDeReserva: {
		"Tenés clase en un rato",
		"Una hora antes de cada reserva, con las computadoras y la materia. Uno por clase, no uno por máquina.",
	},
	domain.CatReservaSinRetirar: {
		"Todavía no retiraste tus computadoras",
		"Quince minutos después de empezada la clase, si nadie fue a buscarlas. A los cuarenta la reserva se libera para otro.",
	},
	domain.CatDevolucionPendiente: {
		"Acordate de devolver",
		"Diez minutos después de la hora en que un equipo que retiraste tenía que volver. Sale uno por equipo.",
	},

	// ── De administración ─────────────────────────────────────────────────
	domain.CatSoporte: {
		"Alguien pidió ayuda",
		"Cada vez que un docente escribe desde \"Pedir ayuda\". No se puede desactivar: es el único canal por el que alguien puede pedir auxilio, y del otro lado hay una clase esperando.",
	},
	domain.CatCuentaPendiente: {
		"Cuentas esperando aprobación",
		"Cada vez que alguien se registra y queda pendiente. Hasta que no la apruebes, esa persona no puede entrar al sistema.",
	},
	domain.CatDevolucionDemorada: {
		"Equipos que no volvieron a horario",
		"Cuando vence el plazo de una entrega y el equipo sigue afuera. Un correo por equipo, sin insistir.",
	},
	domain.CatCierreSinDevolver: {
		"Equipos afuera al cerrar la jornada",
		"El resumen de lo que quedó sin devolver al terminar el día, con el docente que lo tiene reservado mañana. Como mucho, uno por día.",
	},
	domain.CatLicenciaPorVencer: {
		"Licencias de software por vencer",
		"Dos correos por licencia: uno con la anticipación configurada y otro el día del vencimiento. Después se calla.",
	},
	domain.CatPedidoDeMateria: {
		"Pedidos para dictar una materia",
		"Cuando un docente pide que lo asignen a una materia y hay que aprobarlo o rechazarlo.",
	},
	domain.CatSugerencia: {
		"Mensajes del buzón",
		"Cada vez que alguien escribe una sugerencia o avisa que algo no anda.",
	},
}

func toPreferenciasEmailResponse(activas []domain.CategoriaEmail, esAdmin bool) preferenciasEmailResponse {
	elegidas := make(map[domain.CategoriaEmail]bool, len(activas))
	for _, c := range activas {
		elegidas[c] = true
	}

	// Se responden SIEMPRE todas las que le corresponden a esa persona, con su
	// estado: la pantalla dibuja lo que llega y no tiene que conocer la lista
	// por su cuenta.
	suyas := domain.CategoriasPara(esAdmin)
	data := make([]preferenciaEmailResponse, len(suyas))
	for i, c := range suyas {
		texto := textoDeCategoria[c]
		data[i] = preferenciaEmailResponse{
			Categoria:   string(c),
			Grupo:       string(c.Grupo()),
			Etiqueta:    texto.etiqueta,
			Descripcion: texto.descripcion,
			Activa:      elegidas[c],
			Fija:        c.EsFija(),
		}
	}
	return preferenciasEmailResponse{Data: data}
}
