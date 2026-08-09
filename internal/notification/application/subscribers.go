package application

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
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

	// RF-05.9: hay licencias de software por vencer o ya vencidas. Lo
	// dispara el barrido de inventory, no un request: es el único aviso del
	// sistema que nace de un reloj y no de que alguien haya hecho algo.
	bus.Subscribe("licencia.por-vencer", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.AvisoDeLicencias)
		if !ok {
			log.Printf("notification: payload inesperado para licencia.por-vencer: %+v", e.Payload)
			return
		}
		if payload.Total() == 0 {
			return
		}
		mensaje := mensajeDeLicencias(payload)
		entregar("licencia.por-vencer", func(ctx context.Context) error {
			_, err := svc.NotificarATodosLosAdmins(ctx, mensaje, domain.TipoLicenciaPorVencer,
				domain.Referencias{})
			return err
		})
	})

	// ── El barrido de reservas y entregas (RF-08.10 a RF-08.13) ─────
	//
	// Los cinco de abajo los dispara un reloj, no una persona. La
	// idempotencia —que no salgan dos veces— la garantizan las marcas de
	// cada fila del lado de reservation, no estos handlers.

	bus.Subscribe("reserva.recordatorio", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.RecordatorioDeReserva)
		if !ok {
			log.Printf("notification: payload inesperado para reserva.recordatorio: %+v", e.Payload)
			return
		}
		if payload.UsuarioID == "" {
			return // un bloqueo por evaluación no tiene a quién avisarle
		}
		mensaje := mensajeDeRecordatorio(payload)
		entregar("reserva.recordatorio", func(ctx context.Context) error {
			_, err := svc.NotificarUsuario(ctx, payload.UsuarioID, mensaje,
				domain.TipoReservaPorComenzar, domain.Referencias{})
			return err
		})
	})

	bus.Subscribe("reserva.equipo-no-disponible", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.EquipoNoDisponibleParaReserva)
		if !ok {
			log.Printf("notification: payload inesperado para reserva.equipo-no-disponible: %+v", e.Payload)
			return
		}
		if payload.UsuarioID == "" {
			return
		}
		mensaje := mensajeDeEquipoNoDisponible(payload)
		entregar("reserva.equipo-no-disponible", func(ctx context.Context) error {
			_, err := svc.NotificarUsuario(ctx, payload.UsuarioID, mensaje,
				domain.TipoReservaPorComenzar, domain.Referencias{})
			return err
		})
	})

	bus.Subscribe("reserva.no-retirada", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.ReservasLiberadas)
		if !ok {
			log.Printf("notification: payload inesperado para reserva.no-retirada: %+v", e.Payload)
			return
		}
		if payload.UsuarioID == "" {
			return
		}
		mensaje := mensajeDeReservasLiberadas(payload)
		entregar("reserva.no-retirada", func(ctx context.Context) error {
			_, err := svc.NotificarUsuario(ctx, payload.UsuarioID, mensaje,
				domain.TipoReservaNoRetirada, domain.Referencias{})
			return err
		})
	})

	// Los dos de las máquinas que no volvieron van a los Admin: son ellos
	// quienes pueden ir a buscarlas.
	bus.Subscribe("prestamo.demorado", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.PrestamosDemorados)
		if !ok {
			log.Printf("notification: payload inesperado para prestamo.demorado: %+v", e.Payload)
			return
		}
		if len(payload.Prestamos) == 0 {
			return
		}
		mensaje := mensajeDePrestamosDemorados(payload)
		entregar("prestamo.demorado", func(ctx context.Context) error {
			_, err := svc.NotificarATodosLosAdmins(ctx, mensaje, domain.TipoEquipoSinDevolver,
				domain.Referencias{})
			return err
		})
	})

	bus.Subscribe("prestamo.sin-devolver.cierre", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.EquiposSinDevolverAlCierre)
		if !ok {
			log.Printf("notification: payload inesperado para prestamo.sin-devolver.cierre: %+v", e.Payload)
			return
		}
		if len(payload.Equipos) == 0 {
			return
		}
		mensaje := mensajeDeCierre(payload)
		entregar("prestamo.sin-devolver.cierre", func(ctx context.Context) error {
			_, err := svc.NotificarATodosLosAdmins(ctx, mensaje, domain.TipoEquipoSinDevolver,
				domain.Referencias{})
			return err
		})

		// Y al docente que la tiene reservada, si hay uno: es el único para
		// quien esto es accionable antes de mañana.
		for _, pc := range payload.Equipos {
			if pc.ProximoUsuarioID == "" {
				continue
			}
			usuarioID, aviso := pc.ProximoUsuarioID, pc
			entregar("prestamo.sin-devolver.cierre (docente siguiente)", func(ctx context.Context) error {
				mensaje := fmt.Sprintf("%s, que tenés reservado para el %s, quedó fuera del laboratorio al cierre",
					aviso.Etiqueta, formatearFecha(aviso.ProximaFecha))
				_, err := svc.NotificarUsuario(ctx, usuarioID, mensaje,
					domain.TipoReservaPorComenzar, domain.Referencias{})
				return err
			})
		}
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

