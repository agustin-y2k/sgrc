package http

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/auth/application"
	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/middleware"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// Handler agrupa todos los endpoints de auth — un solo Service inyectado,
// cero acceso directo a Postgres ni a argon2/JWT desde acá (eso ya está
// resuelto en application/ e infrastructure/).
type Handler struct {
	svc     *application.Service
	auditor audit.Auditor
	// googleClientID solo se usa para publicarlo en GET /api/auth/config,
	// que es de donde el frontend lo lee para dibujar el botón de Google.
	// Vacío = este despliegue no tiene el ingreso con Google configurado.
	googleClientID string
}

func NewHandler(svc *application.Service, auditor audit.Auditor, googleClientID string) *Handler {
	return &Handler{svc: svc, auditor: auditor, googleClientID: googleClientID}
}

// auditar registra una entrada de auditoría sin abortar la respuesta HTTP
// si falla — un fallo de accountability no debe deshacer una acción de
// negocio que ya se ejecutó con éxito (ver docs/09-seguridad-rbac.md §5).
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
// usuario autenticado — si JWTAuth no corrió antes (error de wiring en
// routes.go), esto responde 401 en vez de panickear con un nil pointer.
func claimsDelContexto(c *fiber.Ctx) (*middleware.Claims, error) {
	claims := middleware.ClaimsFromCtx(c)
	if claims == nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "no autenticado")
	}
	return claims, nil
}

// POST /api/auth/registro
func (h *Handler) Registrar(c *fiber.Ctx) error {
	var req registroRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	_, err := h.svc.Registrar(c.UserContext(), req.Nombre, req.Apellido, req.Email, req.Password,
		application.SolicitudDeAsignacion{Curso: req.CursoSolicitado, Materia: req.MateriaSolicitada})
	if err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusCreated)
}

// POST /api/auth/login
func (h *Handler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	res, err := h.svc.Login(c.UserContext(), req.Email, req.Password)
	if err != nil {
		return mapearError(err)
	}

	return c.JSON(loginResponse{
		Token:               res.Token,
		DebeCambiarPassword: res.DebeCambiarPassword,
	})
}

// GET /api/auth/config — pública, sin autenticar.
//
// Es lo que la pantalla de login consulta antes de dibujarse para saber si
// este despliegue tiene ingreso con Google. No expone nada sensible: el
// client ID es público por definición (viaja al navegador en cada pedido
// que este le hace a Google).
func (h *Handler) Config(c *fiber.Ctx) error {
	return c.JSON(configPublicaResponse{GoogleClientID: h.googleClientID})
}

// POST /api/auth/google — ingreso con una cuenta de Google ya registrada.
//
// Devuelve 404 cuando el token es válido pero no hay ninguna cuenta con
// ese email. Eso no es un error del cliente: es el camino normal la
// primera vez, y es lo que le indica al frontend que tiene que llevar a la
// persona a completar el registro (POST /api/auth/google/registro).
func (h *Handler) LoginConGoogle(c *fiber.Ctx) error {
	var req googleLoginRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	res, err := h.svc.LoginConGoogle(c.UserContext(), req.Credential)
	if err != nil {
		return mapearError(err)
	}

	return c.JSON(loginResponse{
		Token:               res.Token,
		DebeCambiarPassword: res.DebeCambiarPassword,
	})
}

// POST /api/auth/google/registro — autorregistro con cuenta de Google.
//
// Igual que el registro con contraseña: la cuenta queda PENDIENTE hasta
// que un Admin la apruebe (RF-01.3). Tener una cuenta de Google válida
// prueba quién sos, no que la escuela te conozca.
func (h *Handler) RegistrarConGoogle(c *fiber.Ctx) error {
	var req googleRegistroRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	_, err := h.svc.RegistrarConGoogle(c.UserContext(), req.Credential, req.Nombre, req.Apellido,
		application.SolicitudDeAsignacion{Curso: req.CursoSolicitado, Materia: req.MateriaSolicitada})
	if err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusCreated)
}

