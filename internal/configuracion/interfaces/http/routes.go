package http

import (
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/agustin-y2k/sgrc/internal/configuracion/application"
	"github.com/agustin-y2k/sgrc/internal/configuracion/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/middleware"
)

func RegisterRoutes(app *fiber.App, s *application.Service, aut middleware.Autenticacion) {
	g := app.Group("/api/configuracion/operacion", aut.Requerida(), middleware.RequireRol("ADMIN"))
	g.Get("/", func(c *fiber.Ctx) error {
		o, err := s.Leer(c.UserContext())
		if err != nil {
			return fiber.NewError(503, "no se pudo consultar la configuración")
		}
		return c.JSON(o)
	})
	g.Put("/", func(c *fiber.Ctx) error {
		var o domain.Operacion
		if err := c.BodyParser(&o); err != nil {
			return fiber.NewError(400, "configuración inválida")
		}
		var presentes map[string]json.RawMessage
		if err := json.Unmarshal(c.Body(), &presentes); err != nil {
			return fiber.NewError(400, "configuración inválida")
		}
		for _, nombre := range []string{"retiroGraciaMinutos", "recordatorioAntelacionMinutos", "cierreGraciaMinutos", "cierreHoraSinJornada", "licenciasHoraAviso", "version"} {
			if valor, ok := presentes[nombre]; !ok || string(valor) == "null" {
				return fiber.NewError(400, "faltan valores de configuración: "+nombre)
			}
		}
		if err := o.Validar(); err != nil {
			return fiber.NewError(400, err.Error())
		}
		claims := middleware.ClaimsFromCtx(c)
		if claims == nil {
			return fiber.ErrUnauthorized
		}
		resultado, err := s.Guardar(c.UserContext(), o, claims.UserID, middleware.IPCliente(c))
		if errors.Is(err, application.ErrDesactualizada) {
			return fiber.NewError(409, err.Error())
		}
		if err != nil {
			return fiber.NewError(503, "no se pudo guardar la configuración; no se aplicaron cambios")
		}
		return c.JSON(resultado)
	})
}
