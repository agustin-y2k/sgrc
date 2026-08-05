package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/auth/application"
	"github.com/ramiro/sgrc/internal/auth/domain"
)

// mapearError traduce cada error de negocio de application/ y domain/ a su
// código HTTP según docs/08-api-spec.yaml. Centralizado acá para que
// ningún handler tenga que acordarse de memoria qué código corresponde a
// qué error — si mañana se agrega un error nuevo en application/, alcanza
// con agregar un case acá.
//
// El error que no matchea ningún case conocido cae al 500 genérico — nunca
// se expone el mensaje interno tal cual al cliente en ese caso, para no
// filtrar detalles de implementación (ej. un error de SQL crudo).
func mapearError(err error) error {
	switch {
	case errors.Is(err, application.ErrUsuarioNoEncontrado):
		return fiber.NewError(fiber.StatusNotFound, "usuario no encontrado")

	case errors.Is(err, application.ErrCredencialesInvalidas):
		return fiber.NewError(fiber.StatusUnauthorized, "credenciales inválidas")

	case errors.Is(err, application.ErrCuentaNoHabilitada):
		return fiber.NewError(fiber.StatusForbidden, "cuenta no habilitada")

	case errors.Is(err, application.ErrCuentaEnBaja):
		// Mensaje específico de RF-01.3 — se conserva el texto completo,
		// no un genérico, para que quien vuelve entienda que es su propia
		// cuenta vieja.
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, application.ErrEmailYaRegistrado):
		return fiber.NewError(fiber.StatusConflict, "email ya registrado")

	case errors.Is(err, application.ErrPasswordCorta),
		errors.Is(err, application.ErrDatosObligatorios),
		errors.Is(err, domain.ErrEmailInvalido):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	case errors.Is(err, application.ErrUltimoAdmin):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, application.ErrSoloDesdeBaja):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrTransicionInvalida):
		// Cubre, entre otros casos, el intento de cambiar el estado de
		// una cuenta que ya está en BAJA (RF-02.9: es terminal).
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrRolInvalido), errors.Is(err, domain.ErrEstadoInvalido):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	case errors.Is(err, application.ErrIDInvalido),
		errors.Is(err, application.ErrReferenciaInexistente):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
