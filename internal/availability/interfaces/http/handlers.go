package http

import (
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/availability/application"
	"github.com/ramiro/sgrc/internal/availability/domain"
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

// auditar registra una entrada sin abortar la respuesta si falla, igual que
// en auth y reservation: el cambio ya se aplicó, y perder el registro es malo
// pero devolver un error por eso sería peor.
func (h *Handler) auditar(c *fiber.Ctx, actorID, accion, entidad string, detalle map[string]any) {
	if err := h.auditor.Registrar(c.UserContext(), audit.Entrada{
		UsuarioID: actorID,
		Accion:    accion,
		Entidad:   entidad,
		Detalle:   detalle,
		IPOrigen:  middleware.IPCliente(c),
	}); err != nil {
		log.Printf("auditoría: no se pudo registrar %s sobre %s: %v", accion, entidad, err)
	}
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
// resuelve en application/Repo (acotada por usuario_id), no acá: si el bloque
// no es del usuario autenticado, el resultado es indistinguible de "no
// existe" (404).
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

// POST /api/availability/no-disponible-ahora (Admin) — RF-07.5, atajo de un
// solo paso equivalente a POST /mi-excepcion con tipo=NO_DISPONIBLE para hoy.
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

// ── Jornada de la institución ───────────────────────────────────────────

// accionJornadaCambiada es el nombre que queda escrito en audit_log. Va como
// constante y no en línea para que no se escriba distinto en los dos lugares
// que la registran —el camino normal y el de la cascada incompleta— y las
// dos filas terminen pareciendo acciones diferentes.
const accionJornadaCambiada = "JORNADA_CAMBIADA"

// GET /api/jornada — la jornada declarada por la institución.
//
// `definida` viaja al lado de los tramos y no en un endpoint aparte porque
// quien pregunta por la jornada casi siempre necesita las dos cosas, y
// separarlas obligaría a dos llamadas para pintar una sola pantalla. Sin él,
// una lista vacía es ambigua: no se sabe si la escuela todavía no declaró su
// jornada o si eligió no restringir nada.
func (h *Handler) Jornada(c *fiber.Ctx) error {
	bloques, err := h.svc.Jornada(c.UserContext())
	if err != nil {
		return mapearError(err)
	}
	definida, err := h.svc.JornadaDefinida(c.UserContext())
	if err != nil {
		return mapearError(err)
	}

	data := make([]bloqueResponse, len(bloques))
	for i, b := range bloques {
		data[i] = toBloqueJornadaResponse(b)
	}
	return c.JSON(fiber.Map{"data": data, "definida": definida})
}

// PUT /api/jornada (Admin) — reemplaza la jornada entera.
//
// Es un PUT del conjunto y no tres endpoints por tramo porque la jornada es
// una sola decisión de siete días. Con los cambios llegando de a uno no había
// ningún momento en que preguntar "esto es lo que va a quedar, ¿confirmás?",
// y entre una llamada y la otra quedaba a la vista una jornada a medias que
// ya decidía qué reservas se aceptaban.
//
// Una lista vacía es válida y no es lo mismo que no llamar: es la institución
// eligiendo no restringir nada, y deja de preguntársele cuál es su jornada.
func (h *Handler) ReemplazarJornada(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req jornadaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	tramos := make([]application.TramoDeJornada, 0, len(req.Tramos))
	for _, t := range req.Tramos {
		dia, err := domain.ParseDiaSemana(t.DiaSemana)
		if err != nil {
			return mapearError(err)
		}
		horaInicio, err := parseHora(t.HoraInicio)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		horaFin, err := parseHora(t.HoraFin)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		tramos = append(tramos, application.TramoDeJornada{
			DiaSemana: dia, HoraInicio: horaInicio, HoraFin: horaFin,
		})
	}

	resultado, err := h.svc.ReemplazarJornada(c.UserContext(), tramos, req.Confirmado)
	if err != nil {
		// La cascada a medias deja la jornada nueva puesta: hay que auditarla
		// igual, con lo que sí pasó, o el registro diría que el horario de la
		// escuela cambió solo porque nadie lo anotó.
		if errors.Is(err, application.ErrCascadaDeJornada) {
			h.auditar(c, claims.UserID, accionJornadaCambiada, "jornada_institucion", map[string]any{
				"tramos":            len(tramos),
				"cascadaIncompleta": true,
			})
		}
		return mapearError(err)
	}

	// 409 con el detalle adentro, y sin haber tocado nada: el pedido está bien
	// formado, lo que no se puede es aplicarlo sin que alguien se haga cargo
	// de las clases que se caen. El mismo endpoint hace de previsualización,
	// así que no hay forma de que lo que se muestra difiera de lo que después
	// se aplica.
	if !resultado.Guardada {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error":   "el cambio deja reservas fuera de la jornada",
			"impacto": toImpactoResponse(resultado.Impacto),
		})
	}

	// Es la acción administrativa de mayor alcance del sistema: puede cancelar
	// las clases de toda la escuela de una vez. Queda registrado quién la
	// disparó y cuánto se llevó puesto, por lo mismo que
	// CICLO_ARCHIVADO_RESERVAS_ELIMINADAS (docs/09-seguridad-rbac.md §5).
	h.auditar(c, claims.UserID, accionJornadaCambiada, "jornada_institucion", map[string]any{
		"tramos":             len(tramos),
		"reservasCanceladas": resultado.ReservasCanceladas,
	})

	data := make([]bloqueResponse, len(resultado.Bloques))
	for i, b := range resultado.Bloques {
		data[i] = toBloqueJornadaResponse(b)
	}
	return c.JSON(fiber.Map{
		"data":               data,
		"definida":           true,
		"reservasCanceladas": resultado.ReservasCanceladas,
	})
}
