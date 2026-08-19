package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta el buzón bajo /api/sugerencias.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	sugerencias := app.Group("/api/sugerencias")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	sugerencias.Post("/", autenticado, middleware.RateLimit(5, time.Minute), h.Escribir)
	sugerencias.Get("/mias", autenticado, h.ListarPropias)

	sugerencias.Get("/", autenticado, soloAdmin, h.Listar)
	sugerencias.Post("/:id/responder", autenticado, soloAdmin, h.Responder)
}
