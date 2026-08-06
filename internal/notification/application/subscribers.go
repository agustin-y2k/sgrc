package application

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// timeoutNotificacion acota cuánto puede tardar el INSERT de una
// notificación. Sin esto, un Postgres colgado dejaba el handler esperando
// para siempre.
const timeoutNotificacion = 10 * time.Second

// EntregaAsincrona controla si los handlers escriben en su propia goroutine
// (producción) o de forma sincrónica (tests, donde se necesita determinismo).
type EntregaAsincrona bool

const (
	Asincrona EntregaAsincrona = true
	Sincrona  EntregaAsincrona = false
)

// RegisterEventHandlers suscribe al Service a los eventos que auth y
// reservation ya publican — se llama una sola vez desde cmd/main.go,
// después de crear el Service, y antes de levantar el servidor HTTP.
//
// Los errores de estos handlers solo se loguean, nunca se propagan: la
// operación que disparó el evento (registrar un docente, cancelar una
// reserva) ya sucedió y ya se commiteó; notificar es un efecto secundario
// de mejor esfuerzo, no debe poder deshacer ni bloquear nada de lo que ya
// pasó.
//
// Por eso mismo la entrega es asincrónica y con timeout propio. El bus
// publica de forma sincrónica en la goroutine de quien publica (ver
// internal/shared/eventbus), así que un handler lento acá se traduce
// directamente en un request HTTP lento: cancelar una recurrencia de 40
// fechas × 5 PCs emite 200 eventos, y hacer esos 200 INSERT en serie
// dentro del request es justamente lo que no queremos. El contexto tampoco
// puede ser el del request —que se cancela apenas se responde— así que
// cada entrega abre el suyo.
func RegisterEventHandlers(bus eventbus.EventBus, svc *Service) {
	registrarHandlers(bus, svc, Asincrona, nil)
}

// RegisterEventHandlersSincronos es la variante que usan los tests: entrega
// en la misma goroutine, para poder afirmar sobre el resultado sin esperas.
func RegisterEventHandlersSincronos(bus eventbus.EventBus, svc *Service) {
	registrarHandlers(bus, svc, Sincrona, nil)
}

// RegisterEventHandlersConEspera es como la versión asincrónica pero
// registra cada entrega en curso en el WaitGroup, para que un test (o un
// apagado ordenado) pueda esperar a que terminen.
func RegisterEventHandlersConEspera(bus eventbus.EventBus, svc *Service, pendientes *sync.WaitGroup) {
	registrarHandlers(bus, svc, Asincrona, pendientes)
}

// entrega ejecuta el trabajo con su propio contexto acotado, en la
// goroutine que corresponda según el modo. Es lo que usan tanto los avisos
// internos como las copias por correo (ver correos.go), con timeouts
// distintos: escribir una fila no tarda lo mismo que hablar con Gmail.
type entrega func(descripcion string, trabajo func(context.Context) error)

func nuevaEntrega(modo EntregaAsincrona, pendientes *sync.WaitGroup, timeout time.Duration) entrega {
	return func(descripcion string, trabajo func(context.Context) error) {
		correr := func() {
			ctx, cancelar := context.WithTimeout(context.Background(), timeout)
			defer cancelar()
			if err := trabajo(ctx); err != nil {
				log.Printf("notification: error notificando %s: %v", descripcion, err)
			}
		}

		if modo == Sincrona {
			correr()
			return
		}
		if pendientes != nil {
			pendientes.Add(1)
		}
		go func() {
			if pendientes != nil {
				defer pendientes.Done()
			}
			correr()
		}()
	}
}

