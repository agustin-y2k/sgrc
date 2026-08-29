package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
)

// ── Requests ────────────────────────────────────────────────────────────

// pedirMateriaRequest: o se elige una materia de la lista (MateriaID), o se
// escribe cuál es cuando todavía no existe (MateriaSolicitada, y el curso
// donde va).
type pedirMateriaRequest struct {
	MateriaID         string `json:"materiaId,omitempty"`
	CursoSolicitado   string `json:"cursoSolicitado,omitempty"`
	MateriaSolicitada string `json:"materiaSolicitada,omitempty"`
	Motivo            string `json:"motivo"`
}

type resolverPedidoRequest struct {
	Aprobar   bool   `json:"aprobar"`
	Respuesta string `json:"respuesta,omitempty"`
	// CursoID solo hace falta al aprobar un pedido de una materia que no
	// existe: es dónde se la crea. El sistema no lo deduce del texto.
	CursoID string `json:"cursoId,omitempty"`
	// Rol con el que queda asignado. Vacío = TITULAR.
	Rol string `json:"rol,omitempty"`
}

// ── Responses ───────────────────────────────────────────────────────────

type pedidoResponse struct {
	ID        string `json:"id"`
	UsuarioID string `json:"usuarioId"`

	MateriaID         *string `json:"materiaId,omitempty"`
	CursoSolicitado   string  `json:"cursoSolicitado,omitempty"`
	MateriaSolicitada string  `json:"materiaSolicitada,omitempty"`
	// Cómo se llama la materia que ya existe, con su curso, para poder
	// nombrarla en pantalla. Vacíos en un pedido de materia nueva —ahí lo que
	// se pidió son los dos campos de arriba— y también en la respuesta a
	// resolver un pedido, que la pantalla no dibuja: vuelve a pedir la lista.
	MateriaNombre string `json:"materiaNombre,omitempty"`
	CursoNombre   string `json:"cursoNombre,omitempty"`
	// Quién lo pidió. Lo usa la bandeja del Admin, que decide sobre una
	// persona: en el perfil de quien pidió no se dibuja, ahí ya se sabe.
	DocenteNombre string `json:"docenteNombre,omitempty"`
	// EsMateriaNueva le dice a la pantalla que, para aprobar, hay que elegir
	// en qué curso crear la materia.
	EsMateriaNueva bool `json:"esMateriaNueva"`

	Motivo      string     `json:"motivo"`
	Estado      string     `json:"estado"`
	Respuesta   string     `json:"respuesta,omitempty"`
	ResueltoPor *string    `json:"resueltoPor,omitempty"`
	ResueltoEn  *time.Time `json:"resueltoEn,omitempty"`
	CreadoEn    time.Time  `json:"creadoEn"`
}

// toPedidoDetalladoResponse es la de los listados: la misma respuesta más el
// nombre de la materia y el del curso.
func toPedidoDetalladoResponse(d *application.PedidoDetallado) pedidoResponse {
	r := toPedidoResponse(d.Pedido)
	r.MateriaNombre = d.MateriaNombre
	r.CursoNombre = d.CursoNombre
	r.DocenteNombre = d.DocenteNombre
	return r
}

func toPedidoResponse(p *domain.PedidoDeMateria) pedidoResponse {
	return pedidoResponse{
		ID: p.ID, UsuarioID: p.UsuarioID,
		MateriaID: p.MateriaID, CursoSolicitado: p.CursoSolicitado,
		MateriaSolicitada: p.MateriaSolicitada, EsMateriaNueva: p.EsMateriaNueva(),
		Motivo: p.Motivo, Estado: string(p.Estado), Respuesta: p.Respuesta,
		ResueltoPor: p.ResueltoPor, ResueltoEn: p.ResueltoEn, CreadoEn: p.CreadoEn,
	}
}
