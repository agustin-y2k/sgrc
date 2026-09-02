package application

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// Copias por correo de los avisos del sistema (RF-05.8).

// timeoutCorreo es más largo que timeoutNotificacion: aquello es un INSERT
// contra un Postgres de la misma red, esto es un saludo SMTP, un STARTTLS,
// una autenticación y una transferencia contra un servidor de Google.
const timeoutCorreo = 45 * time.Second

// Mensajero manda los correos.
type Mensajero struct {
	enviador     EnviadorDeEmail
	admins       ListadorAdmins
	preferencias PreferenciasEmail
	// urlDelSistema es FRONTEND_ORIGIN, la dirección pública desde la que se
	// entra.
	urlDelSistema string
}

func NewMensajero(enviador EnviadorDeEmail, admins ListadorAdmins, preferencias PreferenciasEmail, urlDelSistema string) *Mensajero {
	return &Mensajero{
		enviador:      enviador,
		admins:        admins,
		preferencias:  preferencias,
		urlDelSistema: strings.TrimRight(strings.TrimSpace(urlDelSistema), "/"),
	}
}

// ══════════════════════════════════════════════════════════════════ Registro
// de los suscriptores
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

	// ── Aviso a los Admin: alguien se registró ────────────────────── Mismo
	// evento que dispara el aviso interno (RF-05.6).
	bus.Subscribe("docente.registro.pendiente", func(e eventbus.Evento) {
		payload, ok := e.Payload.(map[string]string)
		if !ok {
			log.Printf("correo: payload inesperado para docente.registro.pendiente: %+v", e.Payload)
			return
		}
		asunto, cuerpo := m.textoDeDocentePendiente(payload["nombre"], payload["apellido"], payload["email"])
		enviar("por mail el registro pendiente", func(ctx context.Context) error {
			return m.enviarALosAdminsSuscriptos(ctx, domain.CatCuentaPendiente, asunto, cuerpo)
		})
	})

	// ── Aviso a los Admin: licencias por vencer (RF-05.9) ─────────── El único
	// correo del sistema que no lo dispara una persona sino el reloj.
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
			return m.enviarALosAdminsSuscriptos(ctx, domain.CatLicenciaPorVencer, asunto, cuerpo)
		})
	})

	// ── Le cancelaron computadoras de una reserva (RF-05.1/05.2/05.3) ──
	bus.Subscribe("reserva.cancelada", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.CancelacionesDeUsuario)
		if !ok {
			log.Printf("correo: payload inesperado para reserva.cancelada: %+v", e.Payload)
			return
		}
		if len(payload.Reservas) == 0 {
			return
		}
		asunto, cuerpo := m.textoDeCancelacion(payload)
		enviar("por mail la cancelación", func(ctx context.Context) error {
			return m.enviarSiLoQuiere(ctx, payload.Email, domain.CatReservaCancelada, asunto, cuerpo)
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
			return m.enviarSiLoQuiere(ctx, payload.Email, domain.CatRecordatorioDeReserva, asunto, cuerpo)
		})
	})

	// El pedido de un docente a otro (RF-04.12) es de los que más necesitan el
	// correo: la clase suele ser de esta semana, y un aviso que espera a que el
	// dueño entre al sistema llega después de la clase.
	bus.Subscribe("reserva.pedido-de-liberacion", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.PedidoDeLiberacion)
		if !ok {
			log.Printf("correo: payload inesperado para reserva.pedido-de-liberacion: %+v", e.Payload)
			return
		}
		if payload.Email == "" {
			return
		}
		asunto, cuerpo := m.textoDePedidoDeLiberacion(payload)
		enviar("por mail el pedido de liberación", func(ctx context.Context) error {
			return m.enviarSiLoQuiere(ctx, payload.Email, domain.CatPedidoDeLiberacion, asunto, cuerpo)
		})
	})

	// El buzón de soporte: lo que se escribe va a los Admin, y la respuesta a
	// quien escribió. Un pedido de AYUDA usa las categorías fijas, que no se
	// pueden desactivar; una sugerencia o un "algo no anda" usan las
	// optativas, porque pueden esperar a que alguien entre a mirar.
	bus.Subscribe("sugerencia.nueva", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.SugerenciaNueva)
		if !ok {
			log.Printf("correo: payload inesperado para sugerencia.nueva: %+v", e.Payload)
			return
		}
		asunto, cuerpo := m.textoDeSugerencia(payload)
		enviar("por mail una sugerencia", func(ctx context.Context) error {
			return m.enviarALosAdminsSuscriptos(ctx, categoriaDelBuzon(payload.Tipo), asunto, cuerpo)
		})
	})

	bus.Subscribe("sugerencia.seguimiento", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.SugerenciaSeguimiento)
		if !ok {
			log.Printf("correo: payload inesperado para sugerencia.seguimiento: %+v", e.Payload)
			return
		}
		asunto, cuerpo := m.textoDeSeguimiento(payload)
		enviar("por mail el seguimiento de una conversación", func(ctx context.Context) error {
			return m.enviarALosAdminsSuscriptos(ctx, categoriaDelBuzon(payload.Tipo), asunto, cuerpo)
		})
	})

	bus.Subscribe("sugerencia.respondida", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.SugerenciaRespondida)
		if !ok {
			log.Printf("correo: payload inesperado para sugerencia.respondida: %+v", e.Payload)
			return
		}
		if payload.Email == "" {
			return
		}
		asunto, cuerpo := m.textoDeRespuestaASugerencia(payload)
		enviar("por mail la respuesta a una sugerencia", func(ctx context.Context) error {
			return m.enviarSiLoQuiere(ctx, payload.Email, categoriaDeLaRespuesta(payload.Tipo), asunto, cuerpo)
		})
	})

	// Un pedido para dictar una materia va SOLO a los Admin, que son quienes
	// deciden. A quien ya la dicta no se le manda nada: no se le pedía ninguna
	// acción, y un correo que solo pone al tanto es el tipo de aviso que llena
	// la casilla sin cambiar lo que nadie hace.
	bus.Subscribe("materia.pedido.nuevo", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.PedidoDeMateriaNuevo)
		if !ok {
			log.Printf("correo: payload inesperado para materia.pedido.nuevo: %+v", e.Payload)
			return
		}
		asunto, cuerpo := m.textoDePedidoDeMateria(payload)
		enviar("por mail un pedido de materia", func(ctx context.Context) error {
			return m.enviarALosAdminsSuscriptos(ctx, domain.CatPedidoDeMateria, asunto, cuerpo)
		})
	})

	bus.Subscribe("materia.pedido.resuelto", func(e eventbus.Evento) {
		payload, ok := e.Payload.(eventbus.PedidoDeMateriaResuelto)
		if !ok {
			log.Printf("correo: payload inesperado para materia.pedido.resuelto: %+v", e.Payload)
			return
		}
		if payload.Email == "" {
			return
		}
		asunto, cuerpo := m.textoDePedidoResuelto(payload)
		enviar("por mail la resolución de un pedido de materia", func(ctx context.Context) error {
			return m.enviarSiLoQuiere(ctx, payload.Email, domain.CatPedidoDeMateriaResuelto, asunto, cuerpo)
		})
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
			return m.enviarALosAdminsSuscriptos(ctx, domain.CatCierreSinDevolver, asunto, cuerpo)
		})

		// Al docente de la próxima reserva no se le manda nada acá: ver el
		// mismo punto en subscribers.go. Su aviso sale una hora antes de la
		// clase, que es cuando todavía puede hacer algo.
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
			// Único handler que NO loguea el payload: si el tipo no es el esperado
			// igual puede contener el código en claro, y un %+v lo dejaría escrito en
			// los logs del contenedor.
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