func registrarHandlers(bus eventbus.EventBus, svc *Service, modo EntregaAsincrona, pendientes *sync.WaitGroup) {
	entregar := nuevaEntrega(modo, pendientes, timeoutNotificacion)

	// RF-05.6: docente nuevo pendiente de aprobación.
	bus.Subscribe("docente.registro.pendiente", func(e eventbus.Evento) {
		payload, ok := e.Payload.(map[string]string)
		if !ok {
			log.Printf("notification: payload inesperado para docente.registro.pendiente: %+v", e.Payload)
			return
		}
		mensaje := fmt.Sprintf("%s %s se registró y está pendiente de aprobación", payload["nombre"], payload["apellido"])
		usuarioID := payload["usuarioId"]
		entregar("docente.registro.pendiente", func(ctx context.Context) error {
			// El aviso guarda DE QUIÉN habla: es lo que permite cerrarlo solo
			// cuando esa cuenta se aprueba o se rechaza, sin que cada Admin
			// tenga que marcarlo a mano (ver Service.CerrarAvisosSobreUsuario).
			_, err := svc.NotificarATodosLosAdmins(ctx, mensaje, domain.TipoDocentePendiente,
				domain.Referencias{SobreUsuarioID: &usuarioID})
			return err
		})
	})

	// RF-02.8: se dio de baja al único docente de una materia, sus
	// reservas futuras se cancelaron en cascada.
	bus.Subscribe("docente.baja.materia-huerfana", func(e eventbus.Evento) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			log.Printf("notification: payload inesperado para docente.baja.materia-huerfana: %+v", e.Payload)
			return
		}
		mensaje := fmt.Sprintf("Se cancelaron %v reserva(s): el único docente de una materia fue dado de baja", payload["reservasCanceladas"])
		entregar("docente.baja.materia-huerfana", func(ctx context.Context) error {
			_, err := svc.NotificarATodosLosAdmins(ctx, mensaje, domain.TipoGeneral, domain.Referencias{})
			return err
		})
	})

	// RF-02.8 por el otro camino: no se dio de baja a nadie, se le quitó la
	// asignación al último docente de una materia. Es un evento aparte y no
	// el de arriba porque para el Admin que lo lee no son la misma noticia:
	// una habla de una cuenta dada de baja y la otra de una asignación que
	// alguien quitó a mano.
	bus.Subscribe("docente.desasignado.materia-huerfana", func(e eventbus.Evento) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			log.Printf("notification: payload inesperado para docente.desasignado.materia-huerfana: %+v", e.Payload)
			return
		}
		mensaje := fmt.Sprintf("Se cancelaron %v reserva(s): se quitó al último docente asignado a una materia", payload["reservasCanceladas"])
		entregar("docente.desasignado.materia-huerfana", func(ctx context.Context) error {
			_, err := svc.NotificarATodosLosAdmins(ctx, mensaje, domain.TipoGeneral, domain.Referencias{})
			return err
		})
	})

	// RF-05.4: se dio de baja a un docente, pero la materia sigue
	// teniendo otro docente asignado — aviso informativo, sin cascada.
	bus.Subscribe("docente.baja.notificar_admin", func(e eventbus.Evento) {
		mensaje := "Se dio de baja a un docente de una materia que sigue teniendo otro docente asignado"
		entregar("docente.baja.notificar_admin", func(ctx context.Context) error {
			_, err := svc.NotificarATodosLosAdmins(ctx, mensaje, domain.TipoGeneral, domain.Referencias{})
			return err
		})
	})

	// Una cuenta que estaba pendiente se aprobó o se rechazó: el aviso que
	// pedía resolverla ya no tiene nada que pedir. Se cierra para TODOS los
	// Admin, no solo para el que la resolvió — a los demás los mandaría a
	// una lista donde esa persona ya no está.
	bus.Subscribe("cuenta.pendiente.resuelta", func(e eventbus.Evento) {
		payload, ok := e.Payload.(map[string]string)
		if !ok {
			log.Printf("notification: payload inesperado para cuenta.pendiente.resuelta: %+v", e.Payload)
			return
		}
		usuarioID := payload["usuarioId"]
		entregar("cuenta.pendiente.resuelta", func(ctx context.Context) error {
			_, err := svc.CerrarAvisosSobreUsuario(ctx, usuarioID, domain.TipoDocentePendiente)
			return err
		})
	})

	// RF-05.1/05.2/05.3: una reserva puntual se canceló (manual,
	// evaluación estatal, o cambio de estado de PC) — el mismo evento
	// cubre los tres casos, el motivo ya viene armado desde reservation.
	bus.Subscribe("reserva.cancelada", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.CancelacionesDeUsuario)
		if !ok {
			log.Printf("notification: payload inesperado para reserva.cancelada: %+v", e.Payload)
			return
		}
		mensaje := mensajeDeCancelacion(payload)
		// El vínculo a la reserva solo tiene sentido si es una sola: con un
		// lote no hay "la" reserva a la que apuntar.
		var reservaID *string
		if len(payload.Reservas) == 1 {
			id := payload.Reservas[0].ReservaID
			reservaID = &id
		}
		entregar("reserva.cancelada", func(ctx context.Context) error {
			_, err := svc.NotificarUsuario(ctx, payload.UsuarioID, mensaje, domain.TipoReservaCancelada,
				domain.Referencias{ReservaID: reservaID})
			return err
		})
	})
}

