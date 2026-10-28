package http

import (
	"strings"

	"github.com/agustin-y2k/sgrc/internal/shared/audit"
	"github.com/gofiber/fiber/v2"
)

func (h *Handler) RenumerarEquipo(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}
	var req struct {
		IdentificadorActual int    `json:"identificadorActual"`
		Identificador       int    `json:"identificador"`
		Motivo              string `json:"motivo"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}
	id := c.Params("id")
	cambios, err := h.svc.RenumerarEquipo(c.UserContext(), id, req.IdentificadorActual, req.Identificador, req.Motivo)
	if err != nil {
		return mapearError(err)
	}
	if len(cambios) > 0 {
		detalle := detalleDeCambios(cambios)
		detalle["motivo"] = strings.TrimSpace(req.Motivo)
		h.auditar(c, claims.UserID, audit.EquipoEditado, "pc", &id, detalle)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
