package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/availability/application"
	"github.com/ramiro/sgrc/internal/availability/domain"
)

func mapearError(err error) error {
	switch {
	case errors.Is(err, application.ErrBloqueNoEncontrado):
		return fiber.NewError(fiber.StatusNotFound, err.Error())

	case errors.Is(err, application.ErrIDInvalido),
		errors.Is(err, domain.ErrRangoHorarioInvalido),
		errors.Is(err, domain.ErrDiaSemanaInvalido),
		errors.Is(err, domain.ErrTipoExcepcionInvalido),
		errors.Is(err, domain.ErrExcepcionIncoherente):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
