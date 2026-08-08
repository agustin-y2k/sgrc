package application

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// Copias por correo de los avisos del sistema (RF-05.8).
//
// Este archivo es el ÚNICO lugar del proyecto donde se escribe el texto de
// un mail. Está separado de subscribers.go aunque los dos escuchen algunos
// eventos en común, porque son dos canales con reglas distintas: el aviso
// interno es la fuente de verdad —se guarda, se marca como leído, se puede
// cerrar solo— y el correo es una copia que sale de la escuela y ya no
// vuelve.
//
// Por eso son suscriptores aparte y no una rama dentro de los otros: el bus
// admite varios handlers por evento, así que si un envío falla o tarda, el
// aviso interno ya se escribió.

// timeoutCorreo es más largo que timeoutNotificacion: aquello es un INSERT
// contra un Postgres de la misma red, esto es un saludo SMTP, un STARTTLS,
// una autenticación y una transferencia contra un servidor de Google.
const timeoutCorreo = 45 * time.Second

// Mensajero manda los correos. Es un tipo propio y no métodos del Service
// porque no comparte nada con él: no toca el repositorio ni el reloj, y su
// dependencia (el enviador) es opcional, mientras que las del Service no.
type Mensajero struct {
	enviador EnviadorDeEmail
	admins   ListadorAdmins
	// urlDelSistema es FRONTEND_ORIGIN, la dirección pública desde la que
	// se entra. Va en casi todos los mails: decirle a alguien que su cuenta
	// está aprobada sin decirle a dónde entrar lo manda a buscar el link a
	// otro lado.
	urlDelSistema string
}

func NewMensajero(enviador EnviadorDeEmail, admins ListadorAdmins, urlDelSistema string) *Mensajero {
	return &Mensajero{
		enviador:      enviador,
		admins:        admins,
		urlDelSistema: strings.TrimRight(strings.TrimSpace(urlDelSistema), "/"),
	}
}

// ══════════════════════════════════════════════════════════════════
// Registro de los suscriptores
// ══════════════════════════════════════════════════════════════════

// RegisterEmailHandlers suscribe los envíos de correo. Se llama desde
// cmd/main.go, al lado de RegisterEventHandlers.
func RegisterEmailHandlers(bus eventbus.EventBus, m *Mensajero) {
	registrarHandlersDeCorreo(bus, m, Asincrona, nil)
}

// RegisterEmailHandlersSincronos entrega en la misma goroutine — la
// variante de los tests.
func RegisterEmailHandlersSincronos(bus eventbus.EventBus, m *Mensajero) {
	registrarHandlersDeCorreo(bus, m, Sincrona, nil)
}

// RegisterEmailHandlersConEspera registra cada envío en curso en el
// WaitGroup, para que el apagado ordenado no corte un mail a la mitad.
func RegisterEmailHandlersConEspera(bus eventbus.EventBus, m *Mensajero, pendientes *sync.WaitGroup) {
	registrarHandlersDeCorreo(bus, m, Asincrona, pendientes)
}

