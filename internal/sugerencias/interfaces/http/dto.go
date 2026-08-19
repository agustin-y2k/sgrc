package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/sugerencias/domain"
)

// ── Requests ────────────────────────────────────────────────────────────

type escribirRequest struct {
	Tipo   string `json:"tipo"` // AYUDA | PROBLEMA | SUGERENCIA
	Asunto string `json:"asunto"`
	Texto  string `json:"texto"`
	// Pantalla la manda la interfaz (la ruta desde la que se escribió), no
	// la persona. Ver domain.Sugerencia.
	Pantalla string `json:"pantalla,omitempty"`
	Version  string `json:"version,omitempty"`
}

type responderRequest struct {
	Texto string `json:"texto"`
}

// ── Responses ───────────────────────────────────────────────────────────

type mensajeResponse struct {
	ID string `json:"id"`
	// DeAdmin es de qué lado del hilo viene: la pantalla lo usa para alinear
	// y colorear, sin tener que comparar el autor con el usuario de la sesión.
	DeAdmin   bool      `json:"deAdmin"`
	Texto     string    `json:"texto"`
	EscritoEn time.Time `json:"escritoEn"`
}

type sugerenciaResponse struct {
	ID       string `json:"id"`
	Tipo     string `json:"tipo"`
	Asunto   string `json:"asunto"`
	Pantalla string `json:"pantalla,omitempty"`
	Version  string `json:"version,omitempty"`
	Estado   string `json:"estado"`

	// Quien escribió. Solo viaja en el listado del Admin: en el propio ya se
	// sabe de quién son.
	UsuarioID string `json:"usuarioId,omitempty"`

	// EsperaRespuesta: el hilo está abierto y el último que habló fue quien
	// preguntó. Es lo que ordena el trabajo del Admin, y viene calculado del
	// servidor para que las dos pantallas no lo deduzcan distinto.
	EsperaRespuesta bool `json:"esperaRespuesta"`

	Mensajes          []mensajeResponse `json:"mensajes"`
	CreadaEn          time.Time         `json:"creadaEn"`
	UltimaActividadEn time.Time         `json:"ultimaActividadEn"`
}

func toSugerenciaResponse(s *domain.Sugerencia, conAutor bool) sugerenciaResponse {
	mensajes := make([]mensajeResponse, len(s.Mensajes))
	for i, m := range s.Mensajes {
		mensajes[i] = mensajeResponse{
			ID: m.ID, DeAdmin: m.DeAdmin, Texto: m.Texto, EscritoEn: m.EscritoEn,
		}
	}

	r := sugerenciaResponse{
		ID: s.ID, Tipo: string(s.Tipo), Asunto: s.Asunto,
		Pantalla: s.Pantalla, Version: s.Version, Estado: string(s.Estado),
		EsperaRespuesta: s.EsperaRespuestaDelAdmin(),
		Mensajes:        mensajes,
		CreadaEn:        s.CreadaEn, UltimaActividadEn: s.UltimaActividadEn,
	}
	if conAutor {
		r.UsuarioID = s.UsuarioID
	}
	return r
}
