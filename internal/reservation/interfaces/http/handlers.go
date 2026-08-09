package http

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/middleware"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
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

func claimsDelContexto(c *fiber.Ctx) (*middleware.Claims, error) {
	claims := middleware.ClaimsFromCtx(c)
	if claims == nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "no autenticado")
	}
	return claims, nil
}

// POST /api/reservation/reservas (cualquier usuario autenticado — la
// validación real de "está asignado a esa materia" la hace application/)
func (h *Handler) CrearReserva(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req crearReservaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	fecha, err := parseFecha(req.Fecha)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	horaInicio, err := parseHora(req.HoraInicio)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	horaFin, err := parseHora(req.HoraFin)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	// RF-04.1: un Admin puede reservar para cualquier materia no archivada,
	// sin estar asignado a ella.
	grupo, reservas, err := h.svc.CrearReserva(c.UserContext(), req.MateriaID, claims.UserID,
		claims.Rol == "ADMIN", fecha, horaInicio, horaFin, req.EquipoIDs)
	if err != nil {
		return mapearError(err)
	}

	reservasResp := make([]reservaResponse, len(reservas))
	for i, r := range reservas {
		reservasResp[i] = toReservaResponse(r)
	}
	return c.Status(fiber.StatusCreated).JSON(crearReservaResponse{
		Grupo: toReservaGrupoResponse(grupo), Reservas: reservasResp,
	})
}

// POST /api/reservation/reservas/recurrentes (cualquier usuario autenticado)
func (h *Handler) CrearReservaRecurrente(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req crearReservaRecurrenteRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	diaSemana, err := domain.ParseDiaSemana(req.DiaSemana)
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
	fechaInicio, err := parseFecha(req.FechaInicio)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	fechaFin, err := parseFecha(req.FechaFin)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	res, err := h.svc.CrearReservaRecurrente(c.UserContext(), req.MateriaID, claims.UserID,
		claims.Rol == "ADMIN", diaSemana, horaInicio, horaFin, fechaInicio, fechaFin, req.EquipoIDs)
	if err != nil {
		return mapearError(err)
	}

	grupos := make([]reservaGrupoResponse, len(res.Grupos))
	for i, g := range res.Grupos {
		grupos[i] = toReservaGrupoResponse(g)
	}
	return c.Status(fiber.StatusCreated).JSON(crearReservaRecurrenteResponse{ReglaID: res.Regla.ID, Grupos: grupos})
}

// POST /api/reservation/reservas/{id}/cancelar — RF-04.4. Un Admin puede
// cancelar cualquier reserva; un docente solo las suyas.
func (h *Handler) CancelarReserva(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req cancelarReservaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	reserva, err := h.svc.ObtenerReserva(c.UserContext(), id)
	if err != nil {
		return mapearError(err)
	}
	esPropia := reserva.CreadoPor != nil && *reserva.CreadoPor == claims.UserID

	if claims.Rol != "ADMIN" && !esPropia {
		return fiber.NewError(fiber.StatusForbidden, "solo podés cancelar tus propias reservas")
	}

	// RF-04.8: cancelar la reserva de otra persona exige motivo — es el
	// texto que el docente va a recibir en la notificación (RF-05.1), así
	// que vacío lo dejaría sin ninguna explicación. Cancelar la propia no
	// lo pide.
	if !esPropia && strings.TrimSpace(req.Motivo) == "" {
		return mapearError(application.ErrMotivoObligatorio)
	}

	usuarioID := claims.UserID
	if err := h.svc.CancelarReserva(c.UserContext(), id, &usuarioID, req.Motivo); err != nil {
		return mapearError(err)
	}
	if claims.Rol == "ADMIN" {
		h.auditar(c, claims.UserID, audit.ReservaCanceladaPorAdmin, "reserva", &id, map[string]any{"motivo": req.Motivo})
	}
	return c.SendStatus(fiber.StatusOK)
}