func registrarHandlersDeCorreo(bus eventbus.EventBus, m *Mensajero, modo EntregaAsincrona, pendientes *sync.WaitGroup) {
	enviar := nuevaEntrega(modo, pendientes, timeoutCorreo)

	// ── Aviso a los Admin: alguien se registró ──────────────────────
	//
	// Mismo evento que dispara el aviso interno (RF-05.6). El mail es lo que
	// hace que un Admin se entere sin tener la pantalla abierta: una cuenta
	// pendiente que nadie mira es un docente que no puede trabajar.
	bus.Subscribe("docente.registro.pendiente", func(e eventbus.Evento) {
		payload, ok := e.Payload.(map[string]string)
		if !ok {
			log.Printf("correo: payload inesperado para docente.registro.pendiente: %+v", e.Payload)
			return
		}
		asunto, cuerpo := m.textoDeDocentePendiente(payload["nombre"], payload["apellido"], payload["email"])
		enviar("por mail el registro pendiente", func(ctx context.Context) error {
			return m.enviarATodosLosAdmins(ctx, asunto, cuerpo)
		})
	})

	// ── Aviso a los Admin: licencias por vencer (RF-05.9) ───────────
	//
	// El único correo del sistema que no lo dispara una persona sino el
	// reloj. Por eso importa más que los otros que no se repita: nadie
	// tiene el contexto de "yo hice algo, este mail es por eso", así que un
	// duplicado diario se lee como que el sistema está roto. La
	// idempotencia la garantizan las marcas de cada licencia, del lado de
	// inventory.
	bus.Subscribe("licencia.por-vencer", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.AvisoDeLicencias)
		if !ok {
			log.Printf("correo: payload inesperado para licencia.por-vencer: %+v", e.Payload)
			return
		}
		if payload.Total() == 0 {
			return
		}
		asunto, cuerpo := m.textoDeLicencias(payload)
		enviar("por mail las licencias por vencer", func(ctx context.Context) error {
			return m.enviarATodosLosAdmins(ctx, asunto, cuerpo)
		})
	})

	// ── El barrido de reservas y entregas (RF-08.10 a RF-08.13) ─────

	bus.Subscribe("reserva.recordatorio", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.RecordatorioDeReserva)
		if !ok {
			log.Printf("correo: payload inesperado para reserva.recordatorio: %+v", e.Payload)
			return
		}
		if payload.Email == "" {
			return
		}
		asunto, cuerpo := m.textoDeRecordatorio(payload)
		enviar("por mail el recordatorio de la reserva", func(ctx context.Context) error {
			return m.enviador.Enviar(ctx, payload.Email, asunto, cuerpo)
		})
	})

	bus.Subscribe("reserva.equipo-no-disponible", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.EquipoNoDisponibleParaReserva)
		if !ok {
			log.Printf("correo: payload inesperado para reserva.equipo-no-disponible: %+v", e.Payload)
			return
		}
		if payload.Email == "" {
			return
		}
		asunto, cuerpo := m.textoDeEquipoNoDisponible(payload)
		enviar("por mail la PC que no volvió", func(ctx context.Context) error {
			return m.enviador.Enviar(ctx, payload.Email, asunto, cuerpo)
		})
	})

	bus.Subscribe("reserva.no-retirada", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.ReservasLiberadas)
		if !ok {
			log.Printf("correo: payload inesperado para reserva.no-retirada: %+v", e.Payload)
			return
		}
		if payload.Email == "" {
			return
		}
		asunto, cuerpo := m.textoDeReservasLiberadas(payload)
		enviar("por mail la reserva liberada", func(ctx context.Context) error {
			return m.enviador.Enviar(ctx, payload.Email, asunto, cuerpo)
		})
	})

	// El reclamo va a dos lados: a los Admin con la lista completa, y a cada
	// persona que tenga cuenta con el suyo. Quien se llevó una máquina para
	// un trámite y no tiene cuenta no recibe nada — no hay a dónde
	// mandárselo, y por eso el aviso a los Admin es el que no puede fallar.
	bus.Subscribe("prestamo.demorado", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.PrestamosDemorados)
		if !ok {
			log.Printf("correo: payload inesperado para prestamo.demorado: %+v", e.Payload)
			return
		}
		if len(payload.Prestamos) == 0 {
			return
		}
		asunto, cuerpo := m.textoDeDemoraParaAdmins(payload)
		enviar("por mail las devoluciones demoradas", func(ctx context.Context) error {
			return m.enviarATodosLosAdmins(ctx, asunto, cuerpo)
		})

		for _, p := range payload.Prestamos {
			if p.Email == "" {
				continue
			}
			demorado := p
			asunto, cuerpo := m.textoDeDemoraParaQuienLaTiene(demorado)
			enviar("por mail el recordatorio de devolución", func(ctx context.Context) error {
				return m.enviador.Enviar(ctx, demorado.Email, asunto, cuerpo)
			})
		}
	})

	bus.Subscribe("prestamo.sin-devolver.cierre", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.EquiposSinDevolverAlCierre)
		if !ok {
			log.Printf("correo: payload inesperado para prestamo.sin-devolver.cierre: %+v", e.Payload)
			return
		}
		if len(payload.Equipos) == 0 {
			return
		}
		asunto, cuerpo := m.textoDeCierreParaAdmins(payload)
		enviar("por mail el cierre de jornada", func(ctx context.Context) error {
			return m.enviarATodosLosAdmins(ctx, asunto, cuerpo)
		})

		for _, pc := range payload.Equipos {
			if pc.ProximoEmail == "" {
				continue
			}
			aviso := pc
			asunto, cuerpo := m.textoDeCierreParaElProximo(aviso)
			enviar("por mail la PC que le va a faltar al docente siguiente", func(ctx context.Context) error {
				return m.enviador.Enviar(ctx, aviso.ProximoEmail, asunto, cuerpo)
			})
		}
	})

	// ── Aviso a la persona: le aprobaron la cuenta ──────────────────
	bus.Subscribe("cuenta.aprobada", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.CuentaAprobada)
		if !ok {
			log.Printf("correo: payload inesperado para cuenta.aprobada: %+v", e.Payload)
			return
		}
		asunto, cuerpo := m.textoDeCuentaAprobada(payload.Nombre)
		enviar("por mail la cuenta aprobada", func(ctx context.Context) error {
			return m.enviador.Enviar(ctx, payload.Email, asunto, cuerpo)
		})
	})

	// ── Código de recuperación de contraseña ────────────────────────
	bus.Subscribe("password.recuperacion.solicitada", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.DatosDeRecuperacion)
		if !ok {
			// Único handler que NO loguea el payload: si el tipo no es el
			// esperado igual puede contener el código en claro, y un %+v lo
			// dejaría escrito en los logs del contenedor.
			log.Printf("correo: payload inesperado para password.recuperacion.solicitada (tipo %T)", e.Payload)
			return
		}
		asunto, cuerpo := m.textoDeCodigoDeRecuperacion(payload.Nombre, payload.Codigo, payload.MinutosDeVigencia)
		enviar("por mail el código de recuperación", func(ctx context.Context) error {
			return m.enviador.Enviar(ctx, payload.Email, asunto, cuerpo)
		})
	})

	// ── Pidieron recuperar una cuenta que entra con Google ──────────
	bus.Subscribe("password.recuperacion.cuenta-google", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.CuentaSoloConGoogle)
		if !ok {
			log.Printf("correo: payload inesperado para password.recuperacion.cuenta-google: %+v", e.Payload)
			return
		}
		asunto, cuerpo := m.textoDeCuentaSoloConGoogle(payload.Nombre)
		enviar("por mail el aviso de cuenta con Google", func(ctx context.Context) error {
			return m.enviador.Enviar(ctx, payload.Email, asunto, cuerpo)
		})
	})
}

