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
// notificación.
const timeoutNotificacion = 10 * time.Second

// EntregaAsincrona controla si los handlers escriben en su propia goroutine
// (producción) o de forma sincrónica (tests, donde se necesita determinismo).
type EntregaAsincrona bool

const (
	Asincrona EntregaAsincrona = true
	Sincrona  EntregaAsincrona = false
)

// RegisterEventHandlers suscribe al Service a los eventos que auth y
// reservation ya publican — se llama una sola vez desde cmd/main.go, después
// de crear el Service, y antes de levantar el servidor HTTP. Los errores de
// estos handlers solo se loguean, nunca se propagan: la operación que disparó
// el evento (registrar un docente, cancelar una reserva) ya sucedió y ya se
// commiteó; notificar es un efecto secundario de mejor esfuerzo, no debe
// poder deshacer ni bloquear nada de lo que ya pasó.
func RegisterEventHandlers(bus eventbus.EventBus, svc *Service) {
	registrarHandlers(bus, svc, Asincrona, nil)
}

// RegisterEventHandlersSincronos es la variante que usan los tests: entrega
// en la misma goroutine, para poder afirmar sobre el resultado sin esperas.
func RegisterEventHandlersSincronos(bus eventbus.EventBus, svc *Service) {
	registrarHandlers(bus, svc, Sincrona, nil)
}

// RegisterEventHandlersConEspera es como la versión asincrónica pero registra
// cada entrega en curso en el WaitGroup, para que un test (o un apagado
// ordenado) pueda esperar a que terminen.
func RegisterEventHandlersConEspera(bus eventbus.EventBus, svc *Service, pendientes *sync.WaitGroup) {
	registrarHandlers(bus, svc, Asincrona, pendientes)
}

