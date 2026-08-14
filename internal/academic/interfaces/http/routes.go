package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta todas las rutas de academic bajo /api/academic.
//
// Reglas de acceso (docs/09-seguridad-rbac.md §3): los GET de lectura son
// para cualquier usuario autenticado; crear/editar/eliminar/asignar es
// solo ADMIN.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	academic := app.Group("/api/academic")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	// Ciclo lectivo
	academic.Post("/ciclos", autenticado, soloAdmin, h.CrearCiclo)
	academic.Get("/ciclos", autenticado, h.ListarCiclos)

	// RF-04.1: en qué materias puede reservar quien está autenticado.
	academic.Get("/mis-materias", autenticado, h.ListarMisMaterias)
	academic.Post("/ciclos/:id/archivar", autenticado, soloAdmin, h.ArchivarCiclo)

	// Curso
	academic.Post("/ciclos/:cicloId/cursos", autenticado, soloAdmin, h.CrearCurso)
	academic.Get("/ciclos/:cicloId/cursos", autenticado, h.ListarCursos)
	academic.Patch("/cursos/:id", autenticado, soloAdmin, h.EditarCurso)
	academic.Delete("/cursos/:id", autenticado, soloAdmin, h.EliminarCurso)

	// Materia
	academic.Post("/cursos/:cursoId/materias", autenticado, soloAdmin, h.CrearMateria)
	academic.Get("/cursos/:cursoId/materias", autenticado, h.ListarMaterias)
	academic.Patch("/materias/:id", autenticado, soloAdmin, h.EditarMateria)
	academic.Delete("/materias/:id", autenticado, soloAdmin, h.EliminarMateria)

	// DocenteMateria
	academic.Post("/materias/:materiaId/docentes", autenticado, soloAdmin, h.AsignarDocente)
	academic.Get("/materias/:materiaId/docentes", autenticado, h.ListarDocentesDeMateria)
	academic.Patch("/materias/:materiaId/docentes/:docenteMateriaId", autenticado, soloAdmin, h.CambiarRolDocente)
	academic.Delete("/materias/:materiaId/docentes/:docenteMateriaId", autenticado, soloAdmin, h.RemoverDocenteMateria)
}
