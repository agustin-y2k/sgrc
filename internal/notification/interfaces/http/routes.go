package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta las rutas de notification bajo /api/notifications.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	notifications := app.Group("/api/notifications")

	autenticado := aut.Requerida()

	notifications.Get("/", autenticado, h.ListarPropias)
	notifications.Patch("/:id/leida", autenticado, h.MarcarLeida)
	notifications.Post("/leer-todas", autenticado, h.MarcarTodasLeidas)

	// El literal va ANTES de la ruta con parámetro: al revés, "leidas" se
	// resolvería como el id de un aviso.
	notifications.Delete("/leidas", autenticado, h.BorrarNotificacionesLeidas)
	notifications.Delete("/:id", autenticado, h.BorrarNotificacion)

	// Cualquiera autenticado: un docente elige sobre sus correos personales y
	// un Admin además sobre los que van a todos los Admin. Qué casillas ve
	// cada uno lo resuelve el handler con el rol del token, no la ruta.
	notifications.Get("/preferencias-email", autenticado, h.ListarPreferenciasEmail)
	notifications.Put("/preferencias-email", autenticado, h.GuardarPreferenciasEmail)

	// El GET de un aviso solo va AL FINAL, después de todos los literales:
	// "/preferencias-email" está en la misma posición que el id y una ruta con
	// parámetro registrada antes se lo comería. Es el mismo cuidado que
	// "/leidas" arriba, y por eso los dos tienen su test.
	notifications.Get("/:id", autenticado, h.ObtenerNotificacion)
}
