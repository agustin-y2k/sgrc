package http

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/middleware"
	"github.com/ramiro/sgrc/internal/shared/respuesta"
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

// POST /api/carros (Admin)
func (h *Handler) CrearCarro(c *fiber.Ctx) error {
	var req crearCarroRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	carro, err := h.svc.CrearCarro(c.UserContext(), req.Nombre, req.Descripcion)
	if err != nil {
		return mapearError(err)
	}
	return respuesta.Creado(c, "/api/carros", carro.ID, toCarroResponse(carro))
}

// GET /api/carros (cualquier usuario autenticado)
//
// `?incluirRetirados=true` suma los carros dados de baja, y es sólo para el
// Admin: el resto no tiene nada que hacer con un carro que ya no existe, y el
// selector de "dónde va este equipo" se rompería si los ofreciera.
func (h *Handler) ListarCarros(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}
	incluirRetirados := c.Query("incluirRetirados") == "true" && claims.Rol == "ADMIN"

	carros, err := h.svc.ListarCarros(c.UserContext(), incluirRetirados)
	if err != nil {
		return mapearError(err)
	}

	data := make([]carroResponse, len(carros))
	for i, ca := range carros {
		data[i] = toCarroResponse(ca)
	}
	return c.JSON(fiber.Map{"data": data})
}

// PATCH /api/carros/{id} (Admin)
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

// POST /api/carros/{carroId}/equipos (Admin)
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
	return respuesta.Creado(c, "/api/equipos", pc.ID, toEquipoResponse(pc))
}

// GET /api/carros/{carroId}/equipos (cualquier usuario autenticado)
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

// PATCH /api/equipos/{id} (Admin) — datos + mover de carro (RF-03.10)
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
	// Ver el comentario de editarEquipoRequest.Estado: antes esto pasaba con
	// 200 sin hacer nada.
	if req.Estado != nil {
		return fiber.NewError(fiber.StatusBadRequest,
			"el estado del equipo no se cambia por acá: usá PATCH /api/equipos/{id}/estado, "+
				"que es el que cancela las reservas que quedan sin máquina")
	}

	params := application.EditarEquipoParams{
		CarroID: req.CarroID, Freezado: req.Freezado, CPU: req.CPU,
		RAM: req.RAM, SistemaOperativo: req.SistemaOperativo, SoftwareInstalado: req.SoftwareInstalado,
		Tipo: req.Tipo, Nombre: req.Nombre, Reservable: req.Reservable,
		EsComputadora: req.EsComputadora, NumeroSerie: req.NumeroSerie,
	}
	cambios, err := h.svc.EditarEquipo(c.UserContext(), id, params)
	if err != nil {
		return mapearError(err)
	}
	if req.CarroID != nil {
		h.auditar(c, claims.UserID, audit.EquipoMovidoDeCarro, "pc", &id, map[string]any{"carroDestinoId": *req.CarroID})
	}
	// Todo lo demás que se haya movido, con su valor anterior y el nuevo. Sólo
	// si de verdad cambió algo: un PATCH que manda los mismos datos no es una
	// edición, y una entrada que dice «editó» sin decir qué es ruido.
	if len(cambios) > 0 {
		h.auditar(c, claims.UserID, audit.EquipoEditado, "pc", &id, detalleDeCambios(cambios))
	}
	return c.SendStatus(fiber.StatusOK)
}

// PATCH /api/equipos/{id}/estado (Admin) — dispara cascada RF-03.8
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

// DELETE /api/equipos/{id} (Admin) — soft delete, dispara la misma
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

// POST /api/equipos/{id}/reactivar (Admin) — deshace la baja.
//
// No devuelve ninguna cascada porque no hay ninguna que deshacer: las reservas
// que la baja canceló quedan canceladas y los avisos ya salieron. Lo que vuelve
// es la máquina al inventario, nada más.
func (h *Handler) ReactivarEquipo(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	if err := h.svc.ReactivarEquipo(c.UserContext(), id); err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.EquipoReactivado, "pc", &id, nil)
	return c.SendStatus(fiber.StatusOK)
}

// DELETE /api/carros/{id} (Admin) — baja LÓGICA del carro.
//
// Se audita porque la baja libera el nombre: sin esta entrada no quedaría
// rastro de que el «Carro 1» de hoy no es el «Carro 1» del año pasado.
func (h *Handler) DarDeBajaCarro(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	if err := h.svc.DarDeBajaCarro(c.UserContext(), id); err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.CarroDadoDeBaja, "carro", &id, nil)
	return c.SendStatus(fiber.StatusOK)
}

// POST /api/carros/{id}/reactivar (Admin) — deshace la baja.
//
// Se audita por lo mismo que la baja: el nombre se libera al retirar el carro y
// se vuelve a tomar al reactivarlo, así que las dos entradas juntas son lo único
// que explica por qué el «Carro 1» estuvo un tiempo disponible para otro.
func (h *Handler) ReactivarCarro(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	if err := h.svc.ReactivarCarro(c.UserContext(), id); err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.CarroReactivado, "carro", &id, nil)
	return c.SendStatus(fiber.StatusOK)
}

