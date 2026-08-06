package email

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// puertoPorDefecto es el de SMTP con STARTTLS (submission, RFC 6409). Es el
// que usa Gmail y el que corresponde al único modo que soporta este
// paquete: conexión en claro que se cifra con STARTTLS antes de autenticar.
// El 465 (TLS implícito desde el saludo) NO funcionaría acá.
const puertoPorDefecto = "587"

// DesdeEntorno arma el Enviador que corresponda a la configuración.
//
// El correo es OPCIONAL, igual que el ingreso con Google: sin SMTP_HOST el
// sistema arranca, los avisos siguen llegando a la campana de
// notificaciones y lo único que se pierde son las copias por mail. La
// recuperación por autoservicio (RF-01.10) sí queda fuera de servicio, y
// por eso existe el reset asistido por un Admin (RF-01.6).
//
// Lo que NO se tolera es una configuración a medias: con SMTP_HOST puesto y
// SMTP_FROM vacío el proceso arrancaría y cada envío fallaría dentro de una
// goroutine cuyo error solo se loguea, que es el modo de falla que nadie
// mira. Mejor no arrancar.
func DesdeEntorno(getenv func(string) string, ahora func() time.Time) (Enviador, error) {
	leer := func(clave string) string { return strings.TrimSpace(getenv(clave)) }

	host := leer("SMTP_HOST")
	if host == "" {
		return Deshabilitado{}, nil
	}

	cfg := Config{
		Host:               host,
		Puerto:             leer("SMTP_PORT"),
		Usuario:            leer("SMTP_USER"),
		Password:           getenv("SMTP_PASSWORD"), // sin TrimSpace: un espacio puede ser parte de la contraseña
		Desde:              leer("SMTP_FROM"),
		NombreDeQuienEnvia: leer("SMTP_FROM_NAME"),
	}

	if cfg.Puerto == "" {
		cfg.Puerto = puertoPorDefecto
	}
	if n, err := strconv.Atoi(cfg.Puerto); err != nil || n <= 0 || n > 65535 {
		return nil, fmt.Errorf("SMTP_PORT (%q) no es un puerto válido", cfg.Puerto)
	}

	if cfg.Desde == "" {
		// Con Gmail el remitente tiene que ser la misma cuenta que
		// autentica, así que el default útil está a mano.
		if cfg.Usuario == "" {
			return nil, fmt.Errorf("SMTP_HOST está configurado pero falta SMTP_FROM: es la dirección desde la que salen los correos")
		}
		cfg.Desde = cfg.Usuario
	}

	// La contraseña de aplicación de Gmail se muestra en la consola de
	// Google en cuatro grupos de cuatro ("abcd efgh ijkl mnop") y se copia
	// con los espacios. Google los ignora, pero SMTP PLAIN no: la
	// autenticación falla con "Username and Password not accepted", que
	// manda a revisar el usuario y no los espacios que sobran.
	if esGmail(cfg.Host) {
		cfg.Password = strings.ReplaceAll(cfg.Password, " ", "")
	}

	if cfg.Usuario != "" && cfg.Password == "" {
		return nil, fmt.Errorf("SMTP_USER (%q) está configurado pero SMTP_PASSWORD está vacío", cfg.Usuario)
	}

	return NewEnviadorSMTP(cfg, ahora), nil
}

func esGmail(host string) bool {
	h := strings.ToLower(host)
	return h == "smtp.gmail.com" || strings.HasSuffix(h, ".gmail.com") || h == "smtp.googlemail.com"
}
