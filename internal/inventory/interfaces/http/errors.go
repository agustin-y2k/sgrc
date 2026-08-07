package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

func mapearError(err error) error {
	switch {
	case errors.Is(err, application.ErrCarroNoEncontrado),
		errors.Is(err, application.ErrPCNoEncontrada),
		errors.Is(err, application.ErrIncidenciaNoEncontrada):
		return fiber.NewError(fiber.StatusNotFound, err.Error())

	case errors.Is(err, application.ErrIdentificadorDuplicado),
		errors.Is(err, application.ErrNumeroSerieDuplicado):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrTransicionEstadoPCInvalida),
		errors.Is(err, domain.ErrPCYaDadaDeBaja):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrNombreCarroVacio),
		errors.Is(err, domain.ErrIdentificadorInvalido),
		errors.Is(err, domain.ErrNumeroSerieInvalido),
		errors.Is(err, domain.ErrNumeroSerieLargo),
		errors.Is(err, domain.ErrDescripcionVacia),
		errors.Is(err, domain.ErrEstadoPCInvalido),
		errors.Is(err, domain.ErrGravedadInvalida),
		errors.Is(err, domain.ErrEstadoIncidenciaInvalido),
		errors.Is(err, application.ErrIDInvalido),
		errors.Is(err, application.ErrReferenciaInexistente):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
