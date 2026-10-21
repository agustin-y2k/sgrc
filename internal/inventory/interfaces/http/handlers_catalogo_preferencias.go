package http

import "github.com/gofiber/fiber/v2"

type materiaParaPreferenciaResponse struct {
	MateriaID        string  `json:"materiaId"`
	MateriaNombre    string  `json:"materiaNombre"`
	ContenedorID     string  `json:"contenedorId"`
	ContenedorNombre string  `json:"contenedorNombre"`
	Tipo             string  `json:"tipo"`
	Modalidad        *string `json:"modalidad,omitempty"`
	Anio             *int    `json:"anio,omitempty"`
	Division         *string `json:"division,omitempty"`
}

func (h *Handler) CatalogoDePrioridades(c *fiber.Ctx) error {
	filas, err := h.svc.CatalogoDePrioridades(c.UserContext())
	if err != nil {
		return mapearError(err)
	}
	data := make([]materiaParaPreferenciaResponse, 0, len(filas))
	for _, f := range filas {
		data = append(data, materiaParaPreferenciaResponse{f.MateriaID, f.MateriaNombre, f.ContenedorID, f.ContenedorNombre, f.Tipo, f.Modalidad, f.Anio, f.Division})
	}
	return c.JSON(fiber.Map{"data": data})
}
