package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta el buzón bajo /api/sugerencias.
//
// Escribir es para cualquier usuario autenticado —docente o Admin—: un
// Admin nuevo también se topa con cosas que no entiende, y obligarlo a
// pedirle a un docente que las reporte sería absurdo.
//
// El rate limit está puesto en escribir y no en leer: es el único que crea
// filas y manda un aviso a todos los Admin, así que es por donde alguien
// —o un botón que quedó apretado— podría llenar la casilla de correo de
// todo el equipo. Cinco por minuto es holgado para una persona escribiendo
// de verdad.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	sugerencias := app.Group("/api/sugerencias")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	sugerencias.Post("/", autenticado, middleware.RateLimit(5, time.Minute), h.Escribir)
	sugerencias.Get("/mias", autenticado, h.ListarPropias)

	sugerencias.Get("/", autenticado, soloAdmin, h.Listar)
	sugerencias.Post("/:id/responder", autenticado, soloAdmin, h.Responder)
}
