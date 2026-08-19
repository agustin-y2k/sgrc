package http

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/academic/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
)

// Los pedidos para dictar una materia (ver domain.PedidoDeMateria).

// POST /api/academic/pedidos-de-materia — lo hace un docente.
func (h *Handler) PedirMateria(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req pedirMateriaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	var materiaID *string
	if strings.TrimSpace(req.MateriaID) != "" {
		id := strings.TrimSpace(req.MateriaID)
		materiaID = &id
	}

	p, err := h.svc.PedirMateria(c.UserContext(), claims.UserID, materiaID,
		req.CursoSolicitado, req.MateriaSolicitada, req.Motivo)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toPedidoResponse(p))
}

// GET /api/academic/pedidos-de-materia/mios — los propios, con su estado.
func (h *Handler) MisPedidosDeMateria(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	pedidos, err := h.svc.ListarMisPedidos(c.UserContext(), claims.UserID)
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(fiber.Map{"data": aRespuestas(pedidos)})
}

// GET /api/academic/pedidos-de-materia (Admin) — `?pendientes=true` deja
// solo lo que falta resolver.
func (h *Handler) ListarPedidosDeMateria(c *fiber.Ctx) error {
	pedidos, err := h.svc.ListarPedidos(c.UserContext(), c.Query("pendientes") == "true")
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(fiber.Map{"data": aRespuestas(pedidos)})
}

// POST /api/academic/pedidos-de-materia/{id}/resolver (Admin).
func (h *Handler) ResolverPedidoDeMateria(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req resolverPedidoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	// Sin rol explícito lo decide el servicio según la materia (ver
	// rolPorDefecto en service_pedidos.go): arrogarle la titularidad a alguien
	// que se suma a una materia que ya da otro es un dato mal cargado, y el rol
	// es justamente el campo que después alguien lee para saber quién es quién.
	var rol *domain.RolDocente
	if strings.TrimSpace(req.Rol) != "" {
		parseado, err := domain.ParseRolDocente(req.Rol)
		if err != nil {
			return mapearError(err)
		}
		rol = &parseado
	}

	var cursoID *string
	if strings.TrimSpace(req.CursoID) != "" {
		id := strings.TrimSpace(req.CursoID)
		cursoID = &id
	}

	id := c.Params("id")
	p, err := h.svc.ResolverPedido(c.UserContext(), id, claims.UserID, req.Aprobar, req.Respuesta, cursoID, rol)
	if err != nil {
		return mapearError(err)
	}

	accion := audit.PedidoDeMateriaRechazado
	if req.Aprobar {
		accion = audit.PedidoDeMateriaAprobado
	}
	h.auditar(c, claims.UserID, accion, "pedido_de_materia", &id, map[string]any{
		"solicitante": p.UsuarioID,
	})
	return c.JSON(toPedidoResponse(p))
}

func aRespuestas(pedidos []*domain.PedidoDeMateria) []pedidoResponse {
	data := make([]pedidoResponse, len(pedidos))
	for i, p := range pedidos {
		data[i] = toPedidoResponse(p)
	}
	return data
}
