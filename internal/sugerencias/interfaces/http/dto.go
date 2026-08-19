package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/sugerencias/domain"
)

// ── Requests ────────────────────────────────────────────────────────────

type escribirRequest struct {
	Tipo  string `json:"tipo"` // SUGERENCIA | PROBLEMA
	Texto string `json:"texto"`
	// Pantalla la manda la interfaz (la ruta desde la que se escribió), no
	// la persona. Ver domain.Sugerencia.
	Pantalla string `json:"pantalla,omitempty"`
	Version  string `json:"version,omitempty"`
}

type responderRequest struct {
	Respuesta string `json:"respuesta"`
}

// ── Responses ───────────────────────────────────────────────────────────

type sugerenciaResponse struct {
	ID       string `json:"id"`
	Tipo     string `json:"tipo"`
	Texto    string `json:"texto"`
	Pantalla string `json:"pantalla,omitempty"`
	Version  string `json:"version,omitempty"`
	Estado   string `json:"estado"`

	// Quien escribió. Solo viaja en el listado del Admin: en el propio ya se
	// sabe de quién son.
	UsuarioID string `json:"usuarioId,omitempty"`

	Respuesta     string     `json:"respuesta,omitempty"`
	RespondidaPor *string    `json:"respondidaPor,omitempty"`
	RespondidaEn  *time.Time `json:"respondidaEn,omitempty"`
	CreadaEn      time.Time  `json:"creadaEn"`
}

func toSugerenciaResponse(s *domain.Sugerencia, conAutor bool) sugerenciaResponse {
	r := sugerenciaResponse{
		ID: s.ID, Tipo: string(s.Tipo), Texto: s.Texto,
		Pantalla: s.Pantalla, Version: s.Version, Estado: string(s.Estado),
		Respuesta: s.Respuesta, RespondidaPor: s.RespondidaPor,
		RespondidaEn: s.RespondidaEn, CreadaEn: s.CreadaEn,
	}
	if conAutor {
		r.UsuarioID = s.UsuarioID
	}
	return r
}
