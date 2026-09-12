package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/notification/application"
	"github.com/ramiro/sgrc/internal/notification/domain"
)

func mapearError(err error) error {
	switch {
	case errors.Is(err, application.ErrNotificacionNoEncontrada):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	// 403 y no 404: el aviso existe, lo que pasa es que no es de quien pregunta.
	// 409: el aviso existe y es tuyo, pero todavía no lo leíste.
	case errors.Is(err, application.ErrAvisoSinLeer):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.Is(err, application.ErrNoEsTuAviso):
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	case errors.Is(err, domain.ErrYaLeida):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrEstadoInvalido), errors.Is(err, application.ErrIDInvalido),
		errors.Is(err, domain.ErrCategoriaEmailInvalida):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
