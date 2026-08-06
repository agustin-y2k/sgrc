// Package email manda correo por SMTP. Vive en shared/ y no dentro de un
// paquete de dominio porque lo usan dos (auth y notification), mismo
// criterio que shared/security con argon2.
//
// Todo el correo del sistema es texto plano: son mensajes de cuatro líneas,
// y el HTML agregaría multipart/alternative, plantillas y una forma nueva
// de verse mal en el cliente de cada docente sin decir nada más.
package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Enviador es lo que ve el resto del sistema. Los paquetes que lo usan
// declaran su propia interfaz con esta misma firma (la interfaz la define
// quien consume), así que ninguno importa este paquete fuera de cmd/main.go.
type Enviador interface {
	// Enviar manda un mensaje de texto plano. El error dice que no se pudo
	// entregar al servidor SMTP; que el destinatario lo reciba ya no
	// depende de nosotros.
	Enviar(ctx context.Context, para, asunto, cuerpo string) error
}

// ErrSinDestinatario cubre el mensaje armado a partir de una fila
// incompleta: el servidor SMTP daría un error mucho menos claro.
var ErrSinDestinatario = errors.New("email: falta el destinatario")

// ══════════════════════════════════════════════════════════════════
// Enviador deshabilitado
// ══════════════════════════════════════════════════════════════════

// Deshabilitado es lo que se usa cuando el despliegue no configuró SMTP.
// No es un error: el correo es opcional y los avisos internos (la campana
// de notificaciones) llegan igual.
//
// Loguea en vez de devolver error para que el modo sea visible: si alguien
// reporta "no me llega el mail", el log lo explica.
type Deshabilitado struct{}

var _ Enviador = Deshabilitado{}

func (Deshabilitado) Enviar(_ context.Context, para, asunto, _ string) error {
	log.Printf("email: SMTP no configurado, no se envía %q a %s", asunto, para)
	return nil
}

// ══════════════════════════════════════════════════════════════════
// Enviador SMTP
// ══════════════════════════════════════════════════════════════════

// timeoutSMTP acota la conversación entera, no cada paso. Sin él, un
// servidor que acepta la conexión TCP y después no contesta deja la
// goroutine colgada para siempre — y el apagado ordenado las espera.
const timeoutSMTP = 20 * time.Second

// Config es lo que hace falta para hablar con el servidor SMTP.
type Config struct {
	Host string
	// Puerto como string: es lo que sale del entorno y lo único que se hace
	// con él es concatenarlo al host.
	Puerto   string
	Usuario  string
	Password string
	// Desde es la dirección del From. Con Gmail tiene que ser la misma
	// cuenta que autentica (o un alias verificado); si no coincide, Google
	// reescribe el From y el mensaje sale con otra dirección.
	Desde string
	// NombreDeQuienEnvia es lo que ve el destinatario en su lista de
	// correos. Opcional.
	NombreDeQuienEnvia string
}

// EnviadorSMTP implementa Enviador contra un servidor real.
type EnviadorSMTP struct {
	cfg Config
	// ahora se inyecta para poder fijar la fecha en los tests.
	ahora func() time.Time
}

var _ Enviador = (*EnviadorSMTP)(nil)

func NewEnviadorSMTP(cfg Config, ahora func() time.Time) *EnviadorSMTP {
	if ahora == nil {
		ahora = time.Now
	}
	return &EnviadorSMTP{cfg: cfg, ahora: ahora}
}

