package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/reservation/application"
)

// Entregas y devoluciones de PCs (RF-08) — el mostrador.

// GET /api/reservation/prestamos — qué hay afuera ahora mismo.
func (h *Handler) ListarPrestamosAbiertos(c *fiber.Ctx) error {
	prestamos, err := h.svc.ListarPrestamosAbiertos(c.UserContext())
	if err != nil {
		return mapearError(err)
	}

	ahora := h.svc.Ahora()
	data := make([]prestamoResponse, len(prestamos))
	for i, p := range prestamos {
		data[i] = toPrestamoDetalladoResponse(p, ahora)
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/reservation/equipos/{equipoId}/prestamos — el historial de una máquina.
func (h *Handler) HistorialDePrestamosDeEquipo(c *fiber.Ctx) error {
	prestamos, err := h.svc.HistorialDeEquipo(c.UserContext(), c.Params("equipoId"))
	if err != nil {
		return mapearError(err)
	}

	ahora := h.svc.Ahora()
	data := make([]prestamoResponse, len(prestamos))
	for i, p := range prestamos {
		data[i] = toPrestamoDetalladoResponse(p, ahora)
	}
	return c.JSON(fiber.Map{"data": data})
}

// POST /api/reservation/prestamos/por-reserva — entregar las máquinas de una
// reserva.
func (h *Handler) EntregarPorReserva(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req entregarPorReservaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	resultado, err := h.svc.EntregarPorReserva(c.UserContext(), application.EntregaPorReservaParams{
		ReservaIDs:   req.ReservaIDs,
		RetiradoPor:  req.RetiradoPor,
		EntregadoPor: claims.UserID,
	})
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toResultadoEntregaResponse(resultado, h.svc.Ahora()))
}

// POST /api/reservation/prestamos — entrega espontánea, sin reserva detrás.
func (h *Handler) EntregarSuelta(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req entregarSueltaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	resultado, err := h.svc.EntregarSuelta(c.UserContext(), application.EntregaSueltaParams{
		EquipoIDs:          req.EquipoIDs,
		Nombre:             req.Nombre,
		UsuarioID:          req.UsuarioID,
		RetiradoPor:        req.RetiradoPor,
		Motivo:             req.Motivo,
		DevolucionEstimada: req.DevolucionEstimada,
		EntregadoPor:       claims.UserID,
		SalidaAReparacion:  req.SalidaAReparacion,
	})
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toResultadoEntregaResponse(resultado, h.svc.Ahora()))
}

// POST /api/reservation/prestamos/recibir — las máquinas volvieron.
func (h *Handler) RecibirEquipos(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req recibirRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	resultado, err := h.svc.RecibirEquipos(c.UserContext(), req.PrestamoIDs, claims.UserID, req.Observaciones)
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toResultadoDevolucionResponse(resultado, h.svc.Ahora()))
}
