package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta las rutas de reporting bajo /api/reporting — todas
// exclusivas de Admin (RF-06 es una funcionalidad de gestión, no algo que
// un docente necesite consultar sobre otros).
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	reporting := app.Group("/api/reporting")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	reporting.Get("/ciclos/:cicloId/uso-equipos", autenticado, soloAdmin, h.ReporteUsoEquipos)
	reporting.Get("/ciclos/:cicloId/uso-docentes", autenticado, soloAdmin, h.ReporteUsoDocentes)
	reporting.Get("/historico/:anio/uso-equipos", autenticado, soloAdmin, h.HistoricoUsoEquipos)
	reporting.Get("/historico/:anio/uso-docentes", autenticado, soloAdmin, h.HistoricoUsoDocentes)

	// RF-06.3: no dependen del ciclo lectivo — Incidencia sobrevive al
	// archivado, así que siempre se resuelven en vivo.
	reporting.Get("/incidencias/equipos", autenticado, soloAdmin, h.ReporteIncidenciasPorEquipo)
	reporting.Get("/incidencias/carros", autenticado, soloAdmin, h.ReporteIncidenciasPorCarro)
}
