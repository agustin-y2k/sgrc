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
// docs/09-seguridad-rbac.md §4: HSTS, nosniff, X-Frame-Options DENY y una CSP
// restrictiva.
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
// docs/09-seguridad-rbac.md §4).
func CORS(allowedOrigin string) fiber.Handler {
	return cors.New(cors.Config{
		AllowOrigins:     allowedOrigin,
		AllowMethods:     "GET,POST,PATCH,DELETE",
		AllowHeaders:     "Origin,Content-Type,Authorization",
		AllowCredentials: true,
	})
}

// IPCliente devuelve la IP del cliente para rate limiting y auditoría.
func IPCliente(c *fiber.Ctx) string {
	if ip := strings.TrimSpace(c.IP()); ip != "" {
		return ip
	}
	return c.Context().RemoteIP().String()
}

// RateLimit limita a max requests por ventana desde una misma IP, devolviendo
// 429 al superarlo.
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
