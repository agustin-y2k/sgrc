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

	// Cualquiera autenticado: un docente elige sobre sus correos personales y
	// un Admin además sobre los que van a todos los Admin. Qué casillas ve
	// cada uno lo resuelve el handler con el rol del token, no la ruta.
	notifications.Get("/preferencias-email", autenticado, h.ListarPreferenciasEmail)
	notifications.Put("/preferencias-email", autenticado, h.GuardarPreferenciasEmail)
}