// tipoAyuda es el valor que publica sugerencias en el evento. Va como
// literal y no importando su domain: ningún paquete importa el domain de otro
// (docs/06-arquitectura.md §1), y lo que cruza el bus son datos, no tipos.
const tipoAyuda = "AYUDA"

// categoriaDelBuzon elige con qué preferencia se filtra el correo a los Admin,
// según de qué se trate el hilo.
func categoriaDelBuzon(tipo string) domain.CategoriaEmail {
	if tipo == tipoAyuda {
		return domain.CatSoporte
	}
	return domain.CatSugerencia
}

// categoriaDeLaRespuesta es lo mismo para el correo que vuelve a quien
// escribió.
func categoriaDeLaRespuesta(tipo string) domain.CategoriaEmail {
	if tipo == tipoAyuda {
		return domain.CatSoporteRespondido
	}
	return domain.CatSugerenciaRespondida
}

// enviarSiLoQuiere manda un correo personal solo si esa persona no lo apagó
// (RF-05.13). Los tres correos de la cuenta —el código de recuperación, la
// cuenta aprobada, la cuenta con Google— NO pasan por acá y llaman derecho al
// enviador: no son copia de ningún aviso interno, y el del código es el único
// canal posible para alguien que justamente no puede entrar.
func (m *Mensajero) enviarSiLoQuiere(ctx context.Context, para string, categoria domain.CategoriaEmail, asunto, cuerpo string) error {
	if para == "" {
		return nil
	}
	quiere, err := m.preferencias.RecibePorEmail(ctx, para, categoria)
	if err != nil {
		return fmt.Errorf("consultando si %s quiere recibir %s: %w", para, categoria, err)
	}
	if !quiere {
		return nil
	}
	return m.enviador.Enviar(ctx, para, asunto, cuerpo)
}

