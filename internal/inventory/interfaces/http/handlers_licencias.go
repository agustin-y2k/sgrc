package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/inventory/application"
)

// Licencias de software (RF-03.11 a RF-03.14). Todas las rutas son solo
// Admin: el docente no elige PC por su licencia —para eso está
// software_instalado, que sí ve— y una lista de vencimientos no le sirve
// para nada.

// GET /api/inventory/licencias (Admin)
//
// Sin paginar, como los demás listados acotados por el dominio (ciclos,
// cursos, carros): la cantidad la limita el inventario, no el uso.
func (h *Handler) ListarLicencias(c *fiber.Ctx) error {
	licencias, err := h.svc.ListarLicencias(c.UserContext())
	if err != nil {
		return mapearError(err)
	}

	hoy := h.svc.Hoy()
	data := make([]licenciaResponse, len(licencias))
	for i, l := range licencias {
		data[i] = toLicenciaConUbicacionResponse(l, hoy)
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/inventory/equipos/{equipoId}/licencias (Admin)
func (h *Handler) ListarLicenciasPorEquipo(c *fiber.Ctx) error {
	equipoID := c.Params("equipoId")

	licencias, err := h.svc.ListarLicenciasPorEquipo(c.UserContext(), equipoID)
	if err != nil {
		return mapearError(err)
	}

	hoy := h.svc.Hoy()
	data := make([]licenciaResponse, len(licencias))
	for i, l := range licencias {
		data[i] = toLicenciaResponse(l, hoy)
	}
	return c.JSON(fiber.Map{"data": data})
}

// POST /api/inventory/licencias (Admin) — alta de la misma licencia en
// varias PCs de una vez.
//
// Responde 201 aunque alguna se haya salteado: el lote se procesó, y qué
// pasó con cada PC está en el cuerpo. Un 409 acá obligaría a la pantalla a
// deshacer lo que sí entró.
func (h *Handler) CrearLicencias(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req crearLicenciasRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	vencimiento, err := req.aDominio()
	if err != nil {
		return err
	}

	diasAviso := diasAvisoPorDefecto
	if req.DiasAviso != nil {
		diasAviso = *req.DiasAviso
	}

	resultado, err := h.svc.CrearLicencias(c.UserContext(), application.NuevaLicenciaParams{
		EquipoIDs:    req.EquipoIDs,
		Nombre:       req.Nombre,
		DiasDuracion: req.DiasDuracion,
		DiasAviso:    diasAviso,
		Vencimiento:  vencimiento,
		PorUsuario:   claims.UserID,
	})
	if err != nil {
		return mapearError(err)
	}

	hoy := h.svc.Hoy()
	creadas := make([]licenciaResponse, len(resultado.Creadas))
	for i, l := range resultado.Creadas {
		creadas[i] = toLicenciaResponse(l, hoy)
	}
	return c.Status(fiber.StatusCreated).JSON(altaMasivaResponse{
		Creadas:              creadas,
		EquiposQueYaLaTenian: resultado.EquiposQueYaLaTenian,
	})
}

// POST /api/inventory/licencias/renovar (Admin)
//
// Es POST sobre un sub-recurso y no PATCH sobre cada licencia porque
// renovar ocho máquinas es UNA acción del Admin: partirla en ocho requests
// dejaría la pantalla mostrando cuatro renovadas y cuatro no si se corta la
// conexión en el medio.
func (h *Handler) RenovarLicencias(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req renovarLicenciasRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	renovadaEl, err := parsearFechaOpcional("renovadaEl", req.RenovadaEl)
	if err != nil {
		return err
	}

	resultado, err := h.svc.RenovarLicencias(c.UserContext(), req.LicenciaIDs, renovadaEl, claims.UserID)
	if err != nil {
		return mapearError(err)
	}

	hoy := h.svc.Hoy()
	renovadas := make([]licenciaResponse, len(resultado.Renovadas))
	for i, l := range resultado.Renovadas {
		renovadas[i] = toLicenciaResponse(l, hoy)
	}
	return c.JSON(renovacionResponse{
		Renovadas:      renovadas,
		SinFechaPrevia: resultado.SinFechaPrevia,
	})
}

// PATCH /api/inventory/licencias/{id} (Admin) — el "editar el contador en
// cualquier momento": corregir la fecha, cambiar la duración de 30 a 60
// días, o cargar el vencimiento de una licencia que se dio de alta sin él.
func (h *Handler) EditarLicencia(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req editarLicenciaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	vencimiento, err := req.aDominio()
	if err != nil {
		return err
	}

	if err := h.svc.EditarLicencia(c.UserContext(), id, application.EditarLicenciaParams{
		Nombre:       req.Nombre,
		DiasDuracion: req.DiasDuracion,
		DiasAviso:    req.DiasAviso,
		Vencimiento:  vencimiento,
		PorUsuario:   claims.UserID,
	}); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusOK)
}

// DELETE /api/inventory/licencias/{id} (Admin) — borrado real: de una
// licencia no cuelga nada que valga la pena conservar.
func (h *Handler) BorrarLicencia(c *fiber.Ctx) error {
	if err := h.svc.BorrarLicencia(c.UserContext(), c.Params("id")); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
