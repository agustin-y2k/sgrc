package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta las rutas de availability bajo /api/availability
// (RF-07, ver docs/08-api-spec.yaml). GET /admins es para cualquier
// usuario autenticado — el resto son exclusivas de ADMIN sobre SU PROPIO
// horario; la titularidad de un bloque puntual se resuelve en
// application.Repo (acotada por usuario_id), no acá.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	availability := app.Group("/api/availability")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	availability.Get("/admins", autenticado, h.DisponibilidadDeAdmins)

	availability.Get("/mi-horario", autenticado, soloAdmin, h.MiHorario)
	availability.Post("/mi-horario", autenticado, soloAdmin, h.AgregarBloque)
	availability.Patch("/mi-horario/:id", autenticado, soloAdmin, h.EditarBloque)
	availability.Delete("/mi-horario/:id", autenticado, soloAdmin, h.EliminarBloque)

	availability.Post("/mi-excepcion", autenticado, soloAdmin, h.CargarExcepcion)
	availability.Post("/no-disponible-ahora", autenticado, soloAdmin, h.MarcarNoDisponibleAhora)
}
