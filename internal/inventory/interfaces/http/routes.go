package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta todas las rutas de inventory bajo /api/inventory.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	inventory := app.Group("/api/inventory")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	// Carro
	inventory.Post("/carros", autenticado, soloAdmin, h.CrearCarro)
	inventory.Get("/carros", autenticado, h.ListarCarros)
	inventory.Patch("/carros/:id", autenticado, soloAdmin, h.EditarCarro)

	// Equipo — las computadoras de un carro y todo lo demás que se presta.
	inventory.Get("/equipos", autenticado, h.ListarEquipos)
	inventory.Post("/equipos", autenticado, soloAdmin, h.CrearEquipo)
	inventory.Post("/carros/:carroId/equipos", autenticado, soloAdmin, h.CrearEquipoDeCarro)
	inventory.Get("/carros/:carroId/equipos", autenticado, h.ListarEquiposPorCarro)
	inventory.Patch("/equipos/:id", autenticado, soloAdmin, h.EditarEquipo)
	inventory.Patch("/equipos/:id/estado", autenticado, soloAdmin, h.CambiarEstadoEquipo)
	inventory.Delete("/equipos/:id", autenticado, soloAdmin, h.DarDeBajaEquipo)

	// Las categorías de falla ya usadas son su propia colección y no algo
	// colgado de `/incidencias`: no son incidencias, son el vocabulario con el
	// que se las clasifica.
	inventory.Get("/categorias-de-falla", autenticado, h.ListarCategoriasDeFalla)

	// Incidencia
	inventory.Post("/incidencias", autenticado, h.CrearIncidencia)
	inventory.Get("/equipos/:equipoId/incidencias", autenticado, h.ListarIncidenciasPorEquipo)
	inventory.Patch("/incidencias/:id", autenticado, soloAdmin, h.EditarIncidencia)

	// Licencias de software (RF-03.11 a RF-03.14).
	inventory.Get("/licencias", autenticado, soloAdmin, h.ListarLicencias)
	inventory.Post("/licencias", autenticado, soloAdmin, h.CrearLicencias)
	inventory.Post("/licencias/renovar", autenticado, soloAdmin, h.RenovarLicencias)
	inventory.Patch("/licencias/:id", autenticado, soloAdmin, h.EditarLicencia)
	inventory.Delete("/licencias/:id", autenticado, soloAdmin, h.BorrarLicencia)
	inventory.Get("/equipos/:equipoId/licencias", autenticado, soloAdmin, h.ListarLicenciasPorEquipo)

	// Preferencia de materia por equipo (RF-03.21).
	inventory.Get("/equipos/:equipoId/preferencias", autenticado, h.ListarPreferenciasDeEquipo)
	inventory.Get("/materias-en-uso", autenticado, soloAdmin, h.ListarMateriasEnUso)
	inventory.Post("/preferencias", autenticado, soloAdmin, h.MarcarPreferencia)
	inventory.Patch("/preferencias/:id", autenticado, soloAdmin, h.EditarPreferencia)
	inventory.Delete("/preferencias/:id", autenticado, soloAdmin, h.BorrarPreferencia)
}
