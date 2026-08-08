package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/reservation/application"
)

// Entregas y devoluciones de PCs (RF-08) — el mostrador.
//
// Todo es solo Admin: quien entrega y recibe las máquinas es quien hoy
// escribe el papel. Que un docente pudiera marcarse la entrega a sí mismo
// desde el teléfono convertiría el registro en una declaración en vez de en
// una constancia, que es justo lo que hace confiable al papel.
//
// Ninguna de estas operaciones se anota en el audit_log, y no es un olvido:
// el propio préstamo guarda quién entregó, quién recibió y cuándo. Duplicar
// eso en el registro de auditoría sería llenarlo de movimientos de rutina
// —decenas por día— sin agregar un dato que no esté ya.

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
//
// Responde 200 aunque alguna PC no haya salido: el lote se procesó, y qué
// pasó con cada una está en el cuerpo. Un 409 obligaría a la pantalla a
// deshacer las que sí se entregaron, que ya están en manos del docente.
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
		ReservaIDs:        req.ReservaIDs,
		NombreAlternativo: req.NombreAlternativo,
		EntregadoPor:      claims.UserID,
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
		Motivo:             req.Motivo,
		DevolucionEstimada: req.DevolucionEstimada,
		EntregadoPor:       claims.UserID,
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
