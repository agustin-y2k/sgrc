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
		errors.Is(err, application.ErrLicenciaNoEncontrada),
		errors.Is(err, application.ErrCuentaDeEquipoNoEncontrada),
		// 404 y no 204 vacío: se pidió ver una contraseña que no está anotada,
		// y decirlo es más útil que devolver un vacío que parece un error.
		errors.Is(err, application.ErrPasswordNoGuardada),
		errors.Is(err, domain.ErrPreferenciaNoEncontr):
		return fiber.NewError(fiber.StatusNotFound, err.Error())

	// La cuenta existe y la contraseña también; lo que falta es el permiso
	// para verla. 403 y no 404: esconder que existe no protegería nada —la
	// cuenta ya se lista— y confundiría a quien pregunta por qué no la ve.
	case errors.Is(err, application.ErrNoAutorizado):
		return fiber.NewError(fiber.StatusForbidden, err.Error())

	// 503 y no 500: el sistema anda, lo que falta es una configuración de este
	// despliegue. El mensaje dice cuál.
	case errors.Is(err, application.ErrSinClaveDeCifrado):
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())

	// 409 y no 500: el pedido está bien y el sistema también; lo que no encaja
	// es lo guardado contra la clave que corre hoy. Sin este caso caía al 500
	// "error interno" —sin mensaje y sin línea en el log—, y quien apretaba
	// "Ver contraseña" no tenía forma de saber que la salida es volver a
	// cargarla. Mismo criterio que los errores de más abajo: sin un caso acá,
	// una situación prevista se ve como una falla del sistema.
	case errors.Is(err, application.ErrPasswordIlegible):
		return fiber.NewError(fiber.StatusConflict, application.ErrPasswordIlegible.Error())

	case errors.Is(err, application.ErrNombreCarroDuplicado),
		errors.Is(err, application.ErrIdentificadorDuplicado),
		errors.Is(err, application.ErrNumeroSerieDuplicado),
		// El alta masiva saltea los duplicados y los informa en el cuerpo; esto
		// solo salta al RENOMBRAR una licencia al nombre de otra que esa misma PC
		// ya tiene.
		errors.Is(err, application.ErrLicenciaDuplicada),
		errors.Is(err, application.ErrNombreDeEquipoDuplicado),
		errors.Is(err, application.ErrCuentaDeEquipoDuplicada),
		// Igual que las licencias: el alta masiva saltea los duplicados y los
		// informa en el cuerpo; esto solo salta al EDITAR una marca hasta dejarla
		// igual a otra del mismo equipo.
		errors.Is(err, domain.ErrPreferenciaDuplicada):
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
		errors.Is(err, application.ErrVencimientoAmbiguo),
		errors.Is(err, domain.ErrMateriaPreferidaVacia),
		errors.Is(err, domain.ErrMateriaPreferidaLarga),
		errors.Is(err, domain.ErrAnioPreferenciaInvalido),
		errors.Is(err, domain.ErrDivisionPreferenciaInvalida),
		errors.Is(err, domain.ErrDivisionSinAnio),
		errors.Is(err, domain.ErrPrioridadInvalida),
		errors.Is(err, domain.ErrSinEquiposParaPreferi),
		// Las cuentas de cada equipo (RF-03.22). Sin estos casos, escribir mal
		// una cuenta contestaba "error interno" en vez de decir qué falta.
		errors.Is(err, domain.ErrUsuarioCuentaVacio),
		errors.Is(err, domain.ErrUsuarioCuentaLargo),
		errors.Is(err, domain.ErrClaseCuentaVacia),
		errors.Is(err, domain.ErrClaseCuentaLarga),
		errors.Is(err, domain.ErrPrivilegioInvalido),
		errors.Is(err, domain.ErrVisibilidadInvalida),
		errors.Is(err, domain.ErrNotasCuentaLargas),
		errors.Is(err, domain.ErrPasswordCuentaLarga),
		errors.Is(err, domain.ErrPasswordSinTenerlaEs):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
