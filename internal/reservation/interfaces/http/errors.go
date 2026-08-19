package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
)

func mapearError(err error) error {
	switch {
	case errors.Is(err, application.ErrReservaGrupoNoEncontrado),
		errors.Is(err, application.ErrReservaNoEncontrada),
		errors.Is(err, application.ErrPrestamoNoEncontrado):
		return fiber.NewError(fiber.StatusNotFound, err.Error())

	case errors.Is(err, application.ErrMateriaArchivada),
		errors.Is(err, application.ErrDocenteNoAsignado),
		errors.Is(err, application.ErrEquipoNoDisponible),
		errors.Is(err, application.ErrSolapamiento),
		// Los lotes de entrega informan estos dos por PC en el cuerpo, sin fallar;
		// acá solo llegan si alguna vez se pide una entrega de una sola máquina
		// como operación atómica.
		errors.Is(err, application.ErrEquipoYaPrestado),
		errors.Is(err, application.ErrEquipoDadoDeBaja),
		errors.Is(err, application.ErrReservaNoModificable),
		// Los tres del pedido de liberación: la reserva está, pero en un
		// estado que no admite el pedido (RF-04.12).
		errors.Is(err, application.ErrReservaSinDueño),
		errors.Is(err, application.ErrPedidoRepetido),
		errors.Is(err, application.ErrReservaYaEmpezada):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, application.ErrSinEquipos),
		errors.Is(err, application.ErrDemasiadosEquipos),
		errors.Is(err, application.ErrMotivoObligatorio),
		errors.Is(err, application.ErrDemasiadasOcurrencias),
		errors.Is(err, application.ErrSinOcurrencias),
		errors.Is(err, application.ErrIDInvalido),
		errors.Is(err, application.ErrReferenciaInexistente),
		errors.Is(err, domain.ErrRangoHorarioInvalido),
		errors.Is(err, domain.ErrRangoFechasInvalido),
		errors.Is(err, domain.ErrDiaSemanaInvalido),
		errors.Is(err, application.ErrFueraDeJornada),
		errors.Is(err, domain.ErrReservaEnElPasado),
		errors.Is(err, domain.ErrDuracionExcesiva),
		errors.Is(err, domain.ErrNombreDestinatarioVacio),
		errors.Is(err, domain.ErrNombreDestinatarioLargo):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	case errors.Is(err, domain.ErrTransicionReservaInvalida),
		errors.Is(err, domain.ErrTransicionGrupoInvalida),
		errors.Is(err, domain.ErrPrestamoYaDevuelto):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, application.ErrReservaAjena),
		// Pedirse a uno mismo no es un conflicto de estado sino de permiso:
		// sobre la reserva propia esta acción no existe.
		errors.Is(err, application.ErrReservaPropia):
		return fiber.NewError(fiber.StatusForbidden, err.Error())

	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
