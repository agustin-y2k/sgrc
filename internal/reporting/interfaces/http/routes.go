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

	reporting.Get("/ciclos/:cicloId/uso-pcs", autenticado, soloAdmin, h.ReporteUsoPCs)
	reporting.Get("/ciclos/:cicloId/uso-docentes", autenticado, soloAdmin, h.ReporteUsoDocentes)
	reporting.Get("/historico/:anio/uso-pcs", autenticado, soloAdmin, h.HistoricoUsoPCs)
	reporting.Get("/historico/:anio/uso-docentes", autenticado, soloAdmin, h.HistoricoUsoDocentes)

	// RF-06.3: no dependen del ciclo lectivo — Incidencia sobrevive al
	// archivado, así que siempre se resuelven en vivo.
	reporting.Get("/incidencias/pcs", autenticado, soloAdmin, h.ReporteIncidenciasPorPC)
	reporting.Get("/incidencias/carros", autenticado, soloAdmin, h.ReporteIncidenciasPorCarro)
}
