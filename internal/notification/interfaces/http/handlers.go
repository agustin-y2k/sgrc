package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/notification/application"
	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/middleware"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

type Handler struct {
	svc *application.Service
}

func NewHandler(svc *application.Service) *Handler {
	return &Handler{svc: svc}
}

func claimsDelContexto(c *fiber.Ctx) (*middleware.Claims, error) {
	claims := middleware.ClaimsFromCtx(c)
	if claims == nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "no autenticado")
	}
	return claims, nil
}

// GET /api/notifications — las propias del usuario autenticado, con
// filtro opcional ?estado=NO_LEIDA|LEIDA.
func (h *Handler) ListarPropias(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var filtroEstado *domain.Estado
	if v := c.Query("estado"); v != "" {
		e, err := domain.ParseEstado(v)
		if err != nil {
			return mapearError(err)
		}
		filtroEstado = &e
	}

	pagina, err := paginacion.Parsear(c.Query("page"), c.Query("pageSize"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	notificaciones, total, err := h.svc.ListarPorUsuario(c.UserContext(), claims.UserID, filtroEstado, pagina)
	if err != nil {
		return mapearError(err)
	}

	data := make([]notificacionResponse, len(notificaciones))
	for i, n := range notificaciones {
		data[i] = toNotificacionResponse(n)
	}
	return c.JSON(listarNotificacionesResponse{Data: data, Meta: pagina.Meta(total)})
}

// PATCH /api/notifications/{id}/leida — solo el dueño puede marcarla como
// leída (no hay excepción de Admin acá: no tiene sentido que un Admin marque
// como leída la notificación de otra persona).
func (h *Handler) MarcarLeida(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	n, err := h.svc.ObtenerNotificacion(c.UserContext(), id)
	if err != nil {
		return mapearError(err)
	}
	if n.UsuarioID != claims.UserID {
		return fiber.NewError(fiber.StatusForbidden, "no podés marcar como leída una notificación que no es tuya")
	}

	if err := h.svc.MarcarLeida(c.UserContext(), id); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusOK)
}

// POST /api/notifications/leer-todas — RF-05.7. Marca como leídas todas
// las notificaciones sin leer del usuario autenticado.
func (h *Handler) MarcarTodasLeidas(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	n, err := h.svc.MarcarTodasLeidas(c.UserContext(), claims.UserID)
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(fiber.Map{"marcadas": n})
}

// GET /api/notifications/preferencias-email — RF-05.13. Las categorías que le
// corresponden a quien pregunta, con el estado de cada una.
func (h *Handler) ListarPreferenciasEmail(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	activas, err := h.svc.CategoriasDeEmail(c.UserContext(), claims.UserID, esAdmin(claims))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toPreferenciasEmailResponse(activas, esAdmin(claims)))
}

// PUT /api/notifications/preferencias-email — reemplaza la selección entera.
// No es PATCH por casilla a propósito: el panel se guarda como se ve, y dos
// pestañas abiertas no pueden dejar una mezcla que ninguna de las dos mostró.
func (h *Handler) GuardarPreferenciasEmail(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req guardarPreferenciasEmailRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "el cuerpo del pedido no es un JSON válido")
	}

	categorias := make([]domain.CategoriaEmail, 0, len(req.Categorias))
	for _, s := range req.Categorias {
		categoria, err := domain.ParseCategoriaEmail(s)
		if err != nil {
			return mapearError(err)
		}
		// Las fijas salen siempre: la casilla se muestra tildada y sin poder
		// tocarse, así que un pedido que las nombre viene de un cliente que no
		// está mirando el panel.
		if categoria.EsFija() {
			return fiber.NewError(fiber.StatusBadRequest,
				"ese correo sale siempre y no se puede desactivar")
		}
		// Un docente no puede encender un correo que no recibe: sería una fila
		// que no hace nada y una casilla que no vio nunca.
		if !categoria.PuedeElegir(esAdmin(claims)) {
			return fiber.NewError(fiber.StatusForbidden,
				"esa categoría es de los avisos que van a los administradores")
		}
		categorias = append(categorias, categoria)
	}

	activas, err := h.svc.GuardarCategoriasDeEmail(c.UserContext(), claims.UserID, categorias, esAdmin(claims))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toPreferenciasEmailResponse(activas, esAdmin(claims)))
}

func esAdmin(claims *middleware.Claims) bool {
	return claims.Rol == "ADMIN"
}
