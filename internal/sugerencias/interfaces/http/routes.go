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
	// Las propias cuelgan de /api/mis-sugerencias y no de /sugerencias/mias:
	// "mías" no es una sugerencia con ese id, y todo lo propio del sistema usa
	// el mismo prefijo (ver /api/mi-perfil en auth).
	app.Group("/api").Get("/mis-sugerencias", autenticado, h.ListarPropias)

	// Escribir en un hilo NO es solo del Admin: quien preguntó también
	// contesta, y el servicio verifica que sea el suyo. El límite es el mismo
	// que para abrir uno nuevo.
	sugerencias.Post("/:id/mensajes", autenticado, middleware.RateLimit(5, time.Minute), h.Responder)

	sugerencias.Get("/", autenticado, soloAdmin, h.Listar)
	sugerencias.Post("/:id/resolver", autenticado, soloAdmin, h.Resolver)
}
