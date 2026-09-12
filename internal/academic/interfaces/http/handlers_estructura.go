package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/audit"
)

// RF-02.12 — la estructura de un ciclo de a muchos: descargarla, volver a
// cargarla, y copiar las materias de un curso a otros.

// GET /api/ciclos/{cicloId}/estructura (Admin)
//
// Devuelve exactamente lo que acepta el POST de acá abajo. Que sean la misma
// forma es lo que hace que la descarga sirva para algo más que mirar: se corrige
// en una planilla y se vuelve a subir, o se sube sobre el ciclo del año
// siguiente.
func (h *Handler) ExportarEstructura(c *fiber.Ctx) error {
	cicloID := c.Params("cicloId")

	cursos, err := h.svc.ExportarEstructura(c.UserContext(), cicloID)
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(fiber.Map{"cursos": toCursosConMateriasDTO(cursos)})
}

// POST /api/ciclos/{cicloId}/estructura (Admin)
//
// Agrega lo que falta y no toca nada más: ni borra cursos que el archivo no
// nombra, ni renombra materias, ni toca docentes asignados. Subir dos veces el
// mismo archivo es inofensivo.
func (h *Handler) ImportarEstructura(c *fiber.Ctx) error {
	cicloID := c.Params("cicloId")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req importarEstructuraRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	res, err := h.svc.ImportarEstructura(c.UserContext(), cicloID, deCursosConMateriasDTO(req.Cursos))
	if err != nil {
		return mapearError(err)
	}

	// Se audita aunque no haya creado nada: el registro contesta «¿quién cargó
	// esto y cuándo?», y una importación que no creó nada igual explica por qué
	// alguien la ejecutó dos veces.
	h.auditar(c, claims.UserID, audit.EstructuraImportada, "ciclo_lectivo", &cicloID, map[string]any{
		"cursosCreados":      res.CursosCreados,
		"cursosExistentes":   res.CursosExistentes,
		"materiasCreadas":    res.MateriasCreadas,
		"materiasExistentes": res.MateriasExistentes,
	})

	return c.JSON(importarEstructuraResponse{
		CursosCreados:      res.CursosCreados,
		CursosExistentes:   res.CursosExistentes,
		MateriasCreadas:    res.MateriasCreadas,
		MateriasExistentes: res.MateriasExistentes,
	})
}

// POST /api/cursos/{id}/copiar-materias (Admin)
func (h *Handler) CopiarMaterias(c *fiber.Ctx) error {
	cursoOrigenID := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req copiarMateriasRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	res, err := h.svc.CopiarMaterias(c.UserContext(), cursoOrigenID, req.CursosDestinoIDs)
	if err != nil {
		return mapearError(err)
	}

	h.auditar(c, claims.UserID, audit.MateriasCopiadas, "curso", &cursoOrigenID, map[string]any{
		"cursosDestino":      res.CursosDestino,
		"materiasCreadas":    res.MateriasCreadas,
		"materiasExistentes": res.MateriasExistentes,
	})

	return c.JSON(copiarMateriasResponse{
		MateriasCreadas:    res.MateriasCreadas,
		MateriasExistentes: res.MateriasExistentes,
		CursosDestino:      res.CursosDestino,
	})
}
