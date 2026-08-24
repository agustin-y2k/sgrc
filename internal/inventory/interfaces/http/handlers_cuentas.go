package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// Las cuentas de usuario de cada equipo (RF-03.22).

// esAdmin lee el rol del token ya validado. El middleware reemplaza el rol del
// token por el de la base antes de dejar pasar, así que esto es el rol de
// AHORA y no el del momento del login.
func esAdmin(c *fiber.Ctx) bool {
	claims := middleware.ClaimsFromCtx(c)
	return claims != nil && claims.Rol == "ADMIN"
}

// GET /api/inventory/equipos/{equipoId}/cuentas — cualquier autenticado.
//
// La lista sale para todos: la cuenta y su privilegio no son el secreto. Lo
// que cambia según quién pregunta es el campo `puedeVerLaPassword` de cada
// fila, que lo resuelve el servidor.
func (h *Handler) ListarCuentasDeEquipo(c *fiber.Ctx) error {
	equipoID := c.Params("equipoId")

	cuentas, err := h.svc.ListarCuentasDeEquipo(c.UserContext(), equipoID, esAdmin(c))
	if err != nil {
		return mapearError(err)
	}

	respuesta := make([]cuentaResponse, 0, len(cuentas))
	for _, cu := range cuentas {
		respuesta = append(respuesta, toCuentaResponse(cu))
	}
	return c.JSON(fiber.Map{"data": respuesta})
}

// GET /api/inventory/clases-de-cuenta — las clases ya usadas, para sugerir.
//
// Colección propia y no algo colgado de un equipo, igual que
// /categorias-de-falla: no son cuentas, son el vocabulario con el que se las
// clasifica.
func (h *Handler) ListarClasesDeCuenta(c *fiber.Ctx) error {
	clases, err := h.svc.ClasesDeCuentaUsadas(c.UserContext())
	if err != nil {
		return mapearError(err)
	}
	if clases == nil {
		clases = []string{}
	}
	return c.JSON(fiber.Map{"data": clases})
}

// POST /api/inventory/equipos/{equipoId}/cuentas (Admin)
func (h *Handler) CrearCuentaDeEquipo(c *fiber.Ctx) error {
	equipoID := c.Params("equipoId")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req cuentaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	datos, err := req.aDatosDeDominio()
	if err != nil {
		return mapearError(err)
	}

	cuenta, err := h.svc.CrearCuentaDeEquipo(c.UserContext(), equipoID, datos, req.Password)
	if err != nil {
		return mapearError(err)
	}

	// La contraseña NO va en el detalle de la auditoría: el registro de quién
	// tocó qué no puede ser, él mismo, otra copia de las contraseñas.
	h.auditar(c, claims.UserID, audit.CuentaDeEquipoCreada, "equipo_cuenta", &cuenta.ID, map[string]any{
		"equipoId":    equipoID,
		"usuario":     cuenta.Usuario,
		"privilegio":  string(cuenta.Privilegio),
		"visibilidad": string(cuenta.Visibilidad),
	})

	return c.Status(fiber.StatusCreated).JSON(toCuentaResponse(application.CuentaVisible{
		CuentaDeEquipo:     cuenta,
		PuedeVerLaPassword: true, // lo acaba de crear un Admin
		HayPasswordParaVer: cuenta.HayPasswordGuardada(),
	}))
}

// PATCH /api/inventory/cuentas/{id} (Admin)
func (h *Handler) EditarCuentaDeEquipo(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req editarCuentaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	params, err := req.aParamsDeDominio()
	if err != nil {
		return mapearError(err)
	}

	cuenta, err := h.svc.EditarCuentaDeEquipo(c.UserContext(), id, params)
	if err != nil {
		return mapearError(err)
	}

	h.auditar(c, claims.UserID, audit.CuentaDeEquipoEditada, "equipo_cuenta", &id, map[string]any{
		"equipoId":    cuenta.EquipoID,
		"usuario":     cuenta.Usuario,
		"privilegio":  string(cuenta.Privilegio),
		"visibilidad": string(cuenta.Visibilidad),
	})

	return c.JSON(toCuentaResponse(application.CuentaVisible{
		CuentaDeEquipo:     cuenta,
		PuedeVerLaPassword: true,
		HayPasswordParaVer: cuenta.HayPasswordGuardada(),
	}))
}

// DELETE /api/inventory/cuentas/{id} (Admin)
func (h *Handler) BorrarCuentaDeEquipo(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	if err := h.svc.BorrarCuentaDeEquipo(c.UserContext(), id); err != nil {
		return mapearError(err)
	}

	h.auditar(c, claims.UserID, audit.CuentaDeEquipoBorrada, "equipo_cuenta", &id, nil)
	return c.SendStatus(fiber.StatusNoContent)
}

// POST /api/inventory/cuentas/{id}/password — cualquier autenticado; el
// servicio decide si le corresponde.
//
// Es POST y no GET a propósito, por dos razones: un GET termina en el historial
// del navegador y en los logs de acceso con la URL completa, y además esto no
// es una lectura inocua — cada llamada queda registrada como que alguien miró
// esa contraseña.
func (h *Handler) RevelarPasswordDeCuenta(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	cuenta, password, err := h.svc.RevelarPasswordDeCuenta(c.UserContext(), id, esAdmin(c))
	if err != nil {
		return mapearError(err)
	}

	// Se audita SIEMPRE, también cuando la cuenta es pública: el registro sirve
	// para reconstruir quién sabía qué el día que una máquina apareció con algo
	// cambiado, y una cuenta pública no es menos interesante para eso.
	h.auditar(c, claims.UserID, audit.PasswordDeEquipoRevelada, "equipo_cuenta", &id, map[string]any{
		"equipoId":    cuenta.EquipoID,
		"usuario":     cuenta.Usuario,
		"visibilidad": string(cuenta.Visibilidad),
	})

	return c.JSON(fiber.Map{"password": password})
}

// aDatosDeDominio traduce y valida los enums del request. La clase no se
// valida acá: es texto libre y de eso se ocupa el dominio.
func (r cuentaRequest) aDatosDeDominio() (domain.DatosDeCuenta, error) {
	privilegio, err := domain.ParsePrivilegioDeCuenta(r.Privilegio)
	if err != nil {
		return domain.DatosDeCuenta{}, err
	}
	visibilidad, err := domain.ParseVisibilidadDeCuenta(r.Visibilidad)
	if err != nil {
		return domain.DatosDeCuenta{}, err
	}
	return domain.DatosDeCuenta{
		Usuario:       r.Usuario,
		Clase:         r.Clase,
		Privilegio:    privilegio,
		Visibilidad:   visibilidad,
		TienePassword: r.TienePassword,
		Notas:         r.Notas,
	}, nil
}

func (r editarCuentaRequest) aParamsDeDominio() (application.EditarCuentaParams, error) {
	params := application.EditarCuentaParams{
		Usuario:       r.Usuario,
		Clase:         r.Clase,
		TienePassword: r.TienePassword,
		Notas:         r.Notas,
		Password:      r.Password,
	}
	if r.Privilegio != nil {
		privilegio, err := domain.ParsePrivilegioDeCuenta(*r.Privilegio)
		if err != nil {
			return params, err
		}
		params.Privilegio = &privilegio
	}
	if r.Visibilidad != nil {
		visibilidad, err := domain.ParseVisibilidadDeCuenta(*r.Visibilidad)
		if err != nil {
			return params, err
		}
		params.Visibilidad = &visibilidad
	}
	return params, nil
}
