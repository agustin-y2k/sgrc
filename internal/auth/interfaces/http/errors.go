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
		// Un solo case para los tres motivos (pendiente, rechazada, en baja)
		// más el genérico: los tres matchean contra este paraguas por su
		// método Is, y err.Error() ya trae el texto que corresponde a cada
		// uno. Todos son 403 — cambia la explicación, no el veredicto.
		return fiber.NewError(fiber.StatusForbidden, err.Error())

	case errors.Is(err, application.ErrCuentaEnBaja):
		// Mensaje específico de RF-01.3 — se conserva el texto completo,
		// no un genérico, para que quien vuelve entienda que es su propia
		// cuenta vieja.
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, application.ErrEmailYaRegistrado):
		return fiber.NewError(fiber.StatusConflict, "email ya registrado")

	// ── Ingreso con Google ───────────────────────────────────────────
	//
	// El 404 es parte del contrato de POST /api/auth/google, no un fallo:
	// significa "el token es bueno pero todavía no tenés cuenta", y es lo
	// que el frontend usa para mandar a completar el registro.
	case errors.Is(err, application.ErrCuentaGoogleNoRegistrada):
		return fiber.NewError(fiber.StatusNotFound, err.Error())

	case errors.Is(err, application.ErrTokenGoogleInvalido):
		// Mensaje fijo, no err.Error(): el error envuelto lleva el detalle
		// de qué chequeo falló (firma, aud, exp) y eso queda para el log
		// del servidor, no para quien mandó el token.
		return fiber.NewError(fiber.StatusUnauthorized, "el token de Google no es válido")

	case errors.Is(err, application.ErrEmailNoVerificadoPorGoogle),
		errors.Is(err, application.ErrDominioNoPermitido):
		return fiber.NewError(fiber.StatusForbidden, err.Error())

	case errors.Is(err, application.ErrLoginGoogleNoDisponible):
		// 503 y no 400: el pedido está bien formado, es el sistema el que
		// no tiene esta capacidad configurada.
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())

	case errors.Is(err, application.ErrCuentaSinPassword):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	// ── Recuperación de contraseña ───────────────────────────────────
	//
	// Los tres van a 400 y no a 401/404: no son un problema de
	// autenticación (no hay sesión que autenticar) ni un recurso que falte,
	// es un dato del formulario que no sirve. El 404 sería además lo único
	// del sistema capaz de confirmar que un email existe.
	case errors.Is(err, application.ErrCodigoRecuperacionInvalido),
		errors.Is(err, application.ErrCodigoRecuperacionVencido),
		errors.Is(err, application.ErrCodigoRecuperacionSinIntentos):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	case errors.Is(err, application.ErrRecuperacionNoDisponible):
		// 503, igual que el ingreso con Google sin configurar: el pedido
		// está bien formado, es el despliegue el que no tiene correo.
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())

	case errors.Is(err, application.ErrPasswordCorta),
		errors.Is(err, application.ErrDatosObligatorios),
		errors.Is(err, domain.ErrEmailInvalido):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	case errors.Is(err, application.ErrUltimoAdmin):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, application.ErrSoloDesdeBajaORechazada):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, application.ErrAutoDegradacion):
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrPromocionInvalida),
		errors.Is(err, domain.ErrDegradacionInvalida):
		// Se conserva el mensaje completo: dice si la cuenta ya tenía ese
		// rol o si le falta la aprobación, que es justo lo que el Admin
		// necesita para saber qué hacer.
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrTransicionInvalida):
		// Cubre, entre otros casos, el intento de cambiar el estado de
		// una cuenta que ya está en BAJA (RF-02.9: es terminal).
		return fiber.NewError(fiber.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrRolInvalido), errors.Is(err, domain.ErrEstadoInvalido),
		errors.Is(err, domain.ErrRolSolicitadoInvalido):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	case errors.Is(err, application.ErrIDInvalido),
		errors.Is(err, application.ErrReferenciaInexistente):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	default:
		return fiber.NewError(fiber.StatusInternalServerError, "error interno")
	}
}
