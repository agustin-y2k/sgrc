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
}
