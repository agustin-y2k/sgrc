package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/inventory/application"
)

// Marcas de preferencia de materia (RF-03.21).

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

// POST /api/inventory/preferencias (Admin) — la misma marca en varios equipos
// de una vez.
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

// DELETE /api/inventory/preferencias/{id} (Admin) No hay nada que confirmar
// ni ninguna cascada que avisar: sacar la marca devuelve el equipo al orden
// neutral y no toca ninguna reserva.
func (h *Handler) BorrarPreferencia(c *fiber.Ctx) error {
	if err := h.svc.BorrarPreferencia(c.UserContext(), c.Params("id")); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// prioridadPorDefecto: sin decir nada, la marca es de la prioridad más
// fuerte.
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
