package http

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/reporting/application"
)

// rangoDeQuery lee los parámetros opcionales ?desde=&hasta= (RF-06.1).
// Ausentes = sin ese límite.
func rangoDeQuery(c *fiber.Ctx) (desde, hasta *time.Time, err error) {
	if v := c.Query("desde"); v != "" {
		d, e := time.Parse("2006-01-02", v)
		if e != nil {
			return nil, nil, fiber.NewError(fiber.StatusBadRequest, "desde: la fecha debe tener formato YYYY-MM-DD")
		}
		desde = &d
	}
	if v := c.Query("hasta"); v != "" {
		h, e := time.Parse("2006-01-02", v)
		if e != nil {
			return nil, nil, fiber.NewError(fiber.StatusBadRequest, "hasta: la fecha debe tener formato YYYY-MM-DD")
		}
		hasta = &h
	}
	return desde, hasta, nil
}

type Handler struct {
	svc *application.Service
}

func NewHandler(svc *application.Service) *Handler {
	return &Handler{svc: svc}
}

// GET /api/reporting/ciclos/:cicloId/uso-equipos — RF-06.1, en vivo.
func (h *Handler) ReporteUsoEquipos(c *fiber.Ctx) error {
	cicloID := c.Params("cicloId")

	desde, hasta, err := rangoDeQuery(c)
	if err != nil {
		return err
	}

	resumenes, err := h.svc.ReporteUsoEquipos(c.UserContext(), cicloID, desde, hasta)
	if err != nil {
		return mapearError(err)
	}

	data := make([]resumenUsoEquipoResponse, len(resumenes))
	for i, u := range resumenes {
		data[i] = toResumenUsoEquipoResponse(u)
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/reporting/ciclos/:cicloId/uso-docentes — RF-06.2, en vivo.
func (h *Handler) ReporteUsoDocentes(c *fiber.Ctx) error {
	cicloID := c.Params("cicloId")

	desde, hasta, err := rangoDeQuery(c)
	if err != nil {
		return err
	}

	resumenes, err := h.svc.ReporteUsoDocentes(c.UserContext(), cicloID, desde, hasta)
	if err != nil {
		return mapearError(err)
	}

	data := make([]resumenUsoDocenteResponse, len(resumenes))
	for i, u := range resumenes {
		data[i] = toResumenUsoDocenteResponse(u)
	}
	return c.JSON(fiber.Map{"data": data})
}

// parseAnio interpreta el parámetro de ruta :anio como un entero — un
// año con formato inválido (ej. "abc") es un 400 claro, no un 500.
func parseAnio(c *fiber.Ctx) (int, error) {
	anio, err := strconv.Atoi(c.Params("anio"))
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, "el año debe ser un número")
	}
	return anio, nil
}

// GET /api/reporting/historico/:anio/uso-equipos — RF-06.3, ya archivado.
func (h *Handler) HistoricoUsoEquipos(c *fiber.Ctx) error {
	anio, err := parseAnio(c)
	if err != nil {
		return err
	}

	historico, err := h.svc.HistoricoUsoEquipos(c.UserContext(), anio)
	if err != nil {
		return mapearError(err)
	}

	data := make([]historicoUsoEquipoResponse, len(historico))
	for i, h := range historico {
		data[i] = toHistoricoUsoEquipoResponse(h)
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/reporting/historico/:anio/uso-docentes — RF-06.3, ya archivado.
func (h *Handler) HistoricoUsoDocentes(c *fiber.Ctx) error {
	anio, err := parseAnio(c)
	if err != nil {
		return err
	}

	historico, err := h.svc.HistoricoUsoDocentes(c.UserContext(), anio)
	if err != nil {
		return mapearError(err)
	}

	data := make([]historicoUsoDocenteResponse, len(historico))
	for i, hi := range historico {
		data[i] = toHistoricoUsoDocenteResponse(hi)
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/reporting/incidencias/equipos — RF-06.3, incidencias por equipo.
func (h *Handler) ReporteIncidenciasPorEquipo(c *fiber.Ctx) error {
	desde, hasta, err := rangoDeQuery(c)
	if err != nil {
		return err
	}

	resumenes, err := h.svc.ReporteIncidenciasPorEquipo(c.UserContext(), desde, hasta)
	if err != nil {
		return mapearError(err)
	}

	data := make([]resumenIncidenciasEquipoResponse, len(resumenes))
	for i, x := range resumenes {
		data[i] = toResumenIncidenciasEquipoResponse(x)
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/reporting/incidencias/carros — RF-06.3, incidencias por carro.
func (h *Handler) ReporteIncidenciasPorCarro(c *fiber.Ctx) error {
	desde, hasta, err := rangoDeQuery(c)
	if err != nil {
		return err
	}

	resumenes, err := h.svc.ReporteIncidenciasPorCarro(c.UserContext(), desde, hasta)
	if err != nil {
		return mapearError(err)
	}

	data := make([]resumenIncidenciasCarroResponse, len(resumenes))
	for i, x := range resumenes {
		data[i] = toResumenIncidenciasCarroResponse(x)
	}
	return c.JSON(fiber.Map{"data": data})
}

// ── RF-06.5: el estado del parque de equipos ────────────────────────────

// GET /api/reporting/inventario/estado Sin rango de fechas a propósito: es
// una foto de AHORA. "Cuántas estaban rotas en marzo" no se puede responder
// con el estado actual, y aceptar el parámetro daría un número que parece esa
// respuesta sin serlo.
func (h *Handler) ReporteEstadoDelInventario(c *fiber.Ctx) error {
	filas, err := h.svc.EstadoDelInventario(c.UserContext())
	if err != nil {
		return mapearError(err)
	}

	data := make([]estadoDelInventarioResponse, len(filas))
	for i, e := range filas {
		data[i] = toEstadoDelInventarioResponse(e)
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/reporting/inventario/fuera-de-circulacion — la lista que se manda
// a reparar: qué máquinas están afuera y qué se sabe de cada una.
func (h *Handler) ReporteEquiposFueraDeCirculacion(c *fiber.Ctx) error {
	filas, err := h.svc.EquiposFueraDeCirculacion(c.UserContext())
	if err != nil {
		return mapearError(err)
	}

	data := make([]equipoFueraDeCirculacionResponse, len(filas))
	for i, e := range filas {
		data[i] = toEquipoFueraDeCirculacionResponse(e)
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/reporting/incidencias/categorias — qué se rompe, agrupado por tipo
// de falla.
func (h *Handler) ReporteIncidenciasPorCategoria(c *fiber.Ctx) error {
	desde, hasta, err := rangoDeQuery(c)
	if err != nil {
		return err
	}

	filas, err := h.svc.ReporteIncidenciasPorCategoria(c.UserContext(), desde, hasta)
	if err != nil {
		return mapearError(err)
	}

	data := make([]resumenPorCategoriaResponse, len(filas))
	for i, x := range filas {
		data[i] = toResumenPorCategoriaResponse(x)
	}
	return c.JSON(fiber.Map{"data": data})
}
