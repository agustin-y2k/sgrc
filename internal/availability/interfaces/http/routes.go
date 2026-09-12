package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta las rutas de availability bajo /api
// (RF-07, ver docs/08-api-spec.yaml).
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	// El prefijo es "/api" y no "/api": lo que va en la URL es el
	// RECURSO, no el módulo de Go que lo sirve. Con el nombre del módulo, pedir
	// el calendario de una máquina obligaba a saber que el calendario lo sirve
	// `reservation` aunque la máquina sea de `inventory` — o sea, a aprender
	// cómo está partido el servidor por dentro. Mismo criterio que /api/jornada,
	// que ya lo hacía.
	availability := app.Group("/api")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	// La guardia cuelga de su propia ruta y no de "/admins" por lo mismo que la
	// jornada: lo que se pide acá no es la lista de administradores sino QUIÉN
	// ESTÁ DE GUARDIA y hasta cuándo (RF-07.2) — y de eso depende que el barrido
	// automático actúe (RF-07.6). Con el nombre viejo, además, «admins» existía
	// en dos módulos a la vez: acá y en /api/auth/admins, que es otra cosa
	// (dar de alta un administrador).
	guardia := app.Group("/api/guardia")
	guardia.Get("/", autenticado, h.DisponibilidadDeAdmins)

	availability.Get("/mi-horario", autenticado, soloAdmin, h.MiHorario)
	availability.Post("/mi-horario", autenticado, soloAdmin, h.AgregarBloque)
	availability.Patch("/mi-horario/:id", autenticado, soloAdmin, h.EditarBloque)
	availability.Delete("/mi-horario/:id", autenticado, soloAdmin, h.EliminarBloque)

	availability.Post("/mi-excepcion", autenticado, soloAdmin, h.CargarExcepcion)
	availability.Post("/no-disponible-ahora", autenticado, soloAdmin, h.MarcarNoDisponibleAhora)

	// La jornada de la institución cuelga de /api/jornada y no de
	// /api, aunque la sirva este mismo handler: es un dato de la
	// escuela, no la disponibilidad de una persona, y la URL es lo primero que
	// lee quien intenta entender la API. El GET es para cualquier autenticado,
	// no solo Admin: el formulario de reserva lo usa para avisar antes de
	// mandar, y el calendario para saber qué días dibujar.
	//
	// El PUT reemplaza la jornada entera —no hay endpoints por tramo— porque
	// es una sola decisión de siete días: así se valida como conjunto y hay un
	// único momento en el que confirmarla.
	jornada := app.Group("/api/jornada")
	jornada.Get("/", autenticado, h.Jornada)
	jornada.Put("/", autenticado, soloAdmin, h.ReemplazarJornada)
}
