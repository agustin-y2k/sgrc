package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta todas las rutas de academic bajo /api/academic.
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

	// Pedidos para dictar una materia (RF-02: la asignación docente-materia deja
	// de depender de encontrar a un Admin en el pasillo).
	academic.Post("/pedidos-de-materia", autenticado, middleware.RateLimit(5, time.Minute), h.PedirMateria)
	academic.Get("/pedidos-de-materia/mios", autenticado, h.MisPedidosDeMateria)
	academic.Get("/pedidos-de-materia", autenticado, soloAdmin, h.ListarPedidosDeMateria)
	academic.Post("/pedidos-de-materia/:id/resolver", autenticado, soloAdmin, h.ResolverPedidoDeMateria)

	// DocenteMateria
	academic.Post("/materias/:materiaId/docentes", autenticado, soloAdmin, h.AsignarDocente)
	academic.Get("/materias/:materiaId/docentes", autenticado, h.ListarDocentesDeMateria)
	academic.Patch("/materias/:materiaId/docentes/:docenteMateriaId", autenticado, soloAdmin, h.CambiarRolDocente)
	academic.Delete("/materias/:materiaId/docentes/:docenteMateriaId", autenticado, soloAdmin, h.RemoverDocenteMateria)
}