// POST /api/reservation/grupos/{id}/cancelar — RF-04.6, con soloEsta /
// esta-y-siguientes. Mismo criterio de titularidad que CancelarReserva.
func (h *Handler) CancelarOcurrenciaRecurrente(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req cancelarOcurrenciaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	grupo, err := h.svc.ObtenerReservaGrupo(c.UserContext(), id)
	if err != nil {
		return mapearError(err)
	}
	esPropia := grupo.CreadoPor != nil && *grupo.CreadoPor == claims.UserID

	if claims.Rol != "ADMIN" && !esPropia {
		return fiber.NewError(fiber.StatusForbidden, "solo podés cancelar tus propias reservas")
	}
	// Mismo criterio que CancelarReserva (RF-04.8): cancelar lo ajeno pide
	// motivo, cancelar lo propio no.
	if !esPropia && strings.TrimSpace(req.Motivo) == "" {
		return mapearError(application.ErrMotivoObligatorio)
	}

	usuarioID := claims.UserID
	n, err := h.svc.CancelarOcurrenciaRecurrente(c.UserContext(), id, &usuarioID, req.Motivo, req.SoloEsta)
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(cancelarOcurrenciaResponse{ReservasCanceladas: n})
}

// POST /api/reservation/bloqueos-evaluacion (Admin)
func (h *Handler) BloquearEquipos(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req bloquearRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	fecha, err := parseFecha(req.Fecha)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	horaInicio, err := parseHora(req.HoraInicio)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	horaFin, err := parseHora(req.HoraFin)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	usuarioID := claims.UserID
	res, err := h.svc.BloquearEquipos(c.UserContext(), req.EquipoIDs, &usuarioID, fecha, horaInicio, horaFin, req.Motivo)
	if err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.BloqueoEvaluacionCreado, "reserva", nil, map[string]any{
		"equipoIds":           req.EquipoIDs,
		"fecha":               req.Fecha,
		"motivo":              req.Motivo,
		"reservasCanceladas":  res.ReservasCanceladas,
		"docentesNotificados": res.DocentesNotificados,
	})
	return c.Status(fiber.StatusCreated).JSON(toBloquearResponse(res))
}

// GET /api/reservation/grupos/{id} — la reserva propia; un Admin puede ver
// cualquiera.
//
// El chequeo de titularidad faltaba: alcanzaba con estar autenticado para
// leer el grupo de cualquier otra persona, con su nombre y su materia
// adentro. Es el mismo criterio que ya aplicaban ListarReservas (que le
// fuerza el filtro por creador al docente) y las dos cancelaciones — este
// endpoint era el único que se lo salteaba.
func (h *Handler) ObtenerReservaGrupo(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	grupo, err := h.svc.ObtenerReservaGrupo(c.UserContext(), id)
	if err != nil {
		return mapearError(err)
	}

	esPropia := grupo.CreadoPor != nil && *grupo.CreadoPor == claims.UserID
	if claims.Rol != "ADMIN" && !esPropia {
		return fiber.NewError(fiber.StatusForbidden, "solo podés ver tus propias reservas")
	}

	return c.JSON(toReservaGrupoResponse(grupo))
}