// maxPCsEnElMensaje acota el listado: un bloqueo sobre un carro entero
// puede alcanzar 30 PCs de un mismo docente, y un mensaje con treinta
// números no se lee, se saltea.
const maxPCsEnElMensaje = 8

// mensajeDeCancelacion arma UNA frase para todo lo que se le canceló a un
// docente de una sola vez.
//
// Con un evento por Reserva, bloquear tres PCs de una misma reserva para
// una evaluación dejaría tres avisos idénticos en la campana. El docente
// vive eso como una sola cosa —"me sacaron la clase"— y lo que necesita
// saber es qué PCs, no cuántas filas se actualizaron.
//
// El prefijo vive SOLO acá. Quien publica manda la razón pelada ("acto
// escolar", "la PC 3 pasó a FUERA_DE_SERVICIO"): si además armara la frase
// entera, el mensaje salía con el prefijo dos veces.
func mensajeDeCancelacion(p eventbus.CancelacionesDeUsuario) string {
	if len(p.Reservas) == 1 {
		r := p.Reservas[0]
		return fmt.Sprintf("Tu reserva del %s (%s) fue cancelada: %s",
			formatearFecha(r.Fecha), nombrePC(r.PCIdentificador), p.Motivo)
	}

	if fecha, unica := fechaUnica(p.Reservas); unica {
		return fmt.Sprintf("Se cancelaron %d de tus reservas del %s (%s): %s",
			len(p.Reservas), formatearFecha(fecha), listaDePCs(p.Reservas), p.Motivo)
	}

	return fmt.Sprintf("Se cancelaron %d de tus reservas (%s): %s",
		len(p.Reservas), listaDePCs(p.Reservas), p.Motivo)
}

// fechaUnica dice si todas las cancelaciones caen el mismo día — el caso
// habitual (un bloqueo por evaluación, varias PCs de la misma clase), y el
// que permite nombrar la fecha una sola vez en vez de repetirla por PC.
func fechaUnica(reservas []eventbus.ReservaCancelada) (time.Time, bool) {
	primera := reservas[0].Fecha
	for _, r := range reservas[1:] {
		if !mismoDia(r.Fecha, primera) {
			return time.Time{}, false
		}
	}
	return primera, true
}

func mismoDia(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// listaDePCs enumera las PCs afectadas, sin repetir y en orden. Si son
// muchas, corta y dice cuántas quedaron afuera.
func listaDePCs(reservas []eventbus.ReservaCancelada) string {
	vistas := map[int]bool{}
	var ids []int
	for _, r := range reservas {
		if r.PCIdentificador > 0 && !vistas[r.PCIdentificador] {
			vistas[r.PCIdentificador] = true
			ids = append(ids, r.PCIdentificador)
		}
	}
	if len(ids) == 0 {
		// No se pudieron resolver los identificadores: el aviso sale igual,
		// sin el detalle. Perder la notificación sería mucho peor.
		return fmt.Sprintf("%d PCs", len(reservas))
	}
	sort.Ints(ids)

	sobrantes := 0
	if len(ids) > maxPCsEnElMensaje {
		sobrantes = len(ids) - maxPCsEnElMensaje
		ids = ids[:maxPCsEnElMensaje]
	}

	partes := make([]string, len(ids))
	for i, id := range ids {
		partes[i] = nombrePC(id)
	}
	texto := strings.Join(partes, ", ")
	if sobrantes > 0 {
		return fmt.Sprintf("%s y %d más", texto, sobrantes)
	}
	return texto
}

func nombrePC(identificador int) string {
	if identificador <= 0 {
		return "una PC"
	}
	return fmt.Sprintf("PC %d", identificador)
}

func formatearFecha(f time.Time) string {
	return f.Format("02/01/2006")
}
