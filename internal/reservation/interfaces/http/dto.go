// Package http expone las rutas Fiber de reservation — ver
// docs/08-api-spec.yaml para el contrato completo de cada endpoint.
package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// ── Requests ────────────────────────────────────────────────────────────
//
// HoraInicio/HoraFin viajan como "HH:MM" en el JSON — se parsean a
// time.Duration en los handlers (ver parseHora en handlers.go).

type crearReservaRequest struct {
	MateriaID  string   `json:"materiaId"`
	Fecha      string   `json:"fecha"` // "2026-03-09"
	HoraInicio string   `json:"horaInicio"`
	HoraFin    string   `json:"horaFin"`
	PCIDs      []string `json:"pcIds"`
}

type crearReservaRecurrenteRequest struct {
	MateriaID   string   `json:"materiaId"`
	DiaSemana   string   `json:"diaSemana"`
	HoraInicio  string   `json:"horaInicio"`
	HoraFin     string   `json:"horaFin"`
	FechaInicio string   `json:"fechaInicio"`
	FechaFin    string   `json:"fechaFin"`
	PCIDs       []string `json:"pcIds"`
}

type cancelarReservaRequest struct {
	Motivo string `json:"motivo"`
}

type cancelarOcurrenciaRequest struct {
	Motivo   string `json:"motivo"`
	SoloEsta bool   `json:"soloEsta"`
}

type bloquearEvaluacionRequest struct {
	PCIDs      []string `json:"pcIds"`
	Fecha      string   `json:"fecha"`
	HoraInicio string   `json:"horaInicio"`
	HoraFin    string   `json:"horaFin"`
	Motivo     string   `json:"motivo"`
}

// ── Responses ───────────────────────────────────────────────────────────

type reservaGrupoResponse struct {
	ID                    string  `json:"id"`
	MateriaID             string  `json:"materiaId"`
	CreadoPor             *string `json:"creadoPor,omitempty"`
	NombreDocenteSnapshot string  `json:"nombreDocenteSnapshot"`
	Fecha                 string  `json:"fecha"`
	HoraInicio            string  `json:"horaInicio"`
	HoraFin               string  `json:"horaFin"`
	Estado                string  `json:"estado"`
	ReglaRecurrenciaID    *string `json:"reglaRecurrenciaId,omitempty"`
}

func toReservaGrupoResponse(g *domain.ReservaGrupo) reservaGrupoResponse {
	return reservaGrupoResponse{
		ID: g.ID, MateriaID: g.MateriaID, CreadoPor: g.CreadoPor,
		NombreDocenteSnapshot: g.NombreDocenteSnapshot,
		Fecha:                 g.Fecha.Format("2006-01-02"),
		HoraInicio:            formatHora(g.HoraInicio),
		HoraFin:               formatHora(g.HoraFin),
		Estado:                string(g.Estado),
		ReglaRecurrenciaID:    g.ReglaRecurrenciaID,
	}
}

type reservaResponse struct {
	ID                    string     `json:"id"`
	ReservaGrupoID        *string    `json:"reservaGrupoId,omitempty"`
	PCID                  string     `json:"pcId"`
	MateriaID             *string    `json:"materiaId,omitempty"`
	NombreDocenteSnapshot *string    `json:"nombreDocenteSnapshot,omitempty"`
	Fecha                 string     `json:"fecha"`
	HoraInicio            string     `json:"horaInicio"`
	HoraFin               string     `json:"horaFin"`
	Estado                string     `json:"estado"`
	Tipo                  string     `json:"tipo"`
	CreadoPor             *string    `json:"creadoPor,omitempty"`
	CanceladoPor          *string    `json:"canceladoPor,omitempty"`
	MotivoCancelacion     *string    `json:"motivoCancelacion,omitempty"`
	CanceladaEn           *time.Time `json:"canceladaEn,omitempty"`
}

func toReservaResponse(r *domain.Reserva) reservaResponse {
	return reservaResponse{
		ID: r.ID, ReservaGrupoID: r.ReservaGrupoID, PCID: r.PCID, MateriaID: r.MateriaID,
		NombreDocenteSnapshot: r.NombreDocenteSnapshot,
		Fecha:                 r.Fecha.Format("2006-01-02"),
		HoraInicio:            formatHora(r.HoraInicio),
		HoraFin:               formatHora(r.HoraFin),
		Estado:                string(r.Estado),
		Tipo:                  string(r.Tipo),
		CreadoPor:             r.CreadoPor,
		CanceladoPor:          r.CanceladoPor,
		MotivoCancelacion:     r.MotivoCancelacion,
		CanceladaEn:           r.CanceladaEn,
	}
}

// reservaDetalladaResponse agrega a reservaResponse los nombres que hacen
// falta para mostrar la reserva en pantalla. Sin ellos, "Mis reservas"
// solo tenía UUIDs: una reserva de ocho PCs se veía como ocho tarjetas
// idénticas, sin forma de saber cuál era cuál.
type reservaDetalladaResponse struct {
	reservaResponse
	PCIdentificador int    `json:"pcIdentificador"`
	CarroNombre     string `json:"carroNombre"`
	MateriaNombre   string `json:"materiaNombre,omitempty"`
	CursoNombre     string `json:"cursoNombre,omitempty"`
	// Presente solo si la reserva es parte de una serie recurrente. De esto
	// depende que tenga sentido ofrecer "cancelar esta y las siguientes"
	// (RF-04.6). No sirve usar reservaGrupoId como proxy: lo tienen TODAS
	// las reservas, así que la opción aparecería siempre.
	ReglaRecurrenciaID *string `json:"reglaRecurrenciaId,omitempty"`
}

