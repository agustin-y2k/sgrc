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
	// `/equipos` es UNA colección con filtros en la query, y no varias rutas
	// con la condición metida en el path. No es purismo: con una ruta como
	// `/equipos/sueltos`, Fiber resuelve por orden de registro, así que el
	// segmento literal tiene que ganarle a `/:id` o el parámetro se traga la
	// palabra y el listado devuelve un 404 buscando un equipo con ese ID. Es
	// una trampa que no avisa: compila, arranca y falla en tiempo de
	// ejecución. Con `?enCarro=false` no hay orden que respetar.
	//
	// A qué colección se hace POST decide dónde nace el equipo: en
	// `/carros/{id}/equipos` nace adentro de ese carro, en `/equipos` nace
	// suelto. Eso es también lo que cambia qué datos lleva —un equipo de
	// carro tiene identificador y número de serie, uno suelto tiene nombre—,
	// así que son dos cuerpos distintos y no uno con campos opcionales.
	inventory.Get("/equipos", autenticado, h.ListarEquipos)
	inventory.Post("/equipos", autenticado, soloAdmin, h.CrearEquipo)
	inventory.Post("/carros/:carroId/equipos", autenticado, soloAdmin, h.CrearEquipoDeCarro)
	inventory.Get("/carros/:carroId/equipos", autenticado, h.ListarEquiposPorCarro)
	inventory.Patch("/equipos/:id", autenticado, soloAdmin, h.EditarEquipo)
	inventory.Patch("/equipos/:id/estado", autenticado, soloAdmin, h.CambiarEstadoEquipo)
	inventory.Delete("/equipos/:id", autenticado, soloAdmin, h.DarDeBajaEquipo)

	// Las categorías de falla ya usadas son su propia colección y no algo
	// colgado de `/incidencias`: no son incidencias, son el vocabulario con
	// el que se las clasifica. Como sibling tampoco compite con
	// `/incidencias/:id`, que es el otro caso del orden de registro.
	inventory.Get("/categorias-de-falla", autenticado, h.ListarCategoriasDeFalla)

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

	// Preferencia de materia por equipo (RF-03.21). El ABM es solo Admin,
	// pero la lectura por equipo NO: la marca es lo que explica por qué la
	// lista de reserva viene ordenada como viene, y el docente ya ve ese
	// orden. Esconderle el motivo lo deja adivinando.
	inventory.Get("/equipos/:equipoId/preferencias", autenticado, h.ListarPreferenciasDeEquipo)
	inventory.Get("/materias-en-uso", autenticado, soloAdmin, h.ListarMateriasEnUso)
	inventory.Post("/preferencias", autenticado, soloAdmin, h.MarcarPreferencia)
	inventory.Patch("/preferencias/:id", autenticado, soloAdmin, h.EditarPreferencia)
	inventory.Delete("/preferencias/:id", autenticado, soloAdmin, h.BorrarPreferencia)
}
