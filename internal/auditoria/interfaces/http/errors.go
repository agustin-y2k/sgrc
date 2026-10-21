package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/agustin-y2k/sgrc/internal/auditoria/application"
	"github.com/agustin-y2k/sgrc/internal/auditoria/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/paginacion"
)

// mapearError traduce cada error de negocio a su código HTTP.
func mapearError(err error) error {
	switch {
	case errors.Is(err, domain.ErrRangoInvertido),
		errors.Is(err, domain.ErrRangoLargo),
		errors.Is(err, application.ErrIDInvalido),
		errors.Is(err, paginacion.ErrPaginaInvalida),
		errors.Is(err, paginacion.ErrTamanioInvalido):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
