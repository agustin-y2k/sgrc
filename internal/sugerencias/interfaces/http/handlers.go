package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
	"github.com/ramiro/sgrc/internal/sugerencias/application"
)

type Handler struct {
	svc *application.Service
}

func NewHandler(svc *application.Service) *Handler { return &Handler{svc: svc} }

func claimsDelContexto(c *fiber.Ctx) (*middleware.Claims, error) {
	claims := middleware.ClaimsFromCtx(c)
	if claims == nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "no autenticado")
	}
	return claims, nil
}

// POST /api/sugerencias — cualquiera que use el sistema puede escribir.
func (h *Handler) Escribir(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req escribirRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	s, err := h.svc.Escribir(c.UserContext(), claims.UserID, req.Tipo, req.Asunto, req.Texto, req.Pantalla, req.Version)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toSugerenciaResponse(s, false))
}

// GET /api/sugerencias/mias — lo que escribí yo, con las respuestas.
func (h *Handler) ListarPropias(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	pagina, err := paginacion.Parsear(c.Query("page"), c.Query("pageSize"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	sugerencias, total, err := h.svc.ListarPropias(c.UserContext(), claims.UserID, pagina)
	if err != nil {
		return mapearError(err)
	}

	data := make([]sugerenciaResponse, len(sugerencias))
	for i, s := range sugerencias {
		data[i] = toSugerenciaResponse(s, false)
	}
	return c.JSON(fiber.Map{"data": data, "meta": pagina.Meta(total)})
}

// GET /api/sugerencias (Admin) — todo el buzón. `?abiertas=true` deja solo
// lo que falta contestar, que es como se mira casi siempre.
func (h *Handler) Listar(c *fiber.Ctx) error {
	pagina, err := paginacion.Parsear(c.Query("page"), c.Query("pageSize"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	sugerencias, total, err := h.svc.ListarTodas(c.UserContext(), c.Query("abiertas") == "true", pagina)
	if err != nil {
		return mapearError(err)
	}

	data := make([]sugerenciaResponse, len(sugerencias))
	for i, s := range sugerencias {
		data[i] = toSugerenciaResponse(s, true)
	}
	return c.JSON(fiber.Map{"data": data, "meta": pagina.Meta(total)})
}

// POST /api/sugerencias/{id}/mensajes — escribe en el hilo. Lo usan los dos
// lados: un Admin en cualquier conversación, y quien preguntó solo en la
// suya (lo verifica el servicio).
func (h *Handler) Responder(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req responderRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	esAdmin := claims.Rol == "ADMIN"
	s, err := h.svc.Responder(c.UserContext(), c.Params("id"), claims.UserID, esAdmin, req.Texto)
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toSugerenciaResponse(s, esAdmin))
}

// POST /api/sugerencias/{id}/resolver (Admin) — da el tema por terminado.
// Es un acto aparte de contestar: se contesta muchas veces y se cierra una.
func (h *Handler) Resolver(c *fiber.Ctx) error {
	s, err := h.svc.MarcarResuelta(c.UserContext(), c.Params("id"))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toSugerenciaResponse(s, true))
}
