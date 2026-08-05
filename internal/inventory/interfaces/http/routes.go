package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta todas las rutas de inventory bajo /api/inventory.
//
// RF-03.5: cualquier usuario autenticado (no solo Admin) puede reportar
// una incidencia — un docente que encuentra una PC rota tiene que poder
// avisar. El resto de las mutaciones son solo Admin.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	inventory := app.Group("/api/inventory")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	// Carro
	inventory.Post("/carros", autenticado, soloAdmin, h.CrearCarro)
	inventory.Get("/carros", autenticado, h.ListarCarros)
	inventory.Patch("/carros/:id", autenticado, soloAdmin, h.EditarCarro)

	// PC
	inventory.Post("/carros/:carroId/pcs", autenticado, soloAdmin, h.CrearPC)
	inventory.Get("/carros/:carroId/pcs", autenticado, h.ListarPCsPorCarro)
	inventory.Patch("/pcs/:id", autenticado, soloAdmin, h.EditarPC)
	inventory.Patch("/pcs/:id/estado", autenticado, soloAdmin, h.CambiarEstadoPC)
	inventory.Delete("/pcs/:id", autenticado, soloAdmin, h.DarDeBajaPC)

	// Incidencia
	inventory.Post("/incidencias", autenticado, h.CrearIncidencia)
	inventory.Get("/pcs/:pcId/incidencias", autenticado, h.ListarIncidenciasPorPC)
	inventory.Patch("/incidencias/:id", autenticado, soloAdmin, h.EditarIncidencia)
}
