package http

import (
	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
)

// ── Requests ────────────────────────────────────────────────────────────

// marcarPreferenciaRequest: la misma marca aplicada a varios equipos.
type marcarPreferenciaRequest struct {
	EquipoIDs     []string `json:"equipoIds"`
	MateriaNombre string   `json:"materiaNombre"`
	// En reglas generales los ejes ausentes no acotan. Con CursoExacto,
	// el año es obligatorio y modalidad/división ausente exige su ausencia.
	Modalidad *string `json:"modalidad,omitempty"`
	Anio      *int    `json:"anio,omitempty"`
	Division  *string `json:"division,omitempty"`
	// Prioridad ausente = 1, la más fuerte (ver prioridadPorDefecto).
	Prioridad   *int `json:"prioridad,omitempty"`
	CursoExacto bool `json:"cursoExacto,omitempty"`
}

// editarPreferenciaRequest no lleva la materia: cambiarla es otra marca, no
// una corrección de esta.
type editarPreferenciaRequest struct {
	Modalidad   *string `json:"modalidad,omitempty"`
	Anio        *int    `json:"anio,omitempty"`
	Division    *string `json:"division,omitempty"`
	Prioridad   *int    `json:"prioridad,omitempty"`
	CursoExacto *bool   `json:"cursoExacto,omitempty"`
}

// ── Responses ───────────────────────────────────────────────────────────

type preferenciaResponse struct {
	ID            string  `json:"id"`
	EquipoID      string  `json:"equipoId"`
	MateriaNombre string  `json:"materiaNombre"`
	Modalidad     *string `json:"modalidad,omitempty"`
	Anio          *int    `json:"anio,omitempty"`
	Division      *string `json:"division,omitempty"`
	Prioridad     int     `json:"prioridad"`
	CursoExacto   bool    `json:"cursoExacto"`
	// Alcance es la frase ya armada ("Dibujo Técnico de 4°2, Electromecánica").
	Alcance string `json:"alcance"`
}

func toPreferenciaResponse(p *domain.PreferenciaDeEquipo) preferenciaResponse {
	return preferenciaResponse{
		ID: p.ID, EquipoID: p.EquipoID, MateriaNombre: p.MateriaNombre,
		Modalidad: p.Modalidad, Anio: p.Anio, Division: p.Division,
		Prioridad: p.Prioridad, CursoExacto: p.CursoExacto,
		Alcance: p.Alcance(),
	}
}

// altaDePreferenciasResponse separa lo creado de lo que ya estaba marcado.
type altaDePreferenciasResponse struct {
	Creadas          []preferenciaResponse `json:"creadas"`
	EquiposQueYaTeni []string              `json:"equiposQueYaLaTenian,omitempty"`
}
