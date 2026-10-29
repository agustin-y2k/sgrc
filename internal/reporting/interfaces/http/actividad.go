package http

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/agustin-y2k/sgrc/internal/reporting/application"
	"github.com/agustin-y2k/sgrc/internal/reporting/domain"
)

type LectorActividad interface {
	CalcularActividad(context.Context, string, *time.Time, *time.Time) (domain.Actividad, error)
}

func (h *Handler) ConActividad(lector LectorActividad) *Handler { h.actividad = lector; return h }
func (h *Handler) Actividad(c *fiber.Ctx) error {
	desde, hasta, err := rangoDeQuery(c)
	if err != nil {
		return err
	}
	if desde != nil && hasta != nil && hasta.Before(*desde) {
		return mapearError(application.ErrRangoFechasInvalido)
	}
	if h.actividad == nil {
		return fiber.NewError(503, "no se pudo consultar la actividad")
	}
	a, err := h.actividad.CalcularActividad(c.UserContext(), c.Params("cicloId"), desde, hasta)
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(a)
}