// GET /api/auth/me
func (h *Handler) Me(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	u, err := h.svc.ObtenerPerfil(c.UserContext(), claims.UserID)
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toUsuarioResponse(u))
}

// POST /api/auth/cambiar-password
func (h *Handler) CambiarPassword(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req cambiarPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	token, err := h.svc.CambiarPassword(c.UserContext(), claims.UserID, req.PasswordActual, req.PasswordNueva)
	if err != nil {
		return mapearError(err)
	}
	// El token viejo sigue diciendo debeCambiarPassword=true, así que el
	// cliente tiene que reemplazarlo por este o quedaría bloqueado por su
	// propio cambio exitoso (RF-01.6).
	return c.JSON(cambiarPasswordResponse{Token: token})
}

// GET /api/auth/usuarios (Admin)
func (h *Handler) ListarUsuarios(c *fiber.Ctx) error {
	var filtroEstado *domain.Estado
	if v := c.Query("estado"); v != "" {
		e, err := domain.ParseEstado(v)
		if err != nil {
			return mapearError(err)
		}
		filtroEstado = &e
	}

	var filtroRol *domain.Rol
	if v := c.Query("rol"); v != "" {
		r, err := domain.ParseRol(v)
		if err != nil {
			return mapearError(err)
		}
		filtroRol = &r
	}

	pagina, err := paginacion.Parsear(c.Query("page"), c.Query("pageSize"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	usuarios, total, err := h.svc.Listar(c.UserContext(), filtroEstado, filtroRol, pagina)
	if err != nil {
		return mapearError(err)
	}

	data := make([]usuarioResponse, len(usuarios))
	for i, u := range usuarios {
		data[i] = toUsuarioResponse(u)
	}

	return c.JSON(listarUsuariosResponse{Data: data, Meta: pagina.Meta(total)})
}

// PATCH /api/auth/usuarios/{id}/estado (Admin)
func (h *Handler) CambiarEstado(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req cambiarEstadoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	var accion string
	switch req.Estado {
	case string(domain.EstadoAprobada):
		err = h.svc.Aprobar(c.UserContext(), id)
		accion = audit.CuentaAprobada
	case string(domain.EstadoRechazada):
		err = h.svc.Rechazar(c.UserContext(), id)
		accion = audit.CuentaRechazada
	case string(domain.EstadoBaja):
		err = h.svc.DarDeBaja(c.UserContext(), id)
		accion = audit.CuentaBaja
	default:
		return fiber.NewError(fiber.StatusBadRequest, "estado debe ser APROBADA, RECHAZADA o BAJA")
	}
	if err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, accion, "usuario", &id, nil)
	return c.SendStatus(fiber.StatusOK)
}

// POST /api/auth/usuarios/{id}/reset-password (Admin)
func (h *Handler) ResetearPassword(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	temporal, err := h.svc.ResetearPassword(c.UserContext(), id)
	if err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.PasswordReseteada, "usuario", &id, nil)
	return c.JSON(resetPasswordResponse{PasswordTemporal: temporal})
}

// DELETE /api/auth/usuarios/{id} (Admin) — hard delete, solo desde BAJA
func (h *Handler) EliminarDefinitivamente(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	if err := h.svc.EliminarDefinitivamente(c.UserContext(), id); err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.CuentaEliminadaDefinitiva, "usuario", &id, nil)
	return c.SendStatus(fiber.StatusOK)
}

// POST /api/auth/admins (Admin) — crea otro Admin, auto-aprobado
func (h *Handler) CrearAdmin(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req crearAdminRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	nuevoAdmin, err := h.svc.CrearAdmin(c.UserContext(), claims.UserID, req.Nombre, req.Apellido, req.Email, req.Password)
	if err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.AdminCreado, "usuario", &nuevoAdmin.ID, nil)
	return c.SendStatus(fiber.StatusCreated)
}
