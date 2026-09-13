package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta las rutas de reporting bajo /api — todas
// exclusivas de Admin (RF-06 es una funcionalidad de gestión, no algo que un
// docente necesite consultar sobre otros).
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	// El prefijo es "/api" y no "/api": lo que va en la URL es el
	// RECURSO, no el módulo de Go que lo sirve. Con el nombre del módulo, pedir
	// el calendario de una máquina obligaba a saber que el calendario lo sirve
	// `reservation` aunque la máquina sea de `inventory` — o sea, a aprender
	// cómo está partido el servidor por dentro. Mismo criterio que /api/jornada,
	// que ya lo hacía.
	reporting := app.Group("/api")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	reporting.Get("/ciclos/:cicloId/uso-equipos", autenticado, soloAdmin, h.ReporteUsoEquipos)
	reporting.Get("/ciclos/:cicloId/uso-docentes", autenticado, soloAdmin, h.ReporteUsoDocentes)
	reporting.Get("/historico/:anio/uso-equipos", autenticado, soloAdmin, h.HistoricoUsoEquipos)
	reporting.Get("/historico/:anio/uso-docentes", autenticado, soloAdmin, h.HistoricoUsoDocentes)

	// RF-06.3: no dependen del ciclo lectivo — Incidencia sobrevive al
	// archivado, así que siempre se resuelven en vivo.
	//
	// Van bajo "/incidencias/resumen/..." y no bajo "/incidencias/<algo>", que
	// es donde estaban: ahí chocaban con `GET /api/incidencias/:id`, que sirve
	// `inventory`. Fiber resuelve por orden de registro y inventory se registra
	// antes, así que "equipos" entraba como un id, no era un UUID, y las tres
	// contestaban 400 «el ID indicado no tiene un formato válido». La pestaña de
	// Reportes mostraba ese error y los reportes de incidencias no se ejecutaban
	// nunca.
	//
	// La colisión la creó aplanar los prefijos: antes esto vivía en
	// /api/reporting/incidencias/equipos y no se cruzaba con nada. El sistema ya
	// tenía tres casos así, resueltos registrando el literal ANTES del :id
	// dentro del mismo archivo (ver docs/06-arquitectura.md). Acá no alcanza:
	// las dos rutas viven en MÓDULOS distintos, así que el orden se decide en
	// cmd/main.go y no se ve desde ninguno de los dos routes.go. Un segmento más
	// hace que no haya nada que ordenar.
	reporting.Get("/incidencias/resumen/por-equipo", autenticado, soloAdmin, h.ReporteIncidenciasPorEquipo)
	reporting.Get("/incidencias/resumen/por-carro", autenticado, soloAdmin, h.ReporteIncidenciasPorCarro)
	reporting.Get("/incidencias/resumen/por-categoria", autenticado, soloAdmin, h.ReporteIncidenciasPorCategoria)

	// RF-06.5: el estado del parque HOY. No dependen del ciclo lectivo ni
	// aceptan rango de fechas — describen la situación actual, no un período.
	reporting.Get("/inventario/estado", autenticado, soloAdmin, h.ReporteEstadoDelInventario)
	reporting.Get("/inventario/fuera-de-circulacion", autenticado, soloAdmin, h.ReporteEquiposFueraDeCirculacion)
}