// GET /api/preferencias/huerfanas (Admin) — las marcas que ya no
// cruzan con ninguna materia.
//
// Existe porque el vínculo marca-materia es por nombre y no por referencia
// (RF-03.21): renombrar o borrar una materia deja la marca apuntando a un
// nombre que ya no está, y hasta acá eso era invisible — la marca seguía
// mostrándose en la ficha del equipo igual que una que sí aplica.
func (h *Handler) ListarPreferenciasHuerfanas(c *fiber.Ctx) error {
	huerfanas, err := h.svc.ListarPreferenciasHuerfanas(c.UserContext())
	if err != nil {
		return mapearError(err)
	}

	data := make([]preferenciaHuerfanaResponse, len(huerfanas))
	for i, p := range huerfanas {
		data[i] = toPreferenciaHuerfanaResponse(p)
	}
	return c.JSON(fiber.Map{"data": data})
}

// ── Incidencia ──────────────────────────────────────────────────────────

// POST /api/incidencias (cualquier usuario autenticado — un
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
	return respuesta.Creado(c, "/api/incidencias", i.ID, toIncidenciaResponse(i))
}

// GET /api/equipos/{equipoId}/incidencias (cualquier usuario autenticado)
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

// GET /api/categorias-de-falla (cualquier autenticado) Las
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

// PATCH /api/incidencias/{id} (Admin)
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

// POST /api/equipos (Admin) — un proyector, un cargador, una
// notebook suelta.
func (h *Handler) CrearEquipo(c *fiber.Ctx) error {
	var req crearEquipoSueltoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	equipo, err := h.svc.CrearEquipo(c.UserContext(), application.CrearEquipoSueltoParams{
		Tipo: req.Tipo, Nombre: req.Nombre, NumeroSerie: req.NumeroSerie, Reservable: req.Reservable,
		EsComputadora: req.EsComputadora, Freezado: req.Freezado, CPU: req.CPU, RAM: req.RAM,
		SistemaOperativo: req.SistemaOperativo, SoftwareInstalado: req.SoftwareInstalado,
	})
	if err != nil {
		return mapearError(err)
	}
	return respuesta.Creado(c, "/api/equipos", equipo.ID, toEquipoResponse(equipo))
}

// GET /api/equipos (cualquier autenticado) — todo el inventario.
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

// ── GET de un recurso solo ──────────────────────────────────────────────
//
// Ver el bloque «Obtener uno solo» de application/service.go: estos recursos
// se podían editar y borrar sin poder pedirlos. El permiso de cada uno es el
// de su listado.

// GET /api/carros/{id} (cualquier usuario autenticado)
func (h *Handler) ObtenerCarro(c *fiber.Ctx) error {
	carro, err := h.svc.ObtenerCarro(c.UserContext(), c.Params("id"))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toCarroResponse(carro))
}

// GET /api/equipos/{id} (cualquier usuario autenticado)
func (h *Handler) ObtenerEquipo(c *fiber.Ctx) error {
	equipo, err := h.svc.ObtenerEquipo(c.UserContext(), c.Params("id"))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toEquipoResponse(equipo))
}

// GET /api/incidencias/{id} (cualquier usuario autenticado — quien reporta
// una falla puede seguir qué pasó con ella).
func (h *Handler) ObtenerIncidencia(c *fiber.Ctx) error {
	i, err := h.svc.ObtenerIncidencia(c.UserContext(), c.Params("id"))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toIncidenciaResponse(i))
}

// GET /api/licencias/{id} (Admin, igual que el listado)
func (h *Handler) ObtenerLicencia(c *fiber.Ctx) error {
	l, err := h.svc.ObtenerLicencia(c.UserContext(), c.Params("id"))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toLicenciaResponse(l, h.svc.Hoy()))
}

// GET /api/preferencias/{id} (cualquier usuario autenticado, igual que el
// listado por equipo)
func (h *Handler) ObtenerPreferencia(c *fiber.Ctx) error {
	p, err := h.svc.ObtenerPreferencia(c.UserContext(), c.Params("id"))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toPreferenciaResponse(p))
}

// GET /api/cuentas/{id} (cualquier usuario autenticado) — la cuenta, nunca su
// contraseña. Para eso está POST /cuentas/{id}/password, que además audita.
func (h *Handler) ObtenerCuentaDeEquipo(c *fiber.Ctx) error {
	cuenta, err := h.svc.ObtenerCuentaDeEquipo(c.UserContext(), c.Params("id"), esAdmin(c))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toCuentaResponse(cuenta))
}

// detalleDeCambios lleva el mapa del servicio a la forma que guarda la
// auditoría, que es JSON suelto.
func detalleDeCambios(cambios map[string]application.CambioDeCampo) map[string]any {
	detalle := make(map[string]any, len(cambios))
	for campo, c := range cambios {
		detalle[campo] = map[string]any{"antes": c.Antes, "despues": c.Despues}
	}
	return detalle
}
