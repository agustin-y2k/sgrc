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

	// Equipos que no están en ningún carro (RF-03.15): el proyector, los
	// cargadores. Se listan para cualquier autenticado por el mismo motivo
	// que las PCs; darlos de alta es solo de Admin.
	inventory.Post("/equipos", autenticado, soloAdmin, h.CrearEquipo)
	inventory.Get("/equipos", autenticado, h.ListarEquiposSueltos)

	// Incidencia
	inventory.Post("/incidencias", autenticado, h.CrearIncidencia)
	inventory.Get("/pcs/:pcId/incidencias", autenticado, h.ListarIncidenciasPorPC)
	inventory.Patch("/incidencias/:id", autenticado, soloAdmin, h.EditarIncidencia)

	// Licencias de software (RF-03.11 a RF-03.14). Todas solo Admin,
	// incluidas las lecturas: el docente elige PC por software_instalado,
	// que ya ve en la pantalla de reserva; cuándo vence una licencia es
	// trabajo administrativo y no le sirve para decidir nada.
	//
	// /licencias/renovar va antes que /licencias/:id por costumbre, aunque
	// acá no haría falta: son métodos distintos (POST contra PATCH) y Fiber
	// no las puede confundir.
	inventory.Get("/licencias", autenticado, soloAdmin, h.ListarLicencias)
	inventory.Post("/licencias", autenticado, soloAdmin, h.CrearLicencias)
	inventory.Post("/licencias/renovar", autenticado, soloAdmin, h.RenovarLicencias)
	inventory.Patch("/licencias/:id", autenticado, soloAdmin, h.EditarLicencia)
	inventory.Delete("/licencias/:id", autenticado, soloAdmin, h.BorrarLicencia)
	inventory.Get("/pcs/:pcId/licencias", autenticado, soloAdmin, h.ListarLicenciasPorPC)
}
