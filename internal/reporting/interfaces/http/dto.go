// Package http expone las rutas Fiber de reporting — ver
// docs/08-api-spec.yaml para el contrato completo de cada endpoint.
package http

import "github.com/ramiro/sgrc/internal/reporting/domain"

type resumenUsoPCResponse struct {
	PCID              string `json:"pcId"`
	Identificador     int    `json:"identificador"`
	CarroNombre       string `json:"carroNombre"`
	CantidadReservas  int    `json:"cantidadReservas"`
	MinutosReservados int    `json:"minutosReservados"`
}

func toResumenUsoPCResponse(u domain.ResumenUsoPC) resumenUsoPCResponse {
	return resumenUsoPCResponse{
		PCID: u.PCID, Identificador: u.Identificador, CarroNombre: u.CarroNombre,
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

type historicoUsoPCResponse struct {
	ID                    string `json:"id"`
	Anio                  int    `json:"anio"`
	PCID                  string `json:"pcId"`
	IdentificadorSnapshot int    `json:"identificadorSnapshot"`
	CarroNombreSnapshot   string `json:"carroNombreSnapshot"`
	MinutosReservados     int    `json:"minutosReservados"`
	CantidadReservas      int    `json:"cantidadReservas"`
}

func toHistoricoUsoPCResponse(h *domain.HistoricoUsoPC) historicoUsoPCResponse {
	return historicoUsoPCResponse{
		ID: h.ID, Anio: h.Anio, PCID: h.PCID, IdentificadorSnapshot: h.IdentificadorSnapshot,
		CarroNombreSnapshot: h.CarroNombreSnapshot, MinutosReservados: h.MinutosReservados, CantidadReservas: h.CantidadReservas,
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

type resumenIncidenciasPCResponse struct {
	PCID          string `json:"pcId"`
	Identificador int    `json:"identificador"`
	CarroNombre   string `json:"carroNombre"`
	Total         int    `json:"total"`
	Abiertas      int    `json:"abiertas"`
	EnReparacion  int    `json:"enReparacion"`
	EnviadasDGE   int    `json:"enviadasDge"`
	Resueltas     int    `json:"resueltas"`
	Graves        int    `json:"graves"`
}

func toResumenIncidenciasPCResponse(x domain.ResumenIncidenciasPC) resumenIncidenciasPCResponse {
	return resumenIncidenciasPCResponse{
		PCID: x.PCID, Identificador: x.Identificador, CarroNombre: x.CarroNombre,
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
