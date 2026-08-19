package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
)

// mapearError traduce cada error de negocio de application/ y domain/ a su
// código HTTP según docs/08-api-spec.yaml.
func mapearError(err error) error {
	switch {
	case errors.Is(err, application.ErrCicloNoEncontrado),
		errors.Is(err, application.ErrCursoNoEncontrado),
		errors.Is(err, application.ErrMateriaNoEncontrada),
		errors.Is(err, application.ErrDocenteMateriaNoEncontrado),
		errors.Is(err, domain.ErrPedidoNoExiste):
		return fiber.NewError(fiber.StatusNotFound, err.Error())

	case errors.Is(err, application.ErrYaHayCicloActivo),
		errors.Is(err, application.ErrCicloYaTieneAnio),
		errors.Is(err, application.ErrCursoNombreDuplicado),
		errors.Is(err, application.ErrCursoConReservas),
		errors.Is(err, application.ErrMateriaNombreDuplicado),
		errors.Is(err, application.ErrMateriaConReservas),
		errors.Is(err, application.ErrUsuarioNoValidoParaAsignar),
		errors.Is(err, application.ErrYaDictaLaMateria),
		errors.Is(err, application.ErrPedidoDuplicado),
		errors.Is(err, domain.ErrPedidoResuelto),
		errors.Is(err, domain.ErrCicloYaArchivado):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrAnioInvalido),
		errors.Is(err, domain.ErrNombreCursoInvalido),
		errors.Is(err, domain.ErrNombreMateriaVacio),
		errors.Is(err, domain.ErrRolDocenteInvalido),
		errors.Is(err, application.ErrIDInvalido),
		errors.Is(err, application.ErrFaltaCursoParaMateriaNueva),
		errors.Is(err, domain.ErrPedidoSinMateria),
		errors.Is(err, domain.ErrPedidoDobleForma),
		errors.Is(err, domain.ErrMotivoVacio),
		errors.Is(err, domain.ErrMotivoLargo),
		errors.Is(err, domain.ErrRespuestaLarga),
		errors.Is(err, domain.ErrRechazoSinMotivo),
		errors.Is(err, application.ErrReferenciaInexistente):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
