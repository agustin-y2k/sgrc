package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
)

// ── Requests ────────────────────────────────────────────────────────────

// entregarPorReservaRequest — se mandan las reservas puntuales (una por PC),
// no el grupo: el retiro es máquina por máquina, porque el docente puede
// llevarse tres de las cinco que reservó.
type entregarPorReservaRequest struct {
	ReservaIDs []string `json:"reservaIds"`
	// RetiradoPor: quién vino a buscarlas, si no fue el docente de la
	// reserva. Pasa seguido —manda a un alumno o a un colega— y el papel que
	// esto reemplaza lo anota.
	//
	// No cambia de quién es la responsabilidad: el préstamo queda igual a
	// nombre del docente, que es quien reservó.
	RetiradoPor string `json:"retiradoPor,omitempty"`
}

// entregarSueltaRequest es el préstamo sin reserva: "necesito una compu para
// hacer un trámite".
type entregarSueltaRequest struct {
	EquipoIDs []string `json:"equipoIds"`
	// Nombre es obligatorio; usuarioId solo si esa persona tiene cuenta.
	// Quien pide una máquina para un trámite muchas veces no la tiene.
	Nombre    string  `json:"nombre"`
	UsuarioID *string `json:"usuarioId,omitempty"`
	// RetiradoPor opcional: si la pide una persona y la viene a buscar otra.
	RetiradoPor string `json:"retiradoPor,omitempty"`
	Motivo      string `json:"motivo,omitempty"`
	// DevolucionEstimada opcional, ISO 8601. Sin ella no se le reclama nada:
	// "vengo en un rato" es la respuesta honesta.
	DevolucionEstimada *time.Time `json:"devolucionEstimada,omitempty"`
}

type recibirRequest struct {
	PrestamoIDs []string `json:"prestamoIds"`
	// Observaciones vale para todo el lote y normalmente va vacía. Si hay
	// algo puntual que anotar sobre una máquina, se recibe esa sola: atarle
	// la observación a cinco filas diría de las otras cuatro algo que no
	// pasó.
	Observaciones string `json:"observaciones,omitempty"`
}

// ── Responses ───────────────────────────────────────────────────────────

type prestamoResponse struct {
	ID        string  `json:"id"`
	EquipoID  string  `json:"equipoId"`
	ReservaID *string `json:"reservaId,omitempty"`

	// entregadoANombre es quién RESPONDE por el equipo; retiradoPor, quién
	// vino a buscarlo si no fue esa misma persona. Ausente es el caso
	// normal y significa que lo retiró quien responde.
	EntregadoAUsuarioID *string `json:"entregadoAUsuarioId,omitempty"`
	EntregadoANombre    string  `json:"entregadoANombre"`
	RetiradoPor         string  `json:"retiradoPor,omitempty"`
	Motivo              string  `json:"motivo,omitempty"`

	DevolucionEstimada *time.Time `json:"devolucionEstimada,omitempty"`
	EntregadoPor       *string    `json:"entregadoPor,omitempty"`
	EntregadoEn        time.Time  `json:"entregadoEn"`
	DevueltoEn         *time.Time `json:"devueltoEn,omitempty"`
	RecibidoPor        *string    `json:"recibidoPor,omitempty"`
	Observaciones      string     `json:"observaciones,omitempty"`

	// Derivados. Viajan resueltos por la misma razón que el contador de las
	// licencias: si los calculara el navegador, un reloj corrido mostraría
	// una demora distinta de la que el sistema va a reclamar.
	Abierto         bool `json:"abierto"`
	Demorado        bool `json:"demorado"`
	MinutosDeDemora int  `json:"minutosDeDemora,omitempty"`

	// Ubicación. Solo en los listados, que es donde hace falta.
	//
	// Etiqueta es cómo se nombra al equipo: "PC 3" o "Proyector Epson". Se
	// resuelve del lado del servidor para que la misma cosa no se vea
	// distinta según la pantalla.
	Identificador int     `json:"identificador,omitempty"`
	Etiqueta      string  `json:"etiqueta,omitempty"`
	CarroNombre   string  `json:"carroNombre,omitempty"`
	MateriaNombre *string `json:"materiaNombre,omitempty"`
}

