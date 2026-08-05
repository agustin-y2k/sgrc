package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta todas las rutas de reservation bajo
// /api/reservation.
//
// La titularidad de una reserva (RF-04.4: un docente solo cancela las
// suyas) se verifica DENTRO de los handlers de cancelación, no acá — el
// middleware de rol solo distingue ADMIN de "cualquier autenticado", la
// regla más fina de "es tuya o sos Admin" necesita comparar contra el
// dato de la reserva en sí.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	reservation := app.Group("/api/reservation")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	reservation.Get("/reservas", autenticado, h.ListarReservas)
	reservation.Post("/reservas", autenticado, h.CrearReserva)
	reservation.Post("/reservas/recurrentes", autenticado, h.CrearReservaRecurrente)
	reservation.Post("/reservas/:id/cancelar", autenticado, h.CancelarReserva)
	reservation.Post("/grupos/:id/cancelar", autenticado, h.CancelarOcurrenciaRecurrente)
	reservation.Get("/grupos/:id", autenticado, h.ObtenerReservaGrupo)
	reservation.Post("/bloqueos-evaluacion", autenticado, soloAdmin, h.BloquearParaEvaluacion)

	// RF-04.4: el calendario de una PC lo puede ver cualquier usuario
	// autenticado. Vive bajo /api/reservation aunque conceptualmente sea
	// "de la PC", porque el dato es de este paquete — inventory no puede
	// leer reservas sin romper el límite de dominio.
	reservation.Get("/pcs/:pcId/calendario", autenticado, h.CalendarioDePC)

	// RF-04.2: la lista de PCs libres en una franja, de la que el docente
	// tilda las que necesita.
	reservation.Get("/pcs-disponibles", autenticado, h.ListarPCsDisponibles)
}