// maxEquiposEnElMensaje acota el listado: un bloqueo sobre un carro entero
// puede alcanzar 30 PCs de un mismo docente, y un mensaje con treinta
// números no se lee, se saltea.
const maxEquiposEnElMensaje = 8

// mensajeDeCancelacion arma UNA frase para todo lo que se le canceló a un
// docente de una sola vez.
//
// Con un evento por Reserva, bloquear tres PCs de una misma reserva para
// una evaluación dejaría tres avisos idénticos en la campana. El docente
// vive eso como una sola cosa —"me sacaron la clase"— y lo que necesita
// saber es qué equipos, no cuántas filas se actualizaron.
//
// El prefijo vive SOLO acá. Quien publica manda la razón pelada ("acto
// escolar", "la PC 3 pasó a FUERA_DE_SERVICIO"): si además armara la frase
// entera, el mensaje salía con el prefijo dos veces.
func mensajeDeCancelacion(p eventbus.CancelacionesDeUsuario) string {
	if len(p.Reservas) == 1 {
		r := p.Reservas[0]
		return fmt.Sprintf("Tu reserva del %s (%s) fue cancelada: %s",
			formatearFecha(r.Fecha), etiquetaODefecto(r.Etiqueta), p.Motivo)
	}

	if fecha, unica := fechaUnica(p.Reservas); unica {
		return fmt.Sprintf("Se cancelaron %d de tus reservas del %s (%s): %s",
			len(p.Reservas), formatearFecha(fecha), equiposDeLasCanceladas(p.Reservas), p.Motivo)
	}

	return fmt.Sprintf("Se cancelaron %d de tus reservas (%s): %s",
		len(p.Reservas), equiposDeLasCanceladas(p.Reservas), p.Motivo)
}

// fechaUnica dice si todas las cancelaciones caen el mismo día — el caso
// habitual (un bloqueo por evaluación, varios equipos de la misma clase), y el
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

// equiposDeLasCanceladas enumera los equipos afectados, sin repetir y en orden. Si son
// muchos, corta y dice cuántos quedaron afuera.
func equiposDeLasCanceladas(reservas []eventbus.ReservaCancelada) string {
	vistas := map[string]bool{}
	var ids []string
	for _, r := range reservas {
		if r.Etiqueta != "" && !vistas[r.Etiqueta] {
			vistas[r.Etiqueta] = true
			ids = append(ids, r.Etiqueta)
		}
	}
	if len(ids) == 0 {
		// No se pudieron resolver las etiquetas: el aviso sale igual, sin el
		// detalle. Perder la notificación sería mucho peor.
		return fmt.Sprintf("%d equipos", len(reservas))
	}
	// Orden natural y no alfabético: con sort.Strings, "PC 12" va antes que
	// "PC 3" porque compara carácter por carácter. El docente lee la lista
	// de sus máquinas, y verlas desordenadas hace dudar de si son las suyas.
	sort.Slice(ids, func(i, j int) bool { return menorEnOrdenNatural(ids[i], ids[j]) })

	sobrantes := 0
	if len(ids) > maxEquiposEnElMensaje {
		sobrantes = len(ids) - maxEquiposEnElMensaje
		ids = ids[:maxEquiposEnElMensaje]
	}

	texto := strings.Join(ids, ", ")
	if sobrantes > 0 {
		return fmt.Sprintf("%s y %d más", texto, sobrantes)
	}
	return texto
}

// menorEnOrdenNatural compara etiquetas dejando los números al final en
// orden numérico: "PC 3" antes que "PC 12", y "Proyector Epson" donde le
// toque alfabéticamente.
//
// Hace falta desde que las etiquetas son texto: antes eran enteros y
// el orden salía solo.
func menorEnOrdenNatural(a, b string) bool {
	prefijoA, numeroA := partirEtiqueta(a)
	prefijoB, numeroB := partirEtiqueta(b)
	if prefijoA != prefijoB {
		return prefijoA < prefijoB
	}
	return numeroA < numeroB
}

// partirEtiqueta separa "PC 12" en ("PC ", 12). Sin número final devuelve la
// etiqueta entera y -1, que la deja antes que cualquier numerada del mismo
// prefijo — un caso que en la práctica no se da, porque o tiene número o es
// un nombre propio.
func partirEtiqueta(s string) (string, int) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return s, -1
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil {
		return s, -1
	}
	return s[:i], n
}

// etiquetaODefecto cubre el caso en que no se pudo resolver el nombre del
// equipo: el aviso sale igual, sin el detalle. Perder la notificación por no
// poder adornarla sería mucho peor.
func etiquetaODefecto(etiqueta string) string {
	if etiqueta == "" {
		return "un equipo"
	}
	return etiqueta
}

func formatearFecha(f time.Time) string {
	return f.Format("02/01/2006")
}
