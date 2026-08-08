// Package http expone las rutas Fiber de inventory — ver
// docs/08-api-spec.yaml para el contrato completo de cada endpoint.
package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// ── Requests ────────────────────────────────────────────────────────────

type crearCarroRequest struct {
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion"`
}

type editarCarroRequest struct {
	Nombre      *string `json:"nombre,omitempty"`
	Descripcion *string `json:"descripcion,omitempty"`
}

type crearPCRequest struct {
	Identificador     int    `json:"identificador"`
	NumeroSerie       string `json:"numeroSerie"`
	Freezado          bool   `json:"freezado"`
	CPU               string `json:"cpu"`
	RAM               string `json:"ram"`
	SistemaOperativo  string `json:"sistemaOperativo"`
	SoftwareInstalado string `json:"softwareInstalado"`
}

// crearEquipoRequest: algo prestable que no es una computadora de un carro
// (015) — un proyector, un cargador, una notebook suelta.
type crearEquipoRequest struct {
	// Tipo es texto libre: la lista de cosas que presta una escuela no es la
	// misma que la de otra.
	Tipo   string `json:"tipo"`
	Nombre string `json:"nombre"`
	// Reservable: si aparece en la lista de equipos libres al reservar. El
	// proyector sí; un cargador se presta en el momento.
	Reservable bool `json:"reservable"`
}

type editarPCRequest struct {
	CarroID           *string `json:"carroId,omitempty"`
	Freezado          *bool   `json:"freezado,omitempty"`
	CPU               *string `json:"cpu,omitempty"`
	RAM               *string `json:"ram,omitempty"`
	SistemaOperativo  *string `json:"sistemaOperativo,omitempty"`
	SoftwareInstalado *string `json:"softwareInstalado,omitempty"`
	Tipo              *string `json:"tipo,omitempty"`
	Nombre            *string `json:"nombre,omitempty"`
	Reservable        *bool   `json:"reservable,omitempty"`
}

type cambiarEstadoPCRequest struct {
	Estado string  `json:"estado"` // DISPONIBLE | EN_MANTENIMIENTO | FUERA_DE_SERVICIO
	Motivo *string `json:"motivo,omitempty"`
}

type crearIncidenciaRequest struct {
	PCID        string `json:"pcId"`
	Descripcion string `json:"descripcion"`
	Gravedad    string `json:"gravedad"` // LEVE | MODERADA | GRAVE
}

type editarIncidenciaRequest struct {
	Estado           *string `json:"estado,omitempty"`
	MarcarEnviadaDGE bool    `json:"marcarEnviadaDGE"`
}

// ── Responses ───────────────────────────────────────────────────────────

type carroResponse struct {
	ID          string `json:"id"`
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion,omitempty"`
}

func toCarroResponse(c *domain.Carro) carroResponse {
	return carroResponse{ID: c.ID, Nombre: c.Nombre, Descripcion: c.Descripcion}
}

type pcResponse struct {
	ID string `json:"id"`
	// Los tres pueden faltar desde la 015: un proyector no está en ningún
	// carro, no es "PC 3" y puede no traer número de serie.
	CarroID       string `json:"carroId,omitempty"`
	Identificador int    `json:"identificador,omitempty"`
	NumeroSerie   string `json:"numeroSerie,omitempty"`
	// Etiqueta es cómo se lo nombra en cualquier pantalla: "PC 3" o
	// "Proyector Epson".
	Etiqueta          string     `json:"etiqueta"`
	Tipo              string     `json:"tipo"`
	Nombre            string     `json:"nombre,omitempty"`
	Reservable        bool       `json:"reservable"`
	Freezado          bool       `json:"freezado"`
	CPU               string     `json:"cpu,omitempty"`
	RAM               string     `json:"ram,omitempty"`
	SistemaOperativo  string     `json:"sistemaOperativo,omitempty"`
	SoftwareInstalado string     `json:"softwareInstalado,omitempty"`
	Estado            string     `json:"estado"`
	DadaDeBaja        bool       `json:"dadaDeBaja"`
	FechaBaja         *time.Time `json:"fechaBaja,omitempty"`
	FechaAlta         time.Time  `json:"fechaAlta"`
}

func toPCResponse(pc *domain.PC) pcResponse {
	return pcResponse{
		ID: pc.ID, CarroID: pc.CarroID, Identificador: pc.Identificador, NumeroSerie: pc.NumeroSerie,
		Etiqueta: pc.Etiqueta(), Tipo: pc.Tipo, Nombre: pc.Nombre, Reservable: pc.Reservable,
		Freezado: pc.Freezado, CPU: pc.CPU, RAM: pc.RAM, SistemaOperativo: pc.SistemaOperativo,
		SoftwareInstalado: pc.SoftwareInstalado, Estado: string(pc.Estado),
		DadaDeBaja: pc.DadaDeBaja, FechaBaja: pc.FechaBaja, FechaAlta: pc.FechaAlta,
	}
}

type cascadaResponse struct {
	ReservasCanceladas  int `json:"reservasCanceladas"`
	DocentesNotificados int `json:"docentesNotificados"`
}

func toCascadaResponse(r *application.ResultadoCascada) cascadaResponse {
	return cascadaResponse{ReservasCanceladas: r.ReservasCanceladas, DocentesNotificados: r.DocentesNotificados}
}

type incidenciaResponse struct {
	ID            string     `json:"id"`
	PCID          string     `json:"pcId"`
	ReportadoPor  *string    `json:"reportadoPor,omitempty"`
	Descripcion   string     `json:"descripcion"`
	Gravedad      string     `json:"gravedad"`
	Fecha         time.Time  `json:"fecha"`
	EnviadoDGE    bool       `json:"enviadoDge"`
	FechaEnvioDGE *time.Time `json:"fechaEnvioDge,omitempty"`
	Estado        string     `json:"estado"`
}

func toIncidenciaResponse(i *domain.Incidencia) incidenciaResponse {
	return incidenciaResponse{
		ID: i.ID, PCID: i.PCID, ReportadoPor: i.ReportadoPor, Descripcion: i.Descripcion,
		Gravedad: string(i.Gravedad), Fecha: i.Fecha, EnviadoDGE: i.EnviadoDGE,
		FechaEnvioDGE: i.FechaEnvioDGE, Estado: string(i.Estado),
	}
}
