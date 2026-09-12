package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta todas las rutas de academic bajo /api.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	// El prefijo es "/api" y no "/api": lo que va en la URL es el
	// RECURSO, no el módulo de Go que lo sirve. Con el nombre del módulo, pedir
	// el calendario de una máquina obligaba a saber que el calendario lo sirve
	// `reservation` aunque la máquina sea de `inventory` — o sea, a aprender
	// cómo está partido el servidor por dentro. Mismo criterio que /api/jornada,
	// que ya lo hacía.
	academic := app.Group("/api")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	// Ciclo lectivo
	academic.Post("/ciclos", autenticado, soloAdmin, h.CrearCiclo)
	academic.Get("/ciclos/:id", autenticado, h.ObtenerCiclo)
	academic.Get("/ciclos", autenticado, h.ListarCiclos)

	// RF-04.1: en qué materias puede reservar quien está autenticado.
	academic.Get("/mis-materias", autenticado, h.ListarMisMaterias)
	// Quién dicta qué en un ciclo, para las dos pantallas que muestran la
	// relación docente-materia (RF-02.6).
	academic.Get("/ciclos/:cicloId/asignaciones", autenticado, soloAdmin, h.ListarAsignaciones)
	// La misma pregunta sobre otra persona, para el mostrador (RF-08.23).
	academic.Get("/docentes/:usuarioId/materias", autenticado, soloAdmin, h.ListarMateriasDeDocente)
	// Corregir y eliminar un ciclo: las dos existen para deshacer uno creado con
	// el año equivocado, que hasta acá se quedaba con ese año para siempre.
	academic.Patch("/ciclos/:id", autenticado, soloAdmin, h.CorregirCiclo)
	academic.Delete("/ciclos/:id", autenticado, soloAdmin, h.EliminarCiclo)
	academic.Post("/ciclos/:id/archivar", autenticado, soloAdmin, h.ArchivarCiclo)

	// La estructura entera del ciclo —cursos con sus materias— para descargarla
	// y volver a cargarla (RF-02.12). Las dos rutas son la misma para que el
	// verbo diga la dirección y el cuerpo sea el mismo en las dos.
	academic.Get("/ciclos/:cicloId/estructura", autenticado, soloAdmin, h.ExportarEstructura)
	academic.Post("/ciclos/:cicloId/estructura", autenticado, soloAdmin, h.ImportarEstructura)

	// Curso
	academic.Post("/ciclos/:cicloId/cursos", autenticado, soloAdmin, h.CrearCurso)
	academic.Get("/ciclos/:cicloId/cursos", autenticado, h.ListarCursos)
	academic.Get("/cursos/:id", autenticado, h.ObtenerCurso)
	academic.Patch("/cursos/:id", autenticado, soloAdmin, h.EditarCurso)
	academic.Delete("/cursos/:id", autenticado, soloAdmin, h.EliminarCurso)
	academic.Post("/cursos/:id/copiar-materias", autenticado, soloAdmin, h.CopiarMaterias)

	// Materia
	academic.Post("/cursos/:cursoId/materias", autenticado, soloAdmin, h.CrearMateria)
	academic.Get("/cursos/:cursoId/materias", autenticado, h.ListarMaterias)
	academic.Get("/materias/:id", autenticado, h.ObtenerMateria)
	academic.Patch("/materias/:id", autenticado, soloAdmin, h.EditarMateria)
	academic.Delete("/materias/:id", autenticado, soloAdmin, h.EliminarMateria)

	// Pedidos para dictar una materia (RF-02: la asignación docente-materia deja
	// de depender de encontrar a un Admin en el pasillo).
	academic.Post("/pedidos-de-materia", autenticado, middleware.RateLimit(5, time.Minute), h.PedirMateria)
	// Ídem /api/mis-sugerencias: lo propio no es un id de la colección.
	academic.Get("/mis-pedidos-de-materia", autenticado, h.MisPedidosDeMateria)
	academic.Get("/pedidos-de-materia", autenticado, soloAdmin, h.ListarPedidosDeMateria)
	academic.Post("/pedidos-de-materia/:id/resolver", autenticado, soloAdmin, h.ResolverPedidoDeMateria)

	// DocenteMateria
	academic.Post("/materias/:materiaId/docentes", autenticado, soloAdmin, h.AsignarDocente)
	academic.Get("/materias/:materiaId/docentes", autenticado, h.ListarDocentesDeMateria)
	academic.Patch("/materias/:materiaId/docentes/:docenteMateriaId", autenticado, soloAdmin, h.CambiarRolDocente)
	academic.Delete("/materias/:materiaId/docentes/:docenteMateriaId", autenticado, soloAdmin, h.RemoverDocenteMateria)
}
