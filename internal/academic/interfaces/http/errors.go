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
		errors.Is(err, application.ErrCicloArchivado),
		errors.Is(err, domain.ErrPedidoResuelto),
		// Las cuatro de corregir o eliminar un ciclo: el pedido está bien
		// formado, lo que no se puede es hacerlo con el ciclo en ese estado.
		errors.Is(err, application.ErrCicloConReservas),
		errors.Is(err, application.ErrAnioConBloqueos),
		errors.Is(err, application.ErrCicloConCursos),
		errors.Is(err, domain.ErrCicloArchivadoNoSeCorrige),
		errors.Is(err, domain.ErrCicloYaArchivado):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrAnioInvalido),
		errors.Is(err, domain.ErrAnioCursoInvalido),
		errors.Is(err, domain.ErrDivisionLarga),
		errors.Is(err, domain.ErrModalidadLarga),
		errors.Is(err, domain.ErrTextoIlegible),
		errors.Is(err, domain.ErrNombreMateriaVacio),
		errors.Is(err, domain.ErrNombreMateriaLargo),
		errors.Is(err, domain.ErrRolDocenteInvalido),
		errors.Is(err, application.ErrSinCursosParaImportar),
		errors.Is(err, application.ErrImportacionDemasiadoGrande),
		errors.Is(err, application.ErrSinCursosDestino),
		errors.Is(err, application.ErrCopiaAlMismoCurso),
		errors.Is(err, application.ErrCopiaEntreCiclos),
		errors.Is(err, application.ErrCursoOrigenSinMat),
		errors.Is(err, application.ErrDemasiadosDestinos),
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
