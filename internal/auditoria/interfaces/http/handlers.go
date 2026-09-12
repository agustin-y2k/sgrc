package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/auditoria/application"
	"github.com/ramiro/sgrc/internal/auditoria/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

type Handler struct {
	svc *application.Service
}

func NewHandler(svc *application.Service) *Handler {
	return &Handler{svc: svc}
}

// GET /api/auditoria (Admin)
//
// Nota deliberada: consultar el registro NO se audita. Una auditoría que se
// audita a sí misma crece con cada consulta y entierra los cambios de verdad
// bajo el ruido de quien fue a mirarlos.
func (h *Handler) Listar(c *fiber.Ctx) error {
	pagina, err := paginacion.Parsear(c.Query("page"), c.Query("pageSize"))
	if err != nil {
		return mapearError(err)
	}

	filtro := domain.Filtro{
		Accion:    c.Query("accion"),
		Entidad:   c.Query("entidad"),
		EntidadID: c.Query("entidadId"),
		UsuarioID: c.Query("usuarioId"),
	}

	if v := c.Query("desde"); v != "" {
		desde, err := time.Parse(time.DateOnly, v)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "«desde» tiene que ser una fecha con formato AAAA-MM-DD")
		}
		filtro.Desde = &desde
	}
	if v := c.Query("hasta"); v != "" {
		hasta, err := time.Parse(time.DateOnly, v)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "«hasta» tiene que ser una fecha con formato AAAA-MM-DD")
		}
		// El día entero: quien escribe «hasta el 5» espera que entre lo que pasó
		// el 5 a las 23:00. Se convierte a un límite exclusivo del día siguiente
		// porque comparar contra las 00:00 del 5 dejaría afuera todo ese día.
		finDelDia := hasta.AddDate(0, 0, 1)
		filtro.Hasta = &finDelDia
	}

	entradas, total, err := h.svc.Listar(c.UserContext(), filtro, pagina)
	if err != nil {
		return mapearError(err)
	}

	data := make([]entradaResponse, len(entradas))
	for i, e := range entradas {
		data[i] = toEntradaResponse(e)
	}
	return c.JSON(fiber.Map{"data": data, "meta": pagina.Meta(total)})
}

// GET /api/auditoria/opciones (Admin) — qué acciones y entidades existen HOY en
// el registro, para armar los dos selectores sin ofrecer valores que en esta
// instalación nunca ocurrieron.
func (h *Handler) Opciones(c *fiber.Ctx) error {
	opciones, err := h.svc.OpcionesDeFiltro(c.UserContext())
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(opcionesResponse{Acciones: opciones.Acciones, Entidades: opciones.Entidades})
}
