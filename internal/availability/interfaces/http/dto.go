// Package http expone las rutas Fiber de availability — ver
// docs/08-api-spec.yaml para el contrato completo de cada endpoint.
package http

import (
	"github.com/ramiro/sgrc/internal/availability/application"
	"github.com/ramiro/sgrc/internal/availability/domain"
)

// ── Requests ────────────────────────────────────────────────────────────
//
// HoraInicio/HoraFin viajan como "HH:MM" en el JSON — se parsean a
// time.Duration en los handlers (ver parseHora en parsing.go).

type bloqueRequest struct {
	DiaSemana  string `json:"diaSemana"`
	HoraInicio string `json:"horaInicio"`
	HoraFin    string `json:"horaFin"`
}

// editarBloqueRequest: todos los campos opcionales — PATCH parcial (RF-07.3).
type editarBloqueRequest struct {
	DiaSemana  *string `json:"diaSemana"`
	HoraInicio *string `json:"horaInicio"`
	HoraFin    *string `json:"horaFin"`
}

type excepcionRequest struct {
	Fecha      string  `json:"fecha"`
	Tipo       string  `json:"tipo"`
	HoraInicio *string `json:"horaInicio"`
	HoraFin    *string `json:"horaFin"`
	Motivo     *string `json:"motivo"`
}

// ── Responses ───────────────────────────────────────────────────────────

type bloqueResponse struct {
	ID         string `json:"id"`
	DiaSemana  string `json:"diaSemana"`
	HoraInicio string `json:"horaInicio"`
	HoraFin    string `json:"horaFin"`
}

func toBloqueResponse(b *domain.BloqueHorario) bloqueResponse {
	return bloqueResponse{
		ID:         b.ID,
		DiaSemana:  string(b.DiaSemana),
		HoraInicio: formatHora(b.HoraInicio),
		HoraFin:    formatHora(b.HoraFin),
	}
}

type excepcionResponse struct {
	ID         string  `json:"id"`
	Fecha      string  `json:"fecha"`
	Tipo       string  `json:"tipo"`
	HoraInicio *string `json:"horaInicio,omitempty"`
	HoraFin    *string `json:"horaFin,omitempty"`
	Motivo     *string `json:"motivo,omitempty"`
}

func toExcepcionResponse(e *domain.Excepcion) excepcionResponse {
	resp := excepcionResponse{
		ID: e.ID, Fecha: e.Fecha.Format("2006-01-02"), Tipo: string(e.Tipo), Motivo: e.Motivo,
	}
	if e.HoraInicio != nil {
		s := formatHora(*e.HoraInicio)
		resp.HoraInicio = &s
	}
	if e.HoraFin != nil {
		s := formatHora(*e.HoraFin)
		resp.HoraFin = &s
	}
	return resp
}

type adminDisponibilidadResponse struct {
	UsuarioID       string             `json:"usuarioId"`
	Nombre          string             `json:"nombre"`
	Apellido        string             `json:"apellido"`
	DisponibleAhora bool               `json:"disponibleAhora"`
	ExcepcionHoy    *excepcionResponse `json:"excepcionHoy,omitempty"`
	HorarioSemanal  []bloqueResponse   `json:"horarioSemanal"`
}

func toAdminDisponibilidadResponse(a application.AdminDisponibilidad) adminDisponibilidadResponse {
	horario := make([]bloqueResponse, len(a.HorarioSemanal))
	for i, b := range a.HorarioSemanal {
		horario[i] = toBloqueResponse(b)
	}
	var excepcion *excepcionResponse
	if a.ExcepcionHoy != nil {
		e := toExcepcionResponse(a.ExcepcionHoy)
		excepcion = &e
	}
	return adminDisponibilidadResponse{
		UsuarioID:       a.UsuarioID,
		Nombre:          a.Nombre,
		Apellido:        a.Apellido,
		DisponibleAhora: a.DisponibleAhora,
		ExcepcionHoy:    excepcion,
		HorarioSemanal:  horario,
	}
}
