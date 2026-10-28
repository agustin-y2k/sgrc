package http

import (
	"strings"

	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/shared/paginacion"
	"github.com/gofiber/fiber/v2"
)

// La respuesta de consulta omite motivos de cancelación y datos de contacto.
type reservaCalendarioResponse struct {
	BloqueoOperacionID    *string `json:"bloqueoOperacionId,omitempty"`
	ID                    string  `json:"id"`
	ReservaGrupoID        *string `json:"reservaGrupoId,omitempty"`
	EquipoID              string  `json:"equipoId"`
	CreadoPor             *string `json:"creadoPor,omitempty"`
	Propia                bool    `json:"propia"`
	NombreDocenteSnapshot string  `json:"nombreDocenteSnapshot,omitempty"`
	Fecha                 string  `json:"fecha"`
	HoraInicio            string  `json:"horaInicio"`
	HoraFin               string  `json:"horaFin"`
	Estado                string  `json:"estado"`
	Tipo                  string  `json:"tipo"`
	MotivoBloqueo         string  `json:"motivoBloqueo,omitempty"`
	Identificador         int     `json:"identificador"`
	Etiqueta              string  `json:"etiqueta"`
	CarroNombre           string  `json:"carroNombre,omitempty"`
	MateriaNombre         string  `json:"materiaNombre,omitempty"`
	CursoNombre           string  `json:"cursoNombre,omitempty"`
	CursoModalidad        string  `json:"cursoModalidad,omitempty"`
	ReglaRecurrenciaID    *string `json:"reglaRecurrenciaId,omitempty"`
}

func (h *Handler) CalendarioCompartido(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}
	desde, err := parseFecha(c.Query("desde"))
	if err != nil {
		return fiber.NewError(400, "desde: "+err.Error())
	}
	hasta, err := parseFecha(c.Query("hasta"))
	if err != nil {
		return fiber.NewError(400, "hasta: "+err.Error())
	}
	ambito := c.Query("ambito", "todas")
	if ambito != "mias" && ambito != "todas" {
		return fiber.NewError(400, "ámbito inválido: usá mias o todas")
	}
	busqueda := strings.TrimSpace(c.Query("busqueda"))
	if len([]rune(busqueda)) > 100 {
		return fiber.NewError(400, "la búsqueda admite hasta 100 caracteres")
	}
	pagina, err := paginacion.Parsear(c.Query("page"), c.Query("pageSize"))
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	f := application.FiltroReservas{Desde: &desde, Hasta: &hasta, Busqueda: busqueda, Pagina: pagina, IncluirCanceladas: c.Query("incluirCanceladas") == "true"}
	reservas, total, err := h.svc.ListarCalendario(c.UserContext(), f, claims.UserID, ambito == "mias")
	if err != nil {
		return mapearError(err)
	}
	data := make([]reservaCalendarioResponse, 0, len(reservas))
	for _, d := range reservas {
		r := d.Reserva
		data = append(data, reservaCalendarioResponse{
			BloqueoOperacionID: r.BloqueoOperacionID, ID: r.ID, ReservaGrupoID: r.ReservaGrupoID, EquipoID: r.EquipoID, CreadoPor: r.CreadoPor,
			Propia: r.CreadoPor != nil && *r.CreadoPor == claims.UserID, NombreDocenteSnapshot: deref(r.NombreDocenteSnapshot),
			Fecha: r.Fecha.Format("2006-01-02"), HoraInicio: formatHora(r.HoraInicio), HoraFin: formatHora(r.HoraFin),
			Estado: string(r.Estado), Tipo: string(r.Tipo), MotivoBloqueo: r.MotivoBloqueo,
			Identificador: d.Identificador, Etiqueta: d.Etiqueta, CarroNombre: d.CarroNombre,
			MateriaNombre: d.MateriaNombre, CursoNombre: d.CursoNombre, CursoModalidad: d.CursoModalidad, ReglaRecurrenciaID: d.ReglaRecurrenciaID,
		})
	}
	return c.JSON(fiber.Map{"data": data, "meta": pagina.Meta(total)})
}
