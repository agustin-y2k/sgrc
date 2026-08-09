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
	// RF-08.14: cambiar una reserva de máquina sin partir la clase en dos
	// grupos. La titularidad se verifica adentro del handler, como en
	// cancelar.
	reservation.Patch("/reservas/:id/equipo", autenticado, h.CambiarEquipoDeReserva)
	reservation.Post("/grupos/:id/cancelar", autenticado, h.CancelarOcurrenciaRecurrente)
	reservation.Get("/grupos/:id", autenticado, h.ObtenerReservaGrupo)
	reservation.Post("/bloqueos", autenticado, soloAdmin, h.BloquearEquipos)

	// RF-04.4: el calendario de una PC lo puede ver cualquier usuario
	// autenticado. Vive bajo /api/reservation aunque conceptualmente sea
	// "de la PC", porque el dato es de este paquete — inventory no puede
	// leer reservas sin romper el límite de dominio.
	reservation.Get("/equipos/:equipoId/calendario", autenticado, h.CalendarioDeEquipo)

	// RF-04.2: la lista de equipos libres en una franja, de la que el docente
	// tilda las que necesita.
	reservation.Get("/equipos-disponibles", autenticado, h.ListarEquiposDisponibles)

	// RF-08: entregas y devoluciones. Todo solo Admin — quien entrega y
	// recibe las máquinas es quien hoy escribe el papel. Que un docente
	// pudiera marcarse la entrega a sí mismo convertiría el registro en una
	// declaración en vez de en una constancia.
	//
	// /prestamos/por-reserva y /prestamos/recibir van antes que nada que
	// pueda parecerles un parámetro; hoy no hay ninguna ruta /prestamos/:id,
	// pero el orden deja el camino despejado si mañana la hay.
	reservation.Get("/prestamos", autenticado, soloAdmin, h.ListarPrestamosAbiertos)
	reservation.Post("/prestamos/por-reserva", autenticado, soloAdmin, h.EntregarPorReserva)
	reservation.Post("/prestamos/recibir", autenticado, soloAdmin, h.RecibirEquipos)
	reservation.Post("/prestamos", autenticado, soloAdmin, h.EntregarSuelta)
	reservation.Get("/equipos/:equipoId/prestamos", autenticado, soloAdmin, h.HistorialDePrestamosDeEquipo)
}