// GET /api/reservation/reservas — lista reservas con filtros opcionales
// (materiaId, equipoId, desde, hasta, incluirCanceladas).
//
// Control de acceso: un docente solo puede ver las suyas, así que el
// filtro por creador se le fuerza sin importar lo que mande. Un Admin ve
// todas, y puede acotar con ?creadoPor=<id>.
func (h *Handler) ListarReservas(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	filtro := application.FiltroReservas{
		IncluirCanceladas: c.Query("incluirCanceladas") == "true",
	}

	if claims.Rol == "ADMIN" {
		if v := c.Query("creadoPor"); v != "" {
			filtro.CreadoPor = &v
		}
	} else {
		usuarioID := claims.UserID
		filtro.CreadoPor = &usuarioID
	}

	if v := c.Query("materiaId"); v != "" {
		filtro.MateriaID = &v
	}
	if v := c.Query("equipoId"); v != "" {
		filtro.EquipoID = &v
	}
	if v := c.Query("desde"); v != "" {
		desde, err := parseFecha(v)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		filtro.Desde = &desde
	}
	if v := c.Query("hasta"); v != "" {
		hasta, err := parseFecha(v)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		filtro.Hasta = &hasta
	}

	pagina, err := paginacion.Parsear(c.Query("page"), c.Query("pageSize"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	filtro.Pagina = pagina

	reservas, total, err := h.svc.ListarReservas(c.UserContext(), filtro)
	if err != nil {
		return mapearError(err)
	}

	data := make([]reservaDetalladaResponse, len(reservas))
	for i, r := range reservas {
		data[i] = toReservaDetalladaResponse(r)
	}
	return c.JSON(listarReservasResponse{
		Data: data,
		Meta: pagina.Meta(total),
	})
}

// GET /api/reservation/equipos/{equipoId}/calendario?desde&hasta — RF-04.4.
// Cualquier usuario autenticado puede consultarlo: un docente necesita ver
// qué equipos están libres antes de elegir cuáles reservar.
func (h *Handler) CalendarioDeEquipo(c *fiber.Ctx) error {
	equipoID := c.Params("equipoId")

	desde, err := parseFecha(c.Query("desde"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "desde: "+err.Error())
	}
	hasta, err := parseFecha(c.Query("hasta"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "hasta: "+err.Error())
	}

	bloques, err := h.svc.CalendarioDeEquipo(c.UserContext(), equipoID, desde, hasta)
	if err != nil {
		return mapearError(err)
	}

	resp := calendarioEquipoResponse{
		EquipoID: equipoID,
		Desde:    desde.Format("2006-01-02"),
		Hasta:    hasta.Format("2006-01-02"),
		Bloques:  make([]bloqueCalendarioResponse, len(bloques)),
	}
	for i, b := range bloques {
		resp.Bloques[i] = toBloqueCalendarioResponse(b)
	}
	return c.JSON(resp)
}

// GET /api/reservation/equipos-disponibles?fecha&horaInicio&horaFin — RF-04.2.
// La lista de la que el docente tilda las PCs que necesita; no está
// restringida a un solo carro.
func (h *Handler) ListarEquiposDisponibles(c *fiber.Ctx) error {
	fecha, err := parseFecha(c.Query("fecha"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "fecha: "+err.Error())
	}
	horaInicio, err := parseHora(c.Query("horaInicio"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "horaInicio: "+err.Error())
	}
	horaFin, err := parseHora(c.Query("horaFin"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "horaFin: "+err.Error())
	}

	equipos, err := h.svc.ListarEquiposDisponiblesEn(c.UserContext(), fecha, horaInicio, horaFin)
	if err != nil {
		return mapearError(err)
	}

	data := make([]equipoDisponibleResponse, len(equipos))
	for i, p := range equipos {
		data[i] = toEquipoDisponibleResponse(p)
	}
	return c.JSON(equiposDisponiblesResponse{Data: data})
}

// PATCH /api/reservation/reservas/{id}/equipo — cambiar una reserva de máquina.
//
// Existe para que "elegí otra" no obligue a cancelar y volver a reservar,
// que arma un grupo nuevo y parte la clase en dos tarjetas. Es de quien
// tenga la reserva, o de un Admin.
func (h *Handler) CambiarEquipoDeReserva(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req cambiarEquipoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}
	if strings.TrimSpace(req.EquipoID) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "hay que indicar el equipo nuevo")
	}

	reserva, err := h.svc.CambiarEquipoDeReserva(c.UserContext(), c.Params("id"), req.EquipoID,
		claims.UserID, claims.Rol == "ADMIN")
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toReservaResponse(reserva))
}
