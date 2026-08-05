package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/availability/application"
	"github.com/ramiro/sgrc/internal/availability/domain"
	"github.com/ramiro/sgrc/internal/shared/middleware"
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

// GET /api/availability/admins — cualquier usuario autenticado (RF-07.2).
// Puramente informativo: no afecta ninguna otra funcionalidad (RF-07.6).
func (h *Handler) DisponibilidadDeAdmins(c *fiber.Ctx) error {
	resultado, err := h.svc.DisponibilidadDeTodosLosAdmins(c.UserContext())
	if err != nil {
		return mapearError(err)
	}

	data := make([]adminDisponibilidadResponse, len(resultado))
	for i, r := range resultado {
		data[i] = toAdminDisponibilidadResponse(r)
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/availability/mi-horario (Admin) — el patrón semanal propio.
func (h *Handler) MiHorario(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	bloques, err := h.svc.MiHorario(c.UserContext(), claims.UserID)
	if err != nil {
		return mapearError(err)
	}

	data := make([]bloqueResponse, len(bloques))
	for i, b := range bloques {
		data[i] = toBloqueResponse(b)
	}
	return c.JSON(fiber.Map{"data": data})
}

// POST /api/availability/mi-horario (Admin) — RF-07.1/07.3, aplica de
// inmediato para todas las semanas futuras.
func (h *Handler) AgregarBloque(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req bloqueRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	dia, err := domain.ParseDiaSemana(req.DiaSemana)
	if err != nil {
		return mapearError(err)
	}
	horaInicio, err := parseHora(req.HoraInicio)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	horaFin, err := parseHora(req.HoraFin)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	bloque, err := h.svc.AgregarBloque(c.UserContext(), claims.UserID, dia, horaInicio, horaFin)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toBloqueResponse(bloque))
}

// PATCH /api/availability/mi-horario/{id} (Admin) — la titularidad se
// resuelve en application/Repo (acotada por usuario_id), no acá: si el
// bloque no es del usuario autenticado, el resultado es indistinguible de
// "no existe" (404).
func (h *Handler) EditarBloque(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req editarBloqueRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	var dia *domain.DiaSemana
	if req.DiaSemana != nil {
		d, err := domain.ParseDiaSemana(*req.DiaSemana)
		if err != nil {
			return mapearError(err)
		}
		dia = &d
	}
	var horaInicio *time.Duration
	if req.HoraInicio != nil {
		hi, err := parseHora(*req.HoraInicio)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		horaInicio = &hi
	}
	var horaFin *time.Duration
	if req.HoraFin != nil {
		hf, err := parseHora(*req.HoraFin)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		horaFin = &hf
	}

	bloque, err := h.svc.EditarBloque(c.UserContext(), id, claims.UserID, dia, horaInicio, horaFin)
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toBloqueResponse(bloque))
}

// DELETE /api/availability/mi-horario/{id} (Admin) — mismo criterio de
// titularidad que EditarBloque.
func (h *Handler) EliminarBloque(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	if err := h.svc.EliminarBloque(c.UserContext(), id, claims.UserID); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusOK)
}

// POST /api/availability/mi-excepcion (Admin) — RF-07.4. Reemplaza la
// excepción existente para esa fecha si ya había una.
func (h *Handler) CargarExcepcion(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req excepcionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	fecha, err := parseFecha(req.Fecha)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	tipo, err := domain.ParseTipoExcepcion(req.Tipo)
	if err != nil {
		return mapearError(err)
	}

	var horaInicio, horaFin *time.Duration
	if req.HoraInicio != nil {
		hi, err := parseHora(*req.HoraInicio)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		horaInicio = &hi
	}
	if req.HoraFin != nil {
		hf, err := parseHora(*req.HoraFin)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		horaFin = &hf
	}

	excepcion, err := h.svc.CargarExcepcion(c.UserContext(), claims.UserID, fecha, tipo, horaInicio, horaFin, req.Motivo)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toExcepcionResponse(excepcion))
}

// POST /api/availability/no-disponible-ahora (Admin) — RF-07.5, atajo de
// un solo paso equivalente a POST /mi-excepcion con tipo=NO_DISPONIBLE
// para hoy.
func (h *Handler) MarcarNoDisponibleAhora(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	excepcion, err := h.svc.MarcarNoDisponibleAhora(c.UserContext(), claims.UserID)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toExcepcionResponse(excepcion))
}