func (e *EnviadorSMTP) Enviar(ctx context.Context, para, asunto, cuerpo string) error {
	para = strings.TrimSpace(para)
	if para == "" {
		return ErrSinDestinatario
	}

	mensaje := e.armarMensaje(para, asunto, cuerpo)

	ctx, cancelar := context.WithTimeout(ctx, timeoutSMTP)
	defer cancelar()

	direccion := net.JoinHostPort(e.cfg.Host, e.cfg.Puerto)
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", direccion)
	if err != nil {
		return fmt.Errorf("conectando a %s: %w", direccion, err)
	}
	// net/smtp no conoce contextos: sin trasladar el deadline al socket, el
	// timeout de arriba solo cubriría el dial.
	if plazo, hay := ctx.Deadline(); hay {
		_ = conn.SetDeadline(plazo)
	}

	cliente, err := smtp.NewClient(conn, e.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("saludo SMTP con %s: %w", e.cfg.Host, err)
	}
	// Corta la conexión si algo falla en el medio; en el camino feliz Quit
	// ya la cerró y este Close es un no-op.
	defer func() { _ = cliente.Close() }()

	// STARTTLS es obligatorio, no "si el servidor lo ofrece": por esta
	// conexión van la contraseña de la cuenta y, en los mails de
	// recuperación, un código que sirve para entrar al sistema.
	if soportado, _ := cliente.Extension("STARTTLS"); !soportado {
		return fmt.Errorf("el servidor %s no ofrece STARTTLS: no se manda nada en claro", e.cfg.Host)
	}
	if err := cliente.StartTLS(&tls.Config{ServerName: e.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("negociando TLS con %s: %w", e.cfg.Host, err)
	}

	if e.cfg.Usuario != "" {
		auth := smtp.PlainAuth("", e.cfg.Usuario, e.cfg.Password, e.cfg.Host)
		if err := cliente.Auth(auth); err != nil {
			return fmt.Errorf("autenticando contra %s: %w", e.cfg.Host, err)
		}
	}

	if err := cliente.Mail(e.cfg.Desde); err != nil {
		return fmt.Errorf("remitente %s rechazado: %w", e.cfg.Desde, err)
	}
	if err := cliente.Rcpt(para); err != nil {
		return fmt.Errorf("destinatario rechazado: %w", err)
	}

	w, err := cliente.Data()
	if err != nil {
		return fmt.Errorf("abriendo el cuerpo del mensaje: %w", err)
	}
	if _, err := w.Write(mensaje); err != nil {
		_ = w.Close()
		return fmt.Errorf("escribiendo el cuerpo del mensaje: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("cerrando el cuerpo del mensaje: %w", err)
	}

	return cliente.Quit()
}

// armarMensaje arma cabeceras y cuerpo según el RFC 5322.
//
// Los acentos obligan a las dos codificaciones. Las cabeceras solo admiten
// ASCII, así que el asunto va en RFC 2047 (=?utf-8?q?...?=); sin eso
// "Restablecé tu contraseña" llega roto en varios clientes. El cuerpo va en
// quoted-printable, que queda legible incluso crudo y no depende de que el
// servidor acepte 8BITMIME.
func (e *EnviadorSMTP) armarMensaje(para, asunto, cuerpo string) []byte {
	remitente := e.cfg.Desde
	if nombre := strings.TrimSpace(e.cfg.NombreDeQuienEnvia); nombre != "" {
		// El nombre de la escuela puede tener acentos y va en la misma
		// cabecera, así que también se codifica.
		remitente = fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", nombre), e.cfg.Desde)
	}

	var sb strings.Builder
	escribirCabecera := func(clave, valor string) {
		// CRLF y no \n: lo exige el RFC 5321, y hay servidores que cortan la
		// sesión sin explicar por qué si llega solo LF.
		fmt.Fprintf(&sb, "%s: %s\r\n", clave, valor)
	}

	escribirCabecera("From", remitente)
	escribirCabecera("To", para)
	escribirCabecera("Subject", mime.QEncoding.Encode("utf-8", asunto))
	escribirCabecera("Date", e.ahora().Format(time.RFC1123Z))
	escribirCabecera("MIME-Version", "1.0")
	escribirCabecera("Content-Type", `text/plain; charset="utf-8"`)
	escribirCabecera("Content-Transfer-Encoding", "quoted-printable")
	// Ninguno de estos correos espera respuesta: la casilla de la escuela no
	// la lee nadie. Evita además que un autorespondedor del otro lado
	// conteste y genere un rebote.
	escribirCabecera("Auto-Submitted", "auto-generated")
	sb.WriteString("\r\n")

	qp := quotedprintable.NewWriter(&sb)
	// Escribe sobre un strings.Builder, que no falla nunca.
	_, _ = qp.Write([]byte(normalizarSaltos(cuerpo)))
	_ = qp.Close()

	return []byte(sb.String())
}

// normalizarSaltos pasa el cuerpo a CRLF: los textos del código se escriben
// con \n comunes y mandarlos así deja el mensaje con saltos mezclados.
func normalizarSaltos(texto string) string {
	return strings.ReplaceAll(strings.ReplaceAll(texto, "\r\n", "\n"), "\n", "\r\n")
}
