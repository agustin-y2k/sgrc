package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta la consulta del registro bajo /api/auditoria.
//
// Sólo Admin, y sólo lectura: no hay ruta que escriba ni que borre. El registro
// se escribe desde internal/shared/audit, al que no se llega por HTTP.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	auditoria := app.Group("/api/auditoria")

	autenticado := aut.Requerida()
	soloAdmin := middleware.RequireRol("ADMIN")

	auditoria.Get("/", autenticado, soloAdmin, h.Listar)
	auditoria.Get("/opciones", autenticado, soloAdmin, h.Opciones)
}
