package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/agustin-y2k/sgrc/internal/availability/application"
	"github.com/agustin-y2k/sgrc/internal/availability/domain"
)

func mapearError(err error) error {
	switch {
	case errors.Is(err, application.ErrBloqueNoEncontrado):
		return fiber.NewError(fiber.StatusNotFound, err.Error())

	// 409 y no 400: el bloque en sí está bien formado, lo que no se puede es
	// convivir con otro que ya existe.
	case errors.Is(err, domain.ErrBloqueSolapado),
		errors.Is(err, domain.ErrBloqueJornadaSolapado):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, application.ErrDemasiadosTramos),
		errors.Is(err, application.ErrIDInvalido),
		errors.Is(err, domain.ErrRangoHorarioInvalido),
		errors.Is(err, domain.ErrDiaSemanaInvalido),
		errors.Is(err, domain.ErrTipoExcepcionInvalido),
		errors.Is(err, domain.ErrExcepcionIncoherente):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	// Se conserva la causa del fallo de cascada; el cambio completo se revirtió.
	case errors.Is(err, application.ErrCascadaDeJornada):
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())

	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
