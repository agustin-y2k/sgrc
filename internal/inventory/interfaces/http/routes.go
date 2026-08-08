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

	// Equipo — las computadoras de un carro y todo lo demás que se presta.
	//
	// /equipos/sueltos va ANTES que /equipos/:id, y eso no es cosmético:
	// Fiber resuelve por orden de registro, así que al revés el :id se
	// tragaría la palabra "sueltos" y el listado devolvería un 404 buscando
	// un equipo con ese ID.
	inventory.Post("/equipos/sueltos", autenticado, soloAdmin, h.CrearEquipo)
	inventory.Get("/equipos/sueltos", autenticado, h.ListarEquiposSueltos)

	inventory.Post("/carros/:carroId/equipos", autenticado, soloAdmin, h.CrearEquipoDeCarro)
	inventory.Get("/carros/:carroId/equipos", autenticado, h.ListarEquiposPorCarro)
	inventory.Patch("/equipos/:id", autenticado, soloAdmin, h.EditarEquipo)
	inventory.Patch("/equipos/:id/estado", autenticado, soloAdmin, h.CambiarEstadoEquipo)
	inventory.Delete("/equipos/:id", autenticado, soloAdmin, h.DarDeBajaEquipo)

	// Incidencia
	inventory.Post("/incidencias", autenticado, h.CrearIncidencia)
	inventory.Get("/equipos/:equipoId/incidencias", autenticado, h.ListarIncidenciasPorEquipo)
	inventory.Patch("/incidencias/:id", autenticado, soloAdmin, h.EditarIncidencia)

	// Licencias de software (RF-03.11 a RF-03.14). Todas solo Admin,
	// incluidas las lecturas: el docente elige el equipo por software_instalado,
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
	inventory.Get("/equipos/:equipoId/licencias", autenticado, soloAdmin, h.ListarLicenciasPorEquipo)
}
