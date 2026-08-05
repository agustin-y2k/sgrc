package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// SecurityHeaders aplica los headers de seguridad exigidos por
// docs/09-seguridad-rbac.md §4: HSTS, nosniff, X-Frame-Options DENY y una
// CSP restrictiva.
//
// Esta CSP cubre solo las respuestas de /api, que son JSON — donde no hay
// nada que una CSP pueda restringir. La que hace el trabajo real es la de
// frontend/nginx-seguridad.conf, que acompaña al documento HTML: ahí está
// el hash del script inline del tema y el permiso para el botón de Google.
// Las dos son deliberadamente distintas y no hay que "unificarlas": si esta
// se aplicara también al HTML, o aquella a /api, quedarían dos headers
// Content-Security-Policy en la misma respuesta.
//
// helmet.New solo agrega HSTS cuando c.Protocol() == "https", pero en este
// despliegue Cloudflare Tunnel termina el TLS y le habla al contenedor por
// HTTP plano (ver docs/06-arquitectura.md §5-6) — el proceso nunca vería
// "https" y HSTS jamás saldría. Por eso se fuerza el header a mano.
func SecurityHeaders() fiber.Handler {
	h := helmet.New(helmet.Config{
		XFrameOptions:         "DENY",
		ContentTypeNosniff:    "nosniff",
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
	})

	return func(c *fiber.Ctx) error {
		c.Set("Strict-Transport-Security", "max-age=31536000; preload")
		return h(c)
	}
}

// CORS restringe el acceso al dominio del frontend, sin wildcard (ver
// docs/09-seguridad-rbac.md §4). allowedOrigin viene de una variable de
// entorno (FRONTEND_ORIGIN) — nunca hardcodeado ni "*".
func CORS(allowedOrigin string) fiber.Handler {
	return cors.New(cors.Config{
		AllowOrigins:     allowedOrigin,
		AllowMethods:     "GET,POST,PATCH,DELETE",
		AllowHeaders:     "Origin,Content-Type,Authorization",
		AllowCredentials: true,
	})
}

// IPCliente devuelve la IP del cliente para rate limiting y auditoría.
//
// No alcanza con c.IP(): con ProxyHeader configurado y el salto anterior
// dentro de TrustedProxies, Fiber devuelve el valor del header tal cual —
// y si el header no viene (un request que no pasó por Cloudflare, por
// ejemplo el puerto de desarrollo), eso es la cadena vacía. Sin este
// fallback, audit_log.ip_origen quedaba en NULL en todas esas filas.
func IPCliente(c *fiber.Ctx) string {
	if ip := strings.TrimSpace(c.IP()); ip != "" {
		return ip
	}
	return c.Context().RemoteIP().String()
}

// RateLimit limita a max requests por ventana desde una misma IP,
// devolviendo 429 al superarlo. Se usa en rutas públicas sensibles a fuerza
// bruta (login, registro — ver docs/09-seguridad-rbac.md §4).
//
// Depende de que fiber.Config tenga ProxyHeader configurado: detrás de
// Cloudflare Tunnel + nginx, c.IP() sin eso devuelve la IP del proxy y el
// límite pasa a ser un balde único para toda la institución (ver
// cmd/main.go).
func RateLimit(max int, expiration time.Duration) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          max,
		Expiration:   expiration,
		KeyGenerator: IPCliente,
		LimitReached: func(c *fiber.Ctx) error {
			return fiber.NewError(fiber.StatusTooManyRequests, "demasiados intentos, esperá antes de volver a probar")
		},
	})
}

// RateLimitPorEmail limita los intentos contra una misma cuenta, leyendo el
// email del cuerpo del request.
//
// Es el complemento necesario de RateLimit para el login. Limitar solo por
// IP falla en las dos direcciones: los docentes que entran desde el wifi de
// la escuela comparten una salida NAT y se consumen la cuota entre ellos,
// mientras que a alguien probando contraseñas contra una cuenta puntual le
// alcanza con rotar de red para esquivarlo. La cuenta atacada, en cambio,
// es la misma siempre.
func RateLimitPorEmail(max int, expiration time.Duration) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: expiration,
		KeyGenerator: func(c *fiber.Ctx) string {
			var cuerpo struct {
				Email string `json:"email"`
			}
			if err := c.BodyParser(&cuerpo); err != nil || cuerpo.Email == "" {
				// Sin email no hay cuenta que proteger; que lo resuelva el
				// límite por IP y el 400 del handler.
				return "sin-email:" + IPCliente(c)
			}
			return "email:" + strings.ToLower(strings.TrimSpace(cuerpo.Email))
		},
		LimitReached: func(c *fiber.Ctx) error {
			return fiber.NewError(fiber.StatusTooManyRequests,
				"demasiados intentos con esta cuenta, esperá un minuto antes de volver a probar")
		},
	})
}