func toReservaDetalladaResponse(d application.ReservaDetallada) reservaDetalladaResponse {
	return reservaDetalladaResponse{
		reservaResponse:    toReservaResponse(d.Reserva),
		PCIdentificador:    d.PCIdentificador,
		CarroNombre:        d.CarroNombre,
		MateriaNombre:      d.MateriaNombre,
		CursoNombre:        d.CursoNombre,
		ReglaRecurrenciaID: d.ReglaRecurrenciaID,
	}
}

type crearReservaResponse struct {
	Grupo    reservaGrupoResponse `json:"grupo"`
	Reservas []reservaResponse    `json:"reservas"`
}

type crearReservaRecurrenteResponse struct {
	ReglaID string                 `json:"reglaId"`
	Grupos  []reservaGrupoResponse `json:"grupos"`
}

type cancelarOcurrenciaResponse struct {
	ReservasCanceladas int `json:"reservasCanceladas"`
}

type bloquearEvaluacionResponse struct {
	Bloqueos            []reservaResponse `json:"bloqueos"`
	ReservasCanceladas  int               `json:"reservasCanceladas"`
	DocentesNotificados int               `json:"docentesNotificados"`
}

func toBloquearEvaluacionResponse(res *application.ResultadoBloqueoEvaluacion) bloquearEvaluacionResponse {
	bloqueos := make([]reservaResponse, len(res.Bloqueos))
	for i, b := range res.Bloqueos {
		bloqueos[i] = toReservaResponse(b)
	}
	return bloquearEvaluacionResponse{
		Bloqueos: bloqueos, ReservasCanceladas: res.ReservasCanceladas, DocentesNotificados: res.DocentesNotificados,
	}
}

type listarReservasResponse struct {
	Data []reservaDetalladaResponse `json:"data"`
	Meta paginacion.Meta            `json:"meta"`
}

// bloqueCalendarioResponse es lo que RF-04.4 pide mostrar de cada bloque
// ocupado: horario, docente y materia. Para un bloqueo por evaluación
// estatal materia/curso vienen vacíos y el tipo lo aclara.
type bloqueCalendarioResponse struct {
	ReservaID     string `json:"reservaId"`
	Fecha         string `json:"fecha"`
	HoraInicio    string `json:"horaInicio"`
	HoraFin       string `json:"horaFin"`
	Estado        string `json:"estado"`
	Tipo          string `json:"tipo"`
	Docente       string `json:"docente"`
	MateriaNombre string `json:"materiaNombre,omitempty"`
	CursoNombre   string `json:"cursoNombre,omitempty"`
}

type calendarioPCResponse struct {
	PCID    string                     `json:"pcId"`
	Desde   string                     `json:"desde"`
	Hasta   string                     `json:"hasta"`
	Bloques []bloqueCalendarioResponse `json:"bloques"`
}

func toBloqueCalendarioResponse(b application.BloqueCalendario) bloqueCalendarioResponse {
	return bloqueCalendarioResponse{
		ReservaID:     b.Reserva.ID,
		Fecha:         b.Reserva.Fecha.Format("2006-01-02"),
		HoraInicio:    formatHora(b.Reserva.HoraInicio),
		HoraFin:       formatHora(b.Reserva.HoraFin),
		Estado:        string(b.Reserva.Estado),
		Tipo:          string(b.Reserva.Tipo),
		Docente:       deref(b.Reserva.NombreDocenteSnapshot),
		MateriaNombre: b.MateriaNombre,
		CursoNombre:   b.CursoNombre,
	}
}

// deref: nombre_docente_snapshot es nullable en la base (los bloqueos por
// evaluación estatal no tienen docente).
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// pcDisponibleResponse: RF-04.2 + RF-03.7 — lo que el docente necesita
// para tildar qué PCs reservar (incluye software y freezado para poder
// decidir sin consultar inventory aparte).
type pcDisponibleResponse struct {
	PCID              string `json:"pcId"`
	Identificador     int    `json:"identificador"`
	CarroID           string `json:"carroId"`
	CarroNombre       string `json:"carroNombre"`
	Freezado          bool   `json:"freezado"`
	SoftwareInstalado string `json:"softwareInstalado,omitempty"`
}

type pcsDisponiblesResponse struct {
	Data []pcDisponibleResponse `json:"data"`
}

func toPCDisponibleResponse(p application.PCDisponible) pcDisponibleResponse {
	return pcDisponibleResponse{
		PCID: p.PCID, Identificador: p.Identificador,
		CarroID: p.CarroID, CarroNombre: p.CarroNombre,
		Freezado: p.Freezado, SoftwareInstalado: p.SoftwareInstalado,
	}
}