// enviarATodosLosAdmins manda el mismo mensaje a cada Admin APROBADA.
//
// Un envío que falla no corta los demás: que la casilla de uno rebote no es
// razón para que los otros tres se queden sin enterarse. Los errores se
// juntan y se devuelven al final, con el detalle de a quién no se pudo.
func (m *Mensajero) enviarATodosLosAdmins(ctx context.Context, asunto, cuerpo string) error {
	destinatarios, err := m.admins.EmailsDeAdminsAprobados(ctx)
	if err != nil {
		return fmt.Errorf("listando emails de admins: %w", err)
	}

	var fallidos []string
	for _, para := range destinatarios {
		if err := m.enviador.Enviar(ctx, para, asunto, cuerpo); err != nil {
			fallidos = append(fallidos, fmt.Sprintf("%s (%v)", para, err))
		}
	}
	if len(fallidos) > 0 {
		return fmt.Errorf("no se pudo enviar a %d de %d admins: %s",
			len(fallidos), len(destinatarios), strings.Join(fallidos, "; "))
	}
	return nil
}

// ══════════════════════════════════════════════════════════════════
// Los textos
// ══════════════════════════════════════════════════════════════════
//
// Todos son texto plano, cortos y sin nada que haya que apretar. Están
// juntos y separados del envío para poder leerlos de corrido: el tono de lo
// que le llega a un docente es una decisión del sistema, no un detalle
// desperdigado entre handlers.

// firma cierra todos los mensajes: dice de dónde salió el mail y que del
// otro lado no hay nadie leyendo.
const firma = "\n\n--\nSistema de Gestión de Reservas de Carros (SGRC)\n" +
	"Este mensaje se generó automáticamente; no hace falta responderlo."

