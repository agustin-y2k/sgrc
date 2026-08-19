package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// formatoFecha es el mismo de reservation y availability: ISO sin hora. Las
// fechas de una licencia son días de calendario, no instantes.
const formatoFecha = "2006-01-02"

// parsearFechaOpcional convierte "2026-09-03" a time.Time.
func parsearFechaOpcional(campo string, valor *string) (*time.Time, error) {
	if valor == nil || *valor == "" {
		return nil, nil
	}
	t, err := time.Parse(formatoFecha, *valor)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest,
			campo+" tiene que ser una fecha con formato AAAA-MM-DD")
	}
	return &t, nil
}

func formatearFechaOpcional(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(formatoFecha)
	return &s
}

// ── Requests ────────────────────────────────────────────────────────────

// vencimientoRequest son las tres formas de declarar el vencimiento.
type vencimientoRequest struct {
	// RenovadaEl: "la renové el martes". Se le suman los días de duración.
	RenovadaEl *string `json:"renovadaEl,omitempty"`
	// QuedanDias: "quedan 12 días", que es lo que muestra la máquina.
	QuedanDias *int `json:"quedanDias,omitempty"`
	// VenceEl: la fecha de vencimiento, tal cual.
	VenceEl *string `json:"venceEl,omitempty"`
}

func (v vencimientoRequest) aDominio() (application.VencimientoDeclarado, error) {
	renovadaEl, err := parsearFechaOpcional("renovadaEl", v.RenovadaEl)
	if err != nil {
		return application.VencimientoDeclarado{}, err
	}
	venceEl, err := parsearFechaOpcional("venceEl", v.VenceEl)
	if err != nil {
		return application.VencimientoDeclarado{}, err
	}
	return application.VencimientoDeclarado{
		RenovadaEl: renovadaEl,
		QuedanDias: v.QuedanDias,
		VenceEl:    venceEl,
	}, nil
}

// diasAvisoPorDefecto: avisar el día anterior.
const diasAvisoPorDefecto = 1

type crearLicenciasRequest struct {
	// EquipoIDs en plural: el alta es masiva porque el mismo software está en
	// varias máquinas y cargarlo de a una serían ocho formularios iguales.
	EquipoIDs    []string `json:"equipoIds"`
	Nombre       string   `json:"nombre"`
	DiasDuracion int      `json:"diasDuracion"`
	// DiasAviso opcional: sin él, se avisa el día anterior.
	DiasAviso          *int `json:"diasAviso,omitempty"`
	vencimientoRequest      // los tres campos van planos en el JSON
}

type renovarLicenciasRequest struct {
	LicenciaIDs []string `json:"licenciaIds"`
	// RenovadaEl ausente significa "hoy", que es el caso normal. Con fecha,
	// es el del olvido: se renovó el martes y se carga el jueves.
	RenovadaEl *string `json:"renovadaEl,omitempty"`
}

type editarLicenciaRequest struct {
	Nombre       *string `json:"nombre,omitempty"`
	DiasDuracion *int    `json:"diasDuracion,omitempty"`
	DiasAviso    *int    `json:"diasAviso,omitempty"`
	vencimientoRequest
}

// ── Responses ───────────────────────────────────────────────────────────

type licenciaResponse struct {
	ID           string `json:"id"`
	EquipoID     string `json:"equipoId"`
	Nombre       string `json:"nombre"`
	DiasDuracion int    `json:"diasDuracion"`
	DiasAviso    int    `json:"diasAviso"`

	// FechaVencimiento ausente = a verificar, no "no vence nunca" (RF-03.13).
	FechaVencimiento *string `json:"fechaVencimiento,omitempty"`
	UltimaRenovacion *string `json:"ultimaRenovacion,omitempty"`

	VencimientoFijadoPor *string    `json:"vencimientoFijadoPor,omitempty"`
	VencimientoFijadoEn  *time.Time `json:"vencimientoFijadoEn,omitempty"`

	// DiasRestantes y Estado son derivados: no están en la base, se calculan
	// contra el día de hoy.
	DiasRestantes *int   `json:"diasRestantes,omitempty"`
	Estado        string `json:"estado"`

	// Ubicación.
	Etiqueta         string `json:"etiqueta,omitempty"`
	Identificador    int    `json:"identificador,omitempty"`
	CarroID          string `json:"carroId,omitempty"`
	CarroNombre      string `json:"carroNombre,omitempty"`
	EquipoDadoDeBaja bool   `json:"equipoDadoDeBaja,omitempty"`
}

func toLicenciaResponse(l *domain.LicenciaSoftware, hoy time.Time) licenciaResponse {
	r := licenciaResponse{
		ID: l.ID, EquipoID: l.EquipoID, Nombre: l.Nombre,
		DiasDuracion: l.DiasDuracion, DiasAviso: l.DiasAviso,
		FechaVencimiento:     formatearFechaOpcional(l.FechaVencimiento),
		UltimaRenovacion:     formatearFechaOpcional(l.UltimaRenovacion),
		VencimientoFijadoPor: l.VencimientoFijadoPor,
		VencimientoFijadoEn:  l.VencimientoFijadoEn,
		Estado:               string(l.Estado(hoy)),
	}
	if dias, tiene := l.DiasRestantes(hoy); tiene {
		r.DiasRestantes = &dias
	}
	return r
}

func toLicenciaConUbicacionResponse(u *application.LicenciaConUbicacion, hoy time.Time) licenciaResponse {
	r := toLicenciaResponse(u.Licencia, hoy)
	r.Etiqueta = u.Etiqueta
	r.Identificador = u.Identificador
	r.CarroID = u.CarroID
	r.CarroNombre = u.CarroNombre
	r.EquipoDadoDeBaja = u.EquipoDadoDeBaja
	return r
}

// altaMasivaResponse dice qué pasó con cada equipo del lote.
type altaMasivaResponse struct {
	Creadas              []licenciaResponse `json:"creadas"`
	EquiposQueYaLaTenian []string           `json:"equiposQueYaLaTenian,omitempty"`
}

// renovacionResponse — SinFechaPrevia son las que no se pudieron renovar
// porque todavía no tienen vencimiento cargado.
type renovacionResponse struct {
	Renovadas      []licenciaResponse `json:"renovadas"`
	SinFechaPrevia []string           `json:"sinFechaPrevia,omitempty"`
}
