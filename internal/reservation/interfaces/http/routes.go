package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta todas las rutas de reservation bajo /api/reservation.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	reservation := app.Group("/api/reservation")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	reservation.Get("/reservas", autenticado, h.ListarReservas)
	reservation.Post("/reservas", autenticado, h.CrearReserva)
	reservation.Post("/reservas/recurrentes", autenticado, h.CrearReservaRecurrente)
	reservation.Post("/reservas/:id/cancelar", autenticado, h.CancelarReserva)
	// RF-08.14: cambiar una reserva de máquina sin partir la clase en dos
	// grupos.
	reservation.Patch("/reservas/:id/equipo", autenticado, h.CambiarEquipoDeReserva)
	reservation.Post("/grupos/:id/cancelar", autenticado, h.CancelarOcurrenciaRecurrente)
	reservation.Get("/grupos/:id", autenticado, h.ObtenerReservaGrupo)
	reservation.Post("/bloqueos", autenticado, soloAdmin, h.BloquearEquipos)

	// RF-04.4: el calendario de una PC lo puede ver cualquier usuario
	// autenticado.
	reservation.Get("/equipos/:equipoId/calendario", autenticado, h.CalendarioDeEquipo)

	// RF-04.2 y RF-04.11: las dos mitades de la franja — los equipos libres, de
	// los que el docente tilda los que necesita, y los que ya tiene alguien, con
	// quién los tiene.
	reservation.Get("/equipos-disponibles", autenticado, h.ListarEquiposDisponibles)

	// RF-04.12: pedirle al que lo tiene que lo libere.
	reservation.Post("/reservas/:id/pedido-de-liberacion", autenticado, h.PedirLiberacionDeReserva)

	// RF-08: entregas y devoluciones.
	reservation.Get("/prestamos", autenticado, soloAdmin, h.ListarPrestamosAbiertos)
	reservation.Post("/prestamos/por-reserva", autenticado, soloAdmin, h.EntregarPorReserva)
	reservation.Post("/prestamos/recibir", autenticado, soloAdmin, h.RecibirEquipos)
	reservation.Post("/prestamos", autenticado, soloAdmin, h.EntregarSuelta)
	reservation.Get("/equipos/:equipoId/prestamos", autenticado, soloAdmin, h.HistorialDePrestamosDeEquipo)
}