// saludo tolera que el nombre venga vacío: sale de una fila, y un "Hola :"
// colgado se ve mal.
func saludo(nombre string) string {
	if n := strings.TrimSpace(nombre); n != "" {
		return fmt.Sprintf("Hola %s:\n\n", n)
	}
	return "Hola:\n\n"
}

// enlace devuelve la línea con la dirección del sistema, o nada si el
// despliegue no la tiene configurada.
func (m *Mensajero) enlace(prefijo string) string {
	if m.urlDelSistema == "" {
		return ""
	}
	return fmt.Sprintf("\n\n%s\n%s", prefijo, m.urlDelSistema)
}

func (m *Mensajero) textoDeDocentePendiente(nombre, apellido, email string) (asunto, cuerpo string) {
	quien := strings.TrimSpace(nombre + " " + apellido)
	if quien == "" {
		quien = email
	}

	asunto = "Hay una cuenta esperando aprobación"
	cuerpo = fmt.Sprintf(
		"%s se registró en el sistema de reservas y su cuenta está esperando "+
			"que un administrador la apruebe o la rechace.\n\nEmail declarado: %s",
		quien, email)
	cuerpo += m.enlace("Podés resolverlo desde:")
	cuerpo += firma
	return asunto, cuerpo
}

func (m *Mensajero) textoDeCuentaAprobada(nombre string) (asunto, cuerpo string) {
	asunto = "Ya podés entrar al sistema de reservas"
	// No se nombra "tu contraseña": la cuenta puede haberse creado con
	// Google y no tener ninguna. "Los datos con los que te registraste" es
	// cierto en los dos casos.
	cuerpo = saludo(nombre) +
		"Un administrador aprobó tu cuenta. Ya podés ingresar con los datos con " +
		"los que te registraste y empezar a reservar."
	cuerpo += m.enlace("Entrá desde:")
	cuerpo += firma
	return asunto, cuerpo
}

func (m *Mensajero) textoDeCodigoDeRecuperacion(nombre, codigo string, minutos int) (asunto, cuerpo string) {
	asunto = "Tu código para restablecer la contraseña"
	// El código va solo en su renglón y con espacios alrededor: es lo que
	// se va a seleccionar y copiar desde el celular, y pegado a una frase
	// se arrastra media oración con él.
	cuerpo = saludo(nombre) +
		"Pediste restablecer la contraseña de tu cuenta. Este es tu código:\n\n" +
		"    " + codigo + "\n\n" +
		fmt.Sprintf("Vence en %d minutos y sirve una sola vez.", minutos) +
		"\n\nSi no lo pediste vos, no hace falta que hagas nada: tu contraseña " +
		"actual sigue funcionando y este código no le sirve a nadie más que a " +
		"quien pueda leer este mensaje. Si te llega varias veces sin que lo " +
		"hayas pedido, avisale a un administrador."
	// ÚNICO mail sin enlace al sistema, y es el único que contiene una
	// credencial: "código + botón para entrar" es exactamente la forma de un
	// phishing, y acostumbrar a los docentes a apretar ese link es
	// entrenarlos para el día que llegue uno falso. Quien pidió el código ya
	// está en la pantalla que lo pide.
	cuerpo += firma
	return asunto, cuerpo
}

func (m *Mensajero) textoDeCuentaSoloConGoogle(nombre string) (asunto, cuerpo string) {
	asunto = "Tu cuenta ingresa con Google"
	// Existe para que el resultado no sea el silencio: sin él, quien tiene
	// una cuenta de Google pide el código, no le llega nada, y no puede
	// saber si el sistema falló o si escribió mal la dirección.
	cuerpo = saludo(nombre) +
		"Pediste restablecer la contraseña de tu cuenta, pero esa cuenta no tiene " +
		"contraseña propia: entrás con el botón \"Continuar con Google\" de la " +
		"pantalla de ingreso, y es Google quien verifica que seas vos.\n\n" +
		"Por eso no hay ninguna contraseña que restablecer desde acá. Si perdiste " +
		"el acceso a tu cuenta de Google, tenés que recuperarla desde Google."
	cuerpo += m.enlace("La pantalla de ingreso es:")
	cuerpo += firma
	return asunto, cuerpo
}
