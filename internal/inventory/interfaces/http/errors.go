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
		errors.Is(err, application.ErrEquipoNoEncontrado),
		errors.Is(err, application.ErrIncidenciaNoEncontrada),
		errors.Is(err, application.ErrLicenciaNoEncontrada):
		return fiber.NewError(fiber.StatusNotFound, err.Error())

	case errors.Is(err, application.ErrIdentificadorDuplicado),
		errors.Is(err, application.ErrNumeroSerieDuplicado),
		// El alta masiva saltea los duplicados y los informa en el cuerpo;
		// esto solo salta al RENOMBRAR una licencia al nombre de otra que
		// esa misma PC ya tiene.
		errors.Is(err, application.ErrLicenciaDuplicada),
		errors.Is(err, application.ErrNombreDeEquipoDuplicado):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrTransicionEstadoEquipoInvalida),
		errors.Is(err, domain.ErrEquipoYaDadoDeBaja),
		// 409 y no 400: el pedido está bien formado, lo que pasa es que el
		// estado actual del equipo no lo admite.
		errors.Is(err, application.ErrEquipoPrestado):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrNombreCarroVacio),
		errors.Is(err, domain.ErrIdentificadorInvalido),
		errors.Is(err, domain.ErrNumeroSerieInvalido),
		errors.Is(err, domain.ErrNumeroSerieLargo),
		errors.Is(err, domain.ErrTipoEquipoVacio),
		errors.Is(err, domain.ErrTipoEquipoLargo),
		errors.Is(err, domain.ErrNombreEquipoVacio),
		errors.Is(err, domain.ErrNombreEquipoLargo),
		errors.Is(err, domain.ErrDescripcionVacia),
		errors.Is(err, domain.ErrEstadoEquipoInvalido),
		errors.Is(err, domain.ErrGravedadInvalida),
		errors.Is(err, domain.ErrEstadoIncidenciaInvalido),
		errors.Is(err, application.ErrIDInvalido),
		errors.Is(err, application.ErrReferenciaInexistente),
		errors.Is(err, domain.ErrNombreLicenciaVacio),
		errors.Is(err, domain.ErrNombreLicenciaLargo),
		errors.Is(err, domain.ErrDiasDuracionInvalido),
		errors.Is(err, domain.ErrDiasAvisoInvalido),
		errors.Is(err, domain.ErrDiasRestantesInvalido),
		errors.Is(err, application.ErrSinEquipos),
		errors.Is(err, application.ErrSinLicencias),
		errors.Is(err, application.ErrVencimientoAmbiguo):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
