package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/agustin-y2k/sgrc/internal/reporting/application"
	"github.com/agustin-y2k/sgrc/internal/reporting/domain"
)

func mapearError(err error) error {
	switch {
	case errors.Is(err, domain.ErrValorNegativo):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, application.ErrIDInvalido):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, application.ErrRangoFechasInvalido), errors.Is(err, application.ErrHistoricoSinRango):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
