package http

import (
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// ── Requests ────────────────────────────────────────────────────────────

// marcarPreferenciaRequest: la misma marca aplicada a varios equipos.
type marcarPreferenciaRequest struct {
	EquipoIDs     []string `json:"equipoIds"`
	MateriaNombre string   `json:"materiaNombre"`
	Anio          *int     `json:"anio,omitempty"`
	Division      *string  `json:"division,omitempty"`
	// Prioridad ausente = 1, la más fuerte (ver prioridadPorDefecto).
	Prioridad *int `json:"prioridad,omitempty"`
}

// editarPreferenciaRequest no lleva la materia: cambiarla es otra marca, no
// una corrección de esta.
type editarPreferenciaRequest struct {
	Anio      *int    `json:"anio,omitempty"`
	Division  *string `json:"division,omitempty"`
	Prioridad *int    `json:"prioridad,omitempty"`
}

// ── Responses ───────────────────────────────────────────────────────────

type preferenciaResponse struct {
	ID            string  `json:"id"`
	EquipoID      string  `json:"equipoId"`
	MateriaNombre string  `json:"materiaNombre"`
	Anio          *int    `json:"anio,omitempty"`
	Division      *string `json:"division,omitempty"`
	Prioridad     int     `json:"prioridad"`
	// Alcance es la frase ya armada ("Dibujo Técnico de 3°B").
	Alcance string `json:"alcance"`
}

func toPreferenciaResponse(p *domain.PreferenciaDeEquipo) preferenciaResponse {
	return preferenciaResponse{
		ID: p.ID, EquipoID: p.EquipoID, MateriaNombre: p.MateriaNombre,
		Anio: p.Anio, Division: p.Division, Prioridad: p.Prioridad,
		Alcance: p.Alcance(),
	}
}

// altaDePreferenciasResponse separa lo creado de lo que ya estaba marcado.
type altaDePreferenciasResponse struct {
	Creadas          []preferenciaResponse `json:"creadas"`
	EquiposQueYaTeni []string              `json:"equiposQueYaLaTenian,omitempty"`
}
