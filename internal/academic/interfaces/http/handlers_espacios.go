package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/respuesta"
)

// Espacios (RF-02.13): los lugares de la institución que no son cursos.
//
// La API los trata como una colección hermana de /cursos y no como un curso con
// una bandera: son otra cosa, y el nombre de la ruta lo dice.

type espacioRequest struct {
	Nombre string `json:"nombre"`
}

type espacioResponse struct {
	ID             string `json:"id"`
	CicloLectivoID string `json:"cicloLectivoId"`
	Nombre         string `json:"nombre"`
	Archivado      bool   `json:"archivado"`
}

// POST /api/ciclos/{cicloId}/espacios (Admin)
func (h *Handler) CrearEspacio(c *fiber.Ctx) error {
	var req espacioRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	e, err := h.svc.CrearEspacio(c.UserContext(), c.Params("cicloId"), req.Nombre)
	if err != nil {
		return mapearError(err)
	}
	return respuesta.Creado(c, "/api/espacios", e.ID, espacioResponse{
		ID: e.ID, CicloLectivoID: e.CicloLectivoID, Nombre: e.Nombre, Archivado: e.Archivado,
	})
}

// GET /api/ciclos/{cicloId}/espacios (cualquier usuario autenticado)
func (h *Handler) ListarEspacios(c *fiber.Ctx) error {
	espacios, err := h.svc.ListarEspacios(c.UserContext(), c.Params("cicloId"))
	if err != nil {
		return mapearError(err)
	}
	data := make([]espacioResponse, len(espacios))
	for i, e := range espacios {
		data[i] = espacioResponse{
			ID: e.ID, CicloLectivoID: e.CicloLectivoID, Nombre: e.Nombre, Archivado: e.Archivado,
		}
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/espacios/{id} (cualquier usuario autenticado)
func (h *Handler) ObtenerEspacio(c *fiber.Ctx) error {
	e, err := h.svc.ObtenerEspacio(c.UserContext(), c.Params("id"))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(espacioResponse{
		ID: e.ID, CicloLectivoID: e.CicloLectivoID, Nombre: e.Nombre, Archivado: e.Archivado,
	})
}

// PATCH /api/espacios/{id} (Admin)
func (h *Handler) EditarEspacio(c *fiber.Ctx) error {
	var req espacioRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}
	if err := h.svc.EditarEspacio(c.UserContext(), c.Params("id"), req.Nombre); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// DELETE /api/espacios/{id} (Admin) — solo si sus materias no tienen reservas.
func (h *Handler) EliminarEspacio(c *fiber.Ctx) error {
	if err := h.svc.EliminarEspacio(c.UserContext(), c.Params("id")); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