func toPrestamoResponse(p *domain.Prestamo, ahora time.Time) prestamoResponse {
	return prestamoResponse{
		ID: p.ID, EquipoID: p.EquipoID, ReservaID: p.ReservaID,
		EntregadoAUsuarioID: p.EntregadoAUsuarioID,
		EntregadoANombre:    p.EntregadoANombre,
		RetiradoPor:         p.RetiradoPor,
		Motivo:              p.Motivo,
		DevolucionEstimada:  p.DevolucionEstimada,
		EntregadoPor:        p.EntregadoPor,
		EntregadoEn:         p.EntregadoEn,
		DevueltoEn:          p.DevueltoEn,
		RecibidoPor:         p.RecibidoPor,
		Observaciones:       p.Observaciones,
		Abierto:             p.EstaAbierto(),
		Demorado:            p.Demorado(ahora),
		MinutosDeDemora:     p.MinutosDeDemora(ahora),
	}
}

func toPrestamoDetalladoResponse(d *application.PrestamoDetallado, ahora time.Time) prestamoResponse {
	r := toPrestamoResponse(d.Prestamo, ahora)
	r.Identificador = d.Identificador
	r.Etiqueta = d.Etiqueta
	r.CarroNombre = d.CarroNombre
	r.MateriaNombre = d.MateriaNombre
	return r
}

// pcNoEntregadaResponse lleva un código además del texto para que la
// pantalla pueda ofrecer la acción que corresponde: "ver quién la tiene" no
// es lo mismo que "revisá el inventario".
type pcNoEntregadaResponse struct {
	EquipoID string `json:"equipoId"`
	Razon    string `json:"razon"`
	Detalle  string `json:"detalle"`
}

// reservaProximaResponse avisa que una máquina recién entregada tiene una
// reserva encima. No impidió nada: es información para que el Admin decida.
type reservaProximaResponse struct {
	EquipoID string `json:"equipoId"`
	Fecha    string `json:"fecha"`
	Inicio   string `json:"horaInicio"`
	Fin      string `json:"horaFin"`
	Docente  string `json:"docente,omitempty"`
}

type resultadoEntregaResponse struct {
	Entregadas   []prestamoResponse       `json:"entregadas"`
	NoEntregadas []pcNoEntregadaResponse  `json:"noEntregadas,omitempty"`
	Avisos       []reservaProximaResponse `json:"avisos,omitempty"`
}

func toResultadoEntregaResponse(r *application.ResultadoEntrega, ahora time.Time) resultadoEntregaResponse {
	resp := resultadoEntregaResponse{Entregadas: make([]prestamoResponse, len(r.Entregadas))}
	for i, p := range r.Entregadas {
		resp.Entregadas[i] = toPrestamoResponse(p, ahora)
	}
	for _, n := range r.NoEntregadas {
		resp.NoEntregadas = append(resp.NoEntregadas, pcNoEntregadaResponse{
			EquipoID: n.EquipoID, Razon: string(n.Razon), Detalle: n.Detalle,
		})
	}
	for _, a := range r.Avisos {
		resp.Avisos = append(resp.Avisos, reservaProximaResponse{
			EquipoID: a.EquipoID,
			Fecha:    a.Fecha.Format("2006-01-02"),
			Inicio:   formatHora(a.Inicio),
			Fin:      formatHora(a.Fin),
			Docente:  a.Docente,
		})
	}
	return resp
}

type pcNoRecibidaResponse struct {
	PrestamoID string `json:"prestamoId"`
	Detalle    string `json:"detalle"`
}

type resultadoDevolucionResponse struct {
	Recibidos   []prestamoResponse     `json:"recibidos"`
	NoRecibidos []pcNoRecibidaResponse `json:"noRecibidos,omitempty"`
}

func toResultadoDevolucionResponse(r *application.ResultadoDevolucion, ahora time.Time) resultadoDevolucionResponse {
	resp := resultadoDevolucionResponse{Recibidos: make([]prestamoResponse, len(r.Recibidos))}
	for i, p := range r.Recibidos {
		resp.Recibidos[i] = toPrestamoResponse(p, ahora)
	}
	for _, n := range r.NoRecibidos {
		resp.NoRecibidos = append(resp.NoRecibidos, pcNoRecibidaResponse{
			PrestamoID: n.PrestamoID, Detalle: n.Detalle,
		})
	}
	return resp
}
