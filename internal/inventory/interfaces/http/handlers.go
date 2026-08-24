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

// POST /api/inventory/carros/{carroId}/equipos (Admin)
func (h *Handler) CrearEquipoDeCarro(c *fiber.Ctx) error {
	carroID := c.Params("carroId")

	var req crearEquipoDeCarroRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	pc, err := h.svc.CrearEquipoDeCarro(c.UserContext(), carroID, req.Identificador, req.NumeroSerie, req.Freezado,
		req.CPU, req.RAM, req.SistemaOperativo, req.SoftwareInstalado)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toEquipoResponse(pc))
}

// GET /api/inventory/carros/{carroId}/equipos (cualquier usuario autenticado)
func (h *Handler) ListarEquiposPorCarro(c *fiber.Ctx) error {
	carroID := c.Params("carroId")

	equipos, err := h.svc.ListarEquiposPorCarro(c.UserContext(), carroID)
	if err != nil {
		return mapearError(err)
	}

	data := make([]equipoResponse, len(equipos))
	for i, equipo := range equipos {
		data[i] = toEquipoResponse(equipo)
	}
	return c.JSON(fiber.Map{"data": data})
}

// PATCH /api/inventory/equipos/{id} (Admin) — datos + mover de carro (RF-03.10)
func (h *Handler) EditarEquipo(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req editarEquipoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	params := application.EditarEquipoParams{
		CarroID: req.CarroID, Freezado: req.Freezado, CPU: req.CPU,
		RAM: req.RAM, SistemaOperativo: req.SistemaOperativo, SoftwareInstalado: req.SoftwareInstalado,
		Tipo: req.Tipo, Nombre: req.Nombre, Reservable: req.Reservable,
		NumeroSerie: req.NumeroSerie,
	}
	if err := h.svc.EditarEquipo(c.UserContext(), id, params); err != nil {
		return mapearError(err)
	}
	if req.CarroID != nil {
		h.auditar(c, claims.UserID, audit.EquipoMovidoDeCarro, "pc", &id, map[string]any{"carroDestinoId": *req.CarroID})
	}
	return c.SendStatus(fiber.StatusOK)
}

// PATCH /api/inventory/equipos/{id}/estado (Admin) — dispara cascada RF-03.8
func (h *Handler) CambiarEstadoEquipo(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req cambiarEstadoEquipoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	nuevo, err := domain.ParseEstadoEquipo(req.Estado)
	if err != nil {
		return mapearError(err)
	}

	resultado, err := h.svc.CambiarEstadoEquipo(c.UserContext(), id, nuevo, req.Motivo)
	if err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.EquipoEstadoCambiado, "pc", &id, map[string]any{
		"nuevoEstado":        req.Estado,
		"reservasCanceladas": resultado.ReservasCanceladas,
	})
	return c.JSON(toCascadaResponse(resultado))
}

// DELETE /api/inventory/equipos/{id} (Admin) — soft delete, dispara la misma
// cascada que FUERA_DE_SERVICIO (RF-03.9)
func (h *Handler) DarDeBajaEquipo(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	resultado, err := h.svc.DarDeBajaEquipo(c.UserContext(), id)
	if err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.EquipoDadoDeBaja, "pc", &id, map[string]any{
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

	i, err := h.svc.CrearIncidencia(c.UserContext(), req.EquipoID, claims.UserID, req.Descripcion, req.Categoria, gravedad)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toIncidenciaResponse(i))
}

// GET /api/inventory/equipos/{equipoId}/incidencias (cualquier usuario autenticado)
func (h *Handler) ListarIncidenciasPorEquipo(c *fiber.Ctx) error {
	equipoID := c.Params("equipoId")

	incidencias, err := h.svc.ListarIncidenciasPorEquipo(c.UserContext(), equipoID)
	if err != nil {
		return mapearError(err)
	}

	data := make([]incidenciaResponse, len(incidencias))
	for i, inc := range incidencias {
		data[i] = toIncidenciaResponse(inc)
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/inventory/categorias-de-falla (cualquier autenticado) Las
// categorías de falla ya usadas, para sugerirlas al reportar una nueva.
func (h *Handler) ListarCategoriasDeFalla(c *fiber.Ctx) error {
	categorias, err := h.svc.CategoriasDeFallaUsadas(c.UserContext())
	if err != nil {
		return mapearError(err)
	}
	if categorias == nil {
		categorias = []string{}
	}
	return c.JSON(fiber.Map{"data": categorias})
}

// PATCH /api/inventory/incidencias/{id} (Admin)
func (h *Handler) EditarIncidencia(c *fiber.Ctx) error {
	id := c.Params("id")

	var req editarIncidenciaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	params := application.EditarIncidenciaParams{
		MarcarEnviadaASoporte: req.MarcarEnviadaASoporte,
		Categoria:             req.Categoria,
	}
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

// ── Equipos que no están en ningún carro (RF-03.15) ─────────────────────

// POST /api/inventory/equipos (Admin) — un proyector, un cargador, una
// notebook suelta.
func (h *Handler) CrearEquipo(c *fiber.Ctx) error {
	var req crearEquipoSueltoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	equipo, err := h.svc.CrearEquipo(c.UserContext(), req.Tipo, req.Nombre, req.NumeroSerie, req.Reservable)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toEquipoResponse(equipo))
}

// GET /api/inventory/equipos (cualquier autenticado) — todo el inventario.
func (h *Handler) ListarEquipos(c *fiber.Ctx) error {
	// Solo se reconoce el valor exacto "false".
	var soloSueltos bool
	if c.Query("enCarro") == "false" {
		soloSueltos = true
	}

	equipos, err := h.svc.ListarEquipos(c.UserContext(), soloSueltos)
	if err != nil {
		return mapearError(err)
	}

	data := make([]equipoResponse, len(equipos))
	for i, e := range equipos {
		data[i] = toEquipoResponse(e)
	}
	return c.JSON(fiber.Map{"data": data})
}
