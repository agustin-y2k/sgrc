package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/inventory/application"
)

// Marcas de preferencia de materia (RF-03.21).
//
// El ABM es sólo Admin, pero LEER las marcas de un equipo lo puede hacer
// cualquiera autenticado: son la explicación de por qué la lista de reserva
// aparece ordenada como aparece, y esconderle al docente el motivo de un
// orden que sí ve es dejarlo adivinando.

// GET /api/inventory/equipos/{equipoId}/preferencias
func (h *Handler) ListarPreferenciasDeEquipo(c *fiber.Ctx) error {
	equipoID := c.Params("equipoId")

	preferencias, err := h.svc.ListarPreferenciasDeEquipo(c.UserContext(), equipoID)
	if err != nil {
		return mapearError(err)
	}

	data := make([]preferenciaResponse, len(preferencias))
	for i, p := range preferencias {
		data[i] = toPreferenciaResponse(p)
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/inventory/materias-en-uso (Admin) — el selector del formulario.
//
// Existe para que el Admin ELIJA el nombre en vez de tipearlo: la marca se
// guarda como texto, y ese selector es lo único que impide que "Matemática"
// y "Matematica" nazcan como dos marcas distintas.
func (h *Handler) ListarMateriasEnUso(c *fiber.Ctx) error {
	nombres, err := h.svc.NombresDeMateriaEnUso(c.UserContext())
	if err != nil {
		return mapearError(err)
	}
	if nombres == nil {
		nombres = []string{}
	}
	return c.JSON(fiber.Map{"data": nombres})
}

// POST /api/inventory/preferencias (Admin) — la misma marca en varios
// equipos de una vez.
//
// Responde 201 aunque alguna se haya salteado, con el mismo criterio que el
// alta masiva de licencias: el lote se procesó, y qué pasó con cada máquina
// está en el cuerpo.
func (h *Handler) MarcarPreferencia(c *fiber.Ctx) error {
	var req marcarPreferenciaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	resultado, err := h.svc.MarcarPreferencia(c.UserContext(), application.NuevaPreferenciaParams{
		EquipoIDs:     req.EquipoIDs,
		MateriaNombre: req.MateriaNombre,
		Anio:          req.Anio,
		Division:      req.Division,
		Prioridad:     req.prioridadOPorDefecto(),
	})
	if err != nil {
		return mapearError(err)
	}

	creadas := make([]preferenciaResponse, len(resultado.Creadas))
	for i, p := range resultado.Creadas {
		creadas[i] = toPreferenciaResponse(p)
	}
	return c.Status(fiber.StatusCreated).JSON(altaDePreferenciasResponse{
		Creadas:          creadas,
		EquiposQueYaTeni: resultado.EquiposQueYaTeni,
	})
}

// PATCH /api/inventory/preferencias/{id} (Admin) — corrige el alcance y la
// prioridad. La materia no se edita: apuntar a otra es otra marca.
func (h *Handler) EditarPreferencia(c *fiber.Ctx) error {
	id := c.Params("id")

	var req editarPreferenciaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	p, err := h.svc.EditarPreferencia(c.UserContext(), id, req.Anio, req.Division, req.prioridadOPorDefecto())
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toPreferenciaResponse(p))
}

// DELETE /api/inventory/preferencias/{id} (Admin)
//
// No hay nada que confirmar ni ninguna cascada que avisar: sacar la marca
// devuelve el equipo al orden neutral y no toca ninguna reserva.
func (h *Handler) BorrarPreferencia(c *fiber.Ctx) error {
	if err := h.svc.BorrarPreferencia(c.UserContext(), c.Params("id")); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// prioridadPorDefecto: sin decir nada, la marca es de la prioridad más
// fuerte. Es lo que espera quien marca una sola máquina para una materia y
// no piensa en escalones — que va a ser el caso mayoritario.
const prioridadPorDefecto = 1

func (r marcarPreferenciaRequest) prioridadOPorDefecto() int {
	if r.Prioridad == nil {
		return prioridadPorDefecto
	}
	return *r.Prioridad
}

func (r editarPreferenciaRequest) prioridadOPorDefecto() int {
	if r.Prioridad == nil {
		return prioridadPorDefecto
	}
	return *r.Prioridad
}
