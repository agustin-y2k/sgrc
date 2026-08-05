package http

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/middleware"
)

type Handler struct {
	svc     *application.Service
	auditor audit.Auditor
}

func NewHandler(svc *application.Service, auditor audit.Auditor) *Handler {
	return &Handler{svc: svc, auditor: auditor}
}

// auditar registra una entrada de auditoría sin abortar la respuesta HTTP
// si falla (ver internal/auth/interfaces/http.Handler.auditar).
func (h *Handler) auditar(c *fiber.Ctx, actorID, accion, entidad string, entidadID *string, detalle map[string]any) {
	if err := h.auditor.Registrar(c.UserContext(), audit.Entrada{
		UsuarioID: actorID,
		Accion:    accion,
		Entidad:   entidad,
		EntidadID: entidadID,
		Detalle:   detalle,
		IPOrigen:  middleware.IPCliente(c),
	}); err != nil {
		log.Printf("auditoría: no se pudo registrar %s sobre %s: %v", accion, entidad, err)
	}
}

// claimsDelContexto es el único punto donde un handler protegido lee el
// usuario autenticado — mismo patrón que internal/auth/interfaces/http.
func claimsDelContexto(c *fiber.Ctx) (*middleware.Claims, error) {
	claims := middleware.ClaimsFromCtx(c)
	if claims == nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "no autenticado")
	}
	return claims, nil
}

// ── Carro ───────────────────────────────────────────────────────────────

// POST /api/inventory/carros (Admin)
func (h *Handler) CrearCarro(c *fiber.Ctx) error {
	var req crearCarroRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	carro, err := h.svc.CrearCarro(c.UserContext(), req.Nombre, req.Descripcion)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toCarroResponse(carro))
}

// GET /api/inventory/carros (cualquier usuario autenticado)
func (h *Handler) ListarCarros(c *fiber.Ctx) error {
	carros, err := h.svc.ListarCarros(c.UserContext())
	if err != nil {
		return mapearError(err)
	}

	data := make([]carroResponse, len(carros))
	for i, ca := range carros {
		data[i] = toCarroResponse(ca)
	}
	return c.JSON(fiber.Map{"data": data})
}

// PATCH /api/inventory/carros/{id} (Admin)
func (h *Handler) EditarCarro(c *fiber.Ctx) error {
	id := c.Params("id")

	var req editarCarroRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	if err := h.svc.EditarCarro(c.UserContext(), id, req.Nombre, req.Descripcion); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusOK)
}

// ── PC ──────────────────────────────────────────────────────────────────

// POST /api/inventory/carros/{carroId}/pcs (Admin)
func (h *Handler) CrearPC(c *fiber.Ctx) error {
	carroID := c.Params("carroId")

	var req crearPCRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	pc, err := h.svc.CrearPC(c.UserContext(), carroID, req.Identificador, req.NumeroSerie, req.Freezado,
		req.CPU, req.RAM, req.SistemaOperativo, req.SoftwareInstalado)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toPCResponse(pc))
}

// GET /api/inventory/carros/{carroId}/pcs (cualquier usuario autenticado)
func (h *Handler) ListarPCsPorCarro(c *fiber.Ctx) error {
	carroID := c.Params("carroId")

	pcs, err := h.svc.ListarPCsPorCarro(c.UserContext(), carroID)
	if err != nil {
		return mapearError(err)
	}

	data := make([]pcResponse, len(pcs))
	for i, pc := range pcs {
		data[i] = toPCResponse(pc)
	}
	return c.JSON(fiber.Map{"data": data})
}

// PATCH /api/inventory/pcs/{id} (Admin) — datos + mover de carro (RF-03.10)
func (h *Handler) EditarPC(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req editarPCRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	params := application.EditarPCParams{
		CarroID: req.CarroID, Freezado: req.Freezado, CPU: req.CPU,
		RAM: req.RAM, SistemaOperativo: req.SistemaOperativo, SoftwareInstalado: req.SoftwareInstalado,
	}
	if err := h.svc.EditarPC(c.UserContext(), id, params); err != nil {
		return mapearError(err)
	}
	if req.CarroID != nil {
		h.auditar(c, claims.UserID, audit.PCMovidaDeCarro, "pc", &id, map[string]any{"carroDestinoId": *req.CarroID})
	}
	return c.SendStatus(fiber.StatusOK)
}

// PATCH /api/inventory/pcs/{id}/estado (Admin) — dispara cascada RF-03.8
func (h *Handler) CambiarEstadoPC(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req cambiarEstadoPCRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	nuevo, err := domain.ParseEstadoPC(req.Estado)
	if err != nil {
		return mapearError(err)
	}

	resultado, err := h.svc.CambiarEstadoPC(c.UserContext(), id, nuevo, req.Motivo)
	if err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.PCEstadoCambiado, "pc", &id, map[string]any{
		"nuevoEstado":        req.Estado,
		"reservasCanceladas": resultado.ReservasCanceladas,
	})
	return c.JSON(toCascadaResponse(resultado))
}

// DELETE /api/inventory/pcs/{id} (Admin) — soft delete, dispara la misma
// cascada que FUERA_DE_SERVICIO (RF-03.9)
func (h *Handler) DarDeBajaPC(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	resultado, err := h.svc.DarDeBajaPC(c.UserContext(), id)
	if err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.PCDadaDeBaja, "pc", &id, map[string]any{
		"reservasCanceladas": resultado.ReservasCanceladas,
	})
	return c.JSON(toCascadaResponse(resultado))
}

// ── Incidencia ──────────────────────────────────────────────────────────

// POST /api/inventory/incidencias (cualquier usuario autenticado — un
// docente también puede reportar una falla, RF-03.5)
func (h *Handler) CrearIncidencia(c *fiber.Ctx) error {
	var req crearIncidenciaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	gravedad, err := domain.ParseGravedad(req.Gravedad)
	if err != nil {
		return mapearError(err)
	}

	claims, errClaims := claimsDelContexto(c)
	if errClaims != nil {
		return errClaims
	}

	i, err := h.svc.CrearIncidencia(c.UserContext(), req.PCID, claims.UserID, req.Descripcion, gravedad)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toIncidenciaResponse(i))
}

// GET /api/inventory/pcs/{pcId}/incidencias (cualquier usuario autenticado)
func (h *Handler) ListarIncidenciasPorPC(c *fiber.Ctx) error {
	pcID := c.Params("pcId")

	incidencias, err := h.svc.ListarIncidenciasPorPC(c.UserContext(), pcID)
	if err != nil {
		return mapearError(err)
	}

	data := make([]incidenciaResponse, len(incidencias))
	for i, inc := range incidencias {
		data[i] = toIncidenciaResponse(inc)
	}
	return c.JSON(fiber.Map{"data": data})
}

// PATCH /api/inventory/incidencias/{id} (Admin)
func (h *Handler) EditarIncidencia(c *fiber.Ctx) error {
	id := c.Params("id")

	var req editarIncidenciaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	params := application.EditarIncidenciaParams{MarcarEnviadaDGE: req.MarcarEnviadaDGE}
	if req.Estado != nil {
		estado, err := domain.ParseEstadoIncidencia(*req.Estado)
		if err != nil {
			return mapearError(err)
		}
		params.Estado = &estado
	}

	if err := h.svc.EditarIncidencia(c.UserContext(), id, params); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusOK)
}