// enviarALosAdminsSuscriptos manda el mismo mensaje a cada Admin APROBADA que
// haya pedido esa categoría (RF-05.13); si no la pidió nadie, no sale ningún
// mail y el aviso interno queda igual. Un envío que falla no corta los demás:
// que la casilla de uno rebote no es razón para que los otros tres se queden
// sin enterarse.
func (m *Mensajero) enviarALosAdminsSuscriptos(ctx context.Context, categoria domain.CategoriaEmail, asunto, cuerpo string) error {
	destinatarios, err := m.admins.EmailsDeAdminsSuscriptos(ctx, categoria)
	if err != nil {
		return fmt.Errorf("listando emails de admins suscriptos a %s: %w", categoria, err)
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

// ══════════════════════════════════════════════════════════════════ Los
// textos ══════════════════════════════════════════════════════════════════
// Todos son texto plano, cortos y sin nada que haya que apretar.

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
	// No se nombra "tu contraseña": la cuenta puede haberse creado con Google y
	// no tener ninguna.
	cuerpo = saludo(nombre) +
		"Un administrador aprobó tu cuenta. Ya podés ingresar con los datos con " +
		"los que te registraste y empezar a reservar."
	cuerpo += m.enlace("Entrá desde:")
	cuerpo += firma
	return asunto, cuerpo
}

func (m *Mensajero) textoDeCodigoDeRecuperacion(nombre, codigo string, minutos int) (asunto, cuerpo string) {
	asunto = "Tu código para restablecer la contraseña"
	// El código va solo en su renglón y con espacios alrededor: es lo que se va
	// a seleccionar y copiar desde el celular, y pegado a una frase se arrastra
	// media oración con él.
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
	// phishing, y acostumbrar a los docentes a apretar ese link es entrenarlos
	// para el día que llegue uno falso.
	cuerpo += firma
	return asunto, cuerpo
}

func (m *Mensajero) textoDeCuentaSoloConGoogle(nombre string) (asunto, cuerpo string) {
	asunto = "Tu cuenta ingresa con Google"
	// Existe para que el resultado no sea el silencio: sin él, quien tiene una
	// cuenta de Google pide el código, no le llega nada, y no puede saber si el
	// sistema falló o si escribió mal la dirección.
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
