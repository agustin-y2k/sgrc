package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/sugerencias/application"
	"github.com/ramiro/sgrc/internal/sugerencias/domain"
)

func mapearError(err error) error {
	switch {
	case errors.Is(err, domain.ErrSugerenciaNoExist):
		return fiber.NewError(fiber.StatusNotFound, err.Error())

	// 409: el mensaje está bien, lo que no se puede es cerrar dos veces lo
	// mismo — que es lo que pasa cuando dos Admin abren la lista juntos.
	case errors.Is(err, domain.ErrYaResuelta):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	// El hilo existe y no es suyo: 403 y no 404, porque esconderlo no le
	// oculta nada a quien ya tiene el id.
	case errors.Is(err, application.ErrNoEsTuya):
		return fiber.NewError(fiber.StatusForbidden, err.Error())

	case errors.Is(err, application.ErrIDInvalido),
		errors.Is(err, domain.ErrTextoVacio),
		errors.Is(err, domain.ErrTextoLargo),
		errors.Is(err, domain.ErrAsuntoVacio),
		errors.Is(err, domain.ErrAsuntoLargo),
		errors.Is(err, domain.ErrTipoInvalido),
		errors.Is(err, domain.ErrPantallaLarga):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
