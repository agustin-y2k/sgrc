// Package http expone las rutas Fiber de reporting — ver
// docs/08-api-spec.yaml para el contrato completo de cada endpoint.
package http

import "github.com/ramiro/sgrc/internal/reporting/domain"

type resumenUsoEquipoResponse struct {
	EquipoID string `json:"equipoId"`
	// Etiqueta es lo que se muestra: "PC 3" o "Proyector Epson". Los dos de
	// abajo van en 0 y "" cuando el equipo no está en ningún carro.
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
	// UsuarioID ausente = la cuenta se eliminó definitivamente (RF-01.9).
	// El nombre sigue estando; lo que no hay es a quién apuntar.
	UsuarioID         *string `json:"usuarioId,omitempty"`
	NombreDocente     string  `json:"nombreDocente"`
	CantidadReservas  int     `json:"cantidadReservas"`
	MinutosReservados int     `json:"minutosReservados"`
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
	// ningún carro.
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
	EquipoID string `json:"equipoId"`
	// Ver resumenUsoEquipoResponse.Etiqueta.
	Etiqueta         string `json:"etiqueta"`
	Identificador    int    `json:"identificador"`
	CarroNombre      string `json:"carroNombre"`
	Total            int    `json:"total"`
	Abiertas         int    `json:"abiertas"`
	EnReparacion     int    `json:"enReparacion"`
	EnviadasASoporte int    `json:"enviadasASoporte"`
	Resueltas        int    `json:"resueltas"`
	Graves           int    `json:"graves"`
}

func toResumenIncidenciasEquipoResponse(x domain.ResumenIncidenciasEquipo) resumenIncidenciasEquipoResponse {
	return resumenIncidenciasEquipoResponse{
		EquipoID: x.EquipoID, Etiqueta: x.Etiqueta, Identificador: x.Identificador, CarroNombre: x.CarroNombre,
		Total: x.Total, Abiertas: x.Abiertas, EnReparacion: x.EnReparacion,
		EnviadasASoporte: x.EnviadasASoporte, Resueltas: x.Resueltas, Graves: x.Graves,
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

// ── RF-06.5: el estado del parque de equipos ────────────────────────────

type estadoDelInventarioResponse struct {
	// carroId y carroNombre vacíos en la fila de los equipos que no están en
	// ningún carro.
	CarroID         string `json:"carroId,omitempty"`
	CarroNombre     string `json:"carroNombre,omitempty"`
	Disponibles     int    `json:"disponibles"`
	EnMantenimiento int    `json:"enMantenimiento"`
	FueraDeServicio int    `json:"fueraDeServicio"`
	// total excluye los dados de baja: el porcentaje que importa es sobre lo
	// que la institución todavía tiene.
	Total int `json:"total"`
}

func toEstadoDelInventarioResponse(e domain.EstadoDelInventario) estadoDelInventarioResponse {
	return estadoDelInventarioResponse{
		CarroID: e.CarroID, CarroNombre: e.CarroNombre,
		Disponibles: e.Disponibles, EnMantenimiento: e.EnMantenimiento,
		FueraDeServicio: e.FueraDeServicio, Total: e.Total,
	}
}

type equipoFueraDeCirculacionResponse struct {
	EquipoID    string `json:"equipoId"`
	Etiqueta    string `json:"etiqueta"`
	CarroNombre string `json:"carroNombre,omitempty"`
	Estado      string `json:"estado"`
	// Los tres siguientes salen de la última incidencia y pueden faltar: una
	// máquina se puede sacar de circulación sin haber reportado ninguna
	// falla, y ese hueco es un dato — nadie escribió qué tiene.
	Categoria        string `json:"categoria,omitempty"`
	UltimaFalla      string `json:"ultimaFalla,omitempty"`
	EstadoIncidencia string `json:"estadoIncidencia,omitempty"`
}

func toEquipoFueraDeCirculacionResponse(e domain.EquipoFueraDeCirculacion) equipoFueraDeCirculacionResponse {
	return equipoFueraDeCirculacionResponse{
		EquipoID: e.EquipoID, Etiqueta: e.Etiqueta, CarroNombre: e.CarroNombre,
		Estado: e.Estado, Categoria: e.Categoria, UltimaFalla: e.UltimaFalla,
		EstadoIncidencia: e.EstadoIncidencia,
	}
}

type resumenPorCategoriaResponse struct {
	// categoria vacía es la fila de las que nadie pudo diagnosticar.
	Categoria         string `json:"categoria,omitempty"`
	Total             int    `json:"total"`
	Abiertas          int    `json:"abiertas"`
	EquiposAlcanzados int    `json:"equiposAlcanzados"`
}

func toResumenPorCategoriaResponse(x domain.ResumenPorCategoriaDeFalla) resumenPorCategoriaResponse {
	return resumenPorCategoriaResponse{
		Categoria: x.Categoria, Total: x.Total,
		Abiertas: x.Abiertas, EquiposAlcanzados: x.EquiposAlcanzados,
	}
}
