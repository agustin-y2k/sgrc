// Package http expone las rutas Fiber de reporting — ver
// docs/08-api-spec.yaml para el contrato completo de cada endpoint.
package http

import "github.com/ramiro/sgrc/internal/reporting/domain"

type resumenUsoEquipoResponse struct {
	EquipoID string `json:"equipoId"`
	// Etiqueta es lo que se muestra: "PC 3" o "Proyector Epson". Los dos de
	// abajo van en 0 y "" cuando el equipo no está en ningún carro (015).
	Etiqueta          string `json:"etiqueta"`
	Identificador     int    `json:"identificador"`
	CarroNombre       string `json:"carroNombre"`
	CantidadReservas  int    `json:"cantidadReservas"`
	MinutosReservados int    `json:"minutosReservados"`
}

func toResumenUsoEquipoResponse(u domain.ResumenUsoEquipo) resumenUsoEquipoResponse {
	return resumenUsoEquipoResponse{
		EquipoID: u.EquipoID, Etiqueta: u.Etiqueta, Identificador: u.Identificador, CarroNombre: u.CarroNombre,
		CantidadReservas: u.CantidadReservas, MinutosReservados: u.MinutosReservados,
	}
}

type resumenUsoDocenteResponse struct {
	UsuarioID         string `json:"usuarioId"`
	NombreDocente     string `json:"nombreDocente"`
	CantidadReservas  int    `json:"cantidadReservas"`
	MinutosReservados int    `json:"minutosReservados"`
}

func toResumenUsoDocenteResponse(u domain.ResumenUsoDocente) resumenUsoDocenteResponse {
	return resumenUsoDocenteResponse{
		UsuarioID: u.UsuarioID, NombreDocente: u.NombreDocente,
		CantidadReservas: u.CantidadReservas, MinutosReservados: u.MinutosReservados,
	}
}

type historicoUsoEquipoResponse struct {
	ID       string `json:"id"`
	Anio     int    `json:"anio"`
	EquipoID string `json:"equipoId"`
	// Cómo se llamaba el equipo el día que se archivó el ciclo: "PC 3" o
	// "Proyector Epson". Los dos de abajo van en 0 y "" si no estaba en
	// ningún carro (015).
	EtiquetaSnapshot      string `json:"etiquetaSnapshot"`
	IdentificadorSnapshot int    `json:"identificadorSnapshot"`
	CarroNombreSnapshot   string `json:"carroNombreSnapshot"`
	MinutosReservados     int    `json:"minutosReservados"`
	CantidadReservas      int    `json:"cantidadReservas"`
}

func toHistoricoUsoEquipoResponse(h *domain.HistoricoUsoEquipo) historicoUsoEquipoResponse {
	return historicoUsoEquipoResponse{
		ID: h.ID, Anio: h.Anio, EquipoID: h.EquipoID, EtiquetaSnapshot: h.EtiquetaSnapshot,
		IdentificadorSnapshot: h.IdentificadorSnapshot,
		CarroNombreSnapshot:   h.CarroNombreSnapshot, MinutosReservados: h.MinutosReservados, CantidadReservas: h.CantidadReservas,
	}
}

type historicoUsoDocenteResponse struct {
	ID                    string  `json:"id"`
	Anio                  int     `json:"anio"`
	UsuarioID             *string `json:"usuarioId,omitempty"`
	NombreDocenteSnapshot string  `json:"nombreDocenteSnapshot"`
	CantidadReservas      int     `json:"cantidadReservas"`
	MinutosTotales        int     `json:"minutosTotales"`
}

func toHistoricoUsoDocenteResponse(h *domain.HistoricoUsoDocente) historicoUsoDocenteResponse {
	return historicoUsoDocenteResponse{
		ID: h.ID, Anio: h.Anio, UsuarioID: h.UsuarioID, NombreDocenteSnapshot: h.NombreDocenteSnapshot,
		CantidadReservas: h.CantidadReservas, MinutosTotales: h.MinutosTotales,
	}
}

// ── RF-06.3: incidencias por equipo y por carro ────────────────────────

type resumenIncidenciasEquipoResponse struct {
	PCID string `json:"equipoId"`
	// Ver resumenUsoPCResponse.Etiqueta.
	Etiqueta      string `json:"etiqueta"`
	Identificador int    `json:"identificador"`
	CarroNombre   string `json:"carroNombre"`
	Total         int    `json:"total"`
	Abiertas      int    `json:"abiertas"`
	EnReparacion  int    `json:"enReparacion"`
	EnviadasDGE   int    `json:"enviadasDge"`
	Resueltas     int    `json:"resueltas"`
	Graves        int    `json:"graves"`
}

func toResumenIncidenciasEquipoResponse(x domain.ResumenIncidenciasEquipo) resumenIncidenciasEquipoResponse {
	return resumenIncidenciasEquipoResponse{
		PCID: x.EquipoID, Etiqueta: x.Etiqueta, Identificador: x.Identificador, CarroNombre: x.CarroNombre,
		Total: x.Total, Abiertas: x.Abiertas, EnReparacion: x.EnReparacion,
		EnviadasDGE: x.EnviadasDGE, Resueltas: x.Resueltas, Graves: x.Graves,
	}
}

type resumenIncidenciasCarroResponse struct {
	CarroID     string `json:"carroId"`
	CarroNombre string `json:"carroNombre"`
	Total       int    `json:"total"`
	Abiertas    int    `json:"abiertas"`
	Graves      int    `json:"graves"`
}

func toResumenIncidenciasCarroResponse(x domain.ResumenIncidenciasCarro) resumenIncidenciasCarroResponse {
	return resumenIncidenciasCarroResponse{
		CarroID: x.CarroID, CarroNombre: x.CarroNombre,
		Total: x.Total, Abiertas: x.Abiertas, Graves: x.Graves,
	}
}
