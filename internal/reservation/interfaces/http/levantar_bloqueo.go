package http

import (
	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/shared/middleware"
	"github.com/gofiber/fiber/v2"
)

func (h *Handler) LevantarBloqueo(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}
	var req cancelarReservaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(400, "cuerpo de la petición inválido")
	}
	resultado, err := h.svc.LevantarBloqueo(c.UserContext(), application.LevantarBloqueoParams{
		OperacionID: c.Params("id"), SolicitanteID: claims.UserID, EsAdmin: claims.Rol == "ADMIN", Motivo: req.Motivo, IPOrigen: middleware.IPCliente(c),
	})
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(resultado)
}