// entrega ejecuta el trabajo con su propio contexto acotado, en la goroutine
// que corresponda según el modo.
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
			// El aviso guarda DE QUIÉN habla: es lo que permite cerrarlo solo cuando
			// esa cuenta se aprueba o se rechaza, sin que cada Admin tenga que
			// marcarlo a mano (ver Service.CerrarAvisosSobreUsuario).
			_, err := svc.NotificarATodosLosAdmins(ctx, mensaje, domain.TipoDocentePendiente,
				domain.Referencias{SobreUsuarioID: &usuarioID})
			return err
		})
	})

	// RF-02.8, por sus dos caminos: una materia se quedó sin ningún docente y
	// sus reservas futuras se cancelaron en cascada.
	//
	// Son dos eventos y no uno porque para el Admin que lo lee "se dio de baja
	// al docente" y "se le quitó la materia" no son la misma noticia, aunque
	// la consecuencia sea idéntica. Nunca salen los dos por un mismo hecho.
	//
	// Lo que NO existe más es el tercer aviso de esta familia (era RF-05.4):
	// el de la materia que se queda con otro docente y no cancela nada. Ver
	// auth/application/service.go, donde se dejó de publicar.
	porMateriaHuerfana := func(evento, motivo string) {
		bus.Subscribe(evento, func(e eventbus.Evento) {
			payload, ok := e.Payload.(map[string]any)
			if !ok {
				log.Printf("notification: payload inesperado para %s: %+v", evento, e.Payload)
				return
			}
			mensaje := fmt.Sprintf("Se cancelaron %v reserva(s): %s", payload["reservasCanceladas"], motivo)
			entregar(evento, func(ctx context.Context) error {
				_, err := svc.NotificarATodosLosAdmins(ctx, mensaje, domain.TipoGeneral, domain.Referencias{})
				return err
			})
		})
	}
	porMateriaHuerfana("docente.baja.materia-huerfana",
		"el único docente de una materia fue dado de baja")
	porMateriaHuerfana("docente.desasignado.materia-huerfana",
		"se quitó al último docente asignado a una materia")

	// Una cuenta que estaba pendiente se aprobó o se rechazó: el aviso que pedía
	// resolverla ya no tiene nada que pedir.
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

	// Alguien escribió en el buzón: sugerencia o algo que no anda. Va a
	// todos los Admin, salvo a los que ya tienen uno sin leer de esa misma
	// persona (ver NotificarATodosLosAdminsSinRepetir).
	bus.Subscribe("sugerencia.nueva", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.SugerenciaNueva)
		if !ok {
			log.Printf("notification: payload inesperado para sugerencia.nueva: %+v", e.Payload)
			return
		}
		mensaje := mensajeDeSugerencia(payload)
		entregar("sugerencia.nueva", func(ctx context.Context) error {
			_, err := svc.NotificarATodosLosAdminsSinRepetir(ctx, payload.UsuarioID, mensaje,
				domain.TipoSugerencia)
			return err
		})
	})

	// Quien preguntó volvió a escribir en su hilo. Es el caso que más se
	// beneficia del "sin repetir": una conversación de seis mensajes dejaba
	// seis avisos idénticos en la campana de cada Admin.
	bus.Subscribe("sugerencia.seguimiento", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.SugerenciaSeguimiento)
		if !ok {
			log.Printf("notification: payload inesperado para sugerencia.seguimiento: %+v", e.Payload)
			return
		}
		mensaje := mensajeDeSeguimiento(payload)
		entregar("sugerencia.seguimiento", func(ctx context.Context) error {
			_, err := svc.NotificarATodosLosAdminsSinRepetir(ctx, payload.UsuarioID, mensaje,
				domain.TipoSugerencia)
			return err
		})
	})

	// Un Admin contestó: le llega a quien escribió, y el pendiente se cierra
	// para TODOS los Admin — el que contestó ya no tiene nada que hacer, y los
	// demás tampoco.
	bus.Subscribe("sugerencia.respondida", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.SugerenciaRespondida)
		if !ok {
			log.Printf("notification: payload inesperado para sugerencia.respondida: %+v", e.Payload)
			return
		}
		mensaje := mensajeDeRespuestaASugerencia(payload)
		entregar("sugerencia.respondida", func(ctx context.Context) error {
			if _, err := svc.CerrarAvisosSobreUsuario(ctx, payload.UsuarioID, domain.TipoSugerencia); err != nil {
				return err
			}
			_, err := svc.NotificarUsuario(ctx, payload.UsuarioID, mensaje,
				domain.TipoSugerenciaRespondida, domain.Referencias{})
			return err
		})
	})

	// Un docente pidió dictar una materia.
	bus.Subscribe("materia.pedido.nuevo", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.PedidoDeMateriaNuevo)
		if !ok {
			log.Printf("notification: payload inesperado para materia.pedido.nuevo: %+v", e.Payload)
			return
		}
		// Solo a los Admin, que son quienes deciden. A quien ya dicta la
		// materia no se le avisa: no se le pide nada y no puede hacer nada.
		//
		// Guarda de quién habla, que es lo que permite cerrarlo solo cuando
		// alguno lo resuelve (ver materia.pedido.resuelto).
		usuarioID := payload.UsuarioID
		entregar("materia.pedido.nuevo", func(ctx context.Context) error {
			_, err := svc.NotificarATodosLosAdmins(ctx, mensajeDePedidoDeMateria(payload),
				domain.TipoPedidoDeMateria, domain.Referencias{SobreUsuarioID: &usuarioID})
			return err
		})
	})

	// El Admin resolvió el pedido: le llega a quien lo hizo, aprobado o no.
	bus.Subscribe("materia.pedido.resuelto", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.PedidoDeMateriaResuelto)
		if !ok {
			log.Printf("notification: payload inesperado para materia.pedido.resuelto: %+v", e.Payload)
			return
		}
		mensaje := mensajeDePedidoResuelto(payload)
		entregar("materia.pedido.resuelto", func(ctx context.Context) error {
			// Lo decidió uno: deja de estar pendiente para todos.
			if _, err := svc.CerrarAvisosSobreUsuario(ctx, payload.UsuarioID, domain.TipoPedidoDeMateria); err != nil {
				return err
			}
			_, err := svc.NotificarUsuario(ctx, payload.UsuarioID, mensaje,
				domain.TipoPedidoDeMateriaResuelto, domain.Referencias{})
			return err
		})
	})

	// RF-05.9: hay licencias de software por vencer o ya vencidas.
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

	// ── Los dos avisos que se cierran cuando se resuelve lo que los motivó ──
	//
	// Estos no hablan de una persona, así que no se pueden cerrar con
	// CerrarAvisosSobreUsuario: hablan de un conjunto —"hay licencias por
	// renovar", "quedaron equipos afuera"— que además se rearma cada vez,
	// porque las licencias no vencen todas el mismo día. Por eso el cierre no
	// es "se renovó una" sino "ya no queda ninguna", y quien sabe eso es el
	// módulo dueño, que lo manda contado.

	bus.Subscribe("licencia.pendientes", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.PendientesDeLicencia)
		if !ok {
			log.Printf("notification: payload inesperado para licencia.pendientes: %+v", e.Payload)
			return
		}
		if payload.Pendientes > 0 {
			return // todavía queda trabajo: el aviso sigue diciendo la verdad
		}
		entregar("licencia.pendientes", func(ctx context.Context) error {
			_, err := svc.CerrarAvisosPendientesDe(ctx, domain.TipoLicenciaPorVencer)
			return err
		})
	})

	bus.Subscribe("prestamo.cierre.pendientes", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.PendientesDelCierre)
		if !ok {
			log.Printf("notification: payload inesperado para prestamo.cierre.pendientes: %+v", e.Payload)
			return
		}
		if payload.Pendientes > 0 {
			return // sigue habiendo equipos afuera de los que ya se avisó
		}
		entregar("prestamo.cierre.pendientes", func(ctx context.Context) error {
			_, err := svc.CerrarAvisosPendientesDe(ctx, domain.TipoEquipoSinDevolver)
			return err
		})
	})

	// ── El barrido de reservas y entregas (RF-08.10 a RF-08.13) ───── Los cinco
	// de abajo los dispara un reloj, no una persona.

	// El recordatorio de "en un rato tenés clase" NO escribe en la campana:
	// sale solo como correo, y apagado por defecto (RF-05.13). Era el aviso de
	// mayor volumen del sistema —uno por clase y por día, para siempre— y el
	// único que no traía ninguna noticia.
	//
	// Y el aviso de "una PC tuya no volvió al laboratorio" ya no existe en
	// ningún canal: quien resuelve eso es el Admin en el mostrador, cambiando
	// la máquina que falta por otra libre, y se entera mirando la pantalla de
	// entregas. Avisarle al docente una hora antes era pedirle que resolviera
	// algo que no puede resolver.

	// RF-04.12. La referencia a la reserva y a quien pide no es decorativa: es
	// lo que sostiene la regla de un pedido por reserva, por solicitante y por
	// día, que se verifica preguntando si esta fila ya existe.
	bus.Subscribe("reserva.pedido-de-liberacion", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.PedidoDeLiberacion)
		if !ok {
			log.Printf("notification: payload inesperado para reserva.pedido-de-liberacion: %+v", e.Payload)
			return
		}
		if payload.UsuarioID == "" {
			return
		}
		mensaje := mensajeDePedidoDeLiberacion(payload)
		reservaID, solicitanteID := payload.ReservaID, payload.SolicitanteID
		entregar("reserva.pedido-de-liberacion", func(ctx context.Context) error {
			_, err := svc.NotificarUsuario(ctx, payload.UsuarioID, mensaje,
				domain.TipoPedidoDeLiberacion, domain.Referencias{
					ReservaID: &reservaID,
					// SobreUsuarioID es de quién HABLA el aviso: le llega al
					// dueño y trata sobre el otro docente.
					SobreUsuarioID: &solicitanteID,
				})
			return err
		})
	})

	// El corte de fin de jornada va a los Admin: son ellos quienes pueden ir a
	// buscar las máquinas.
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

		// Al docente de la próxima reserva NO se le avisa acá. El corte sale de
		// noche, cuando ya no puede hacer nada, y el aviso de "tu PC puede no
		// estar" le llega igual una hora antes de su clase —que es cuando
		// todavía puede conseguir otra— desde reserva.equipo-no-disponible.
		// Mandar los dos es un aviso de madrugada que no cambia nada.
	})

	// RF-05.1/05.2/05.3: una reserva puntual se canceló (manual, bloqueo
	// administrativo, o cambio de estado del equipo) — el mismo evento cubre los
	// tres casos, el motivo ya viene armado desde reservation.
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
// puede alcanzar 30 PCs de un mismo docente, y un mensaje con treinta números
// no se lee, se saltea.
const maxEquiposEnElMensaje = 8

// mensajeDeCancelacion arma UNA frase para todo lo que se le canceló a un
// docente de una sola vez.
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
// habitual (un bloqueo administrativo, varios equipos de la misma clase), y
// el que permite nombrar la fecha una sola vez en vez de repetirla por PC.
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
	// Orden natural y no alfabético: con sort.Strings, "PC 12" va antes que "PC
	// 3" porque compara carácter por carácter.
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

// menorEnOrdenNatural compara etiquetas dejando los números al final en orden
// numérico: "PC 3" antes que "PC 12", y "Proyector Epson" donde le toque
// alfabéticamente.
func menorEnOrdenNatural(a, b string) bool {
	prefijoA, numeroA := partirEtiqueta(a)
	prefijoB, numeroB := partirEtiqueta(b)
	if prefijoA != prefijoB {
		return prefijoA < prefijoB
	}
	return numeroA < numeroB
}

// partirEtiqueta separa "PC 12" en ("PC ", 12).
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
// equipo: el aviso sale igual, sin el detalle.
func etiquetaODefecto(etiqueta string) string {
	if etiqueta == "" {
		return "un equipo"
	}
	return etiqueta
}

func formatearFecha(f time.Time) string {
	return f.Format("02/01/2006")
}
