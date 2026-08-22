package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta las rutas de availability bajo /api/availability
// (RF-07, ver docs/08-api-spec.yaml).
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

	// La jornada de la institución cuelga de /api/jornada y no de
	// /api/availability, aunque la sirva este mismo handler: es un dato de la
	// escuela, no la disponibilidad de una persona, y la URL es lo primero que
	// lee quien intenta entender la API. El GET es para cualquier autenticado,
	// no solo Admin: el formulario de reserva lo usa para avisar antes de
	// mandar, y el calendario para saber qué días dibujar.
	//
	// El PUT reemplaza la jornada entera —no hay endpoints por tramo— porque
	// es una sola decisión de siete días: así se valida como conjunto y hay un
	// único momento en el que confirmarla.
	jornada := app.Group("/api/jornada")
	jornada.Get("/", autenticado, h.Jornada)
	jornada.Put("/", autenticado, soloAdmin, h.ReemplazarJornada)
}
