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

// parsearFechaOpcional convierte "2026-09-03" a time.Time. Devuelve un 400
// con el campo nombrado en vez de dejar que el valor llegue como cero al
// dominio — una fecha vacía silenciosa terminaría fijando el vencimiento en
// el año 1.
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

// vencimientoRequest son las tres formas de declarar el vencimiento. Está
// embebido en el alta y en la edición para que las dos acepten exactamente
// lo mismo: cargar la fecha por primera vez y corregirla son la misma
// operación con distinto punto de partida.
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

// diasAvisoPorDefecto: avisar el día anterior. Es lo que hace falta para una
// licencia que se renueva sola en cinco minutos; una que dependa de que
// alguien la consiga se configura con más.
const diasAvisoPorDefecto = 1

type crearLicenciasRequest struct {
	// PCIDs en plural: el alta es masiva porque el mismo software está en
	// varias máquinas y cargarlo de a una serían ocho formularios iguales.
	PCIDs        []string `json:"pcIds"`
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
	PCID         string `json:"pcId"`
	Nombre       string `json:"nombre"`
	DiasDuracion int    `json:"diasDuracion"`
	DiasAviso    int    `json:"diasAviso"`

	// FechaVencimiento ausente = a verificar. Ver la migración 012.
	FechaVencimiento *string `json:"fechaVencimiento,omitempty"`
	UltimaRenovacion *string `json:"ultimaRenovacion,omitempty"`

	VencimientoFijadoPor *string    `json:"vencimientoFijadoPor,omitempty"`
	VencimientoFijadoEn  *time.Time `json:"vencimientoFijadoEn,omitempty"`

	// DiasRestantes y Estado son derivados: no están en la base, se
	// calculan contra el día de hoy. Viajan resueltos desde el backend para
	// que la pantalla, el correo y el job digan lo mismo — si el navegador
	// los calculara, bastaría con tener el reloj corrido para que la tabla
	// mostrara un día distinto del que dispara el aviso.
	DiasRestantes *int   `json:"diasRestantes,omitempty"`
	Estado        string `json:"estado"`

	// Ubicación. Solo viene en el listado general; en el de una PC ya se
	// sabe de qué máquina se está hablando.
	PCIdentificador int    `json:"pcIdentificador,omitempty"`
	CarroID         string `json:"carroId,omitempty"`
	CarroNombre     string `json:"carroNombre,omitempty"`
	PCDadaDeBaja    bool   `json:"pcDadaDeBaja,omitempty"`
}

func toLicenciaResponse(l *domain.LicenciaSoftware, hoy time.Time) licenciaResponse {
	r := licenciaResponse{
		ID: l.ID, PCID: l.PCID, Nombre: l.Nombre,
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
	r.PCIdentificador = u.PCIdentificador
	r.CarroID = u.CarroID
	r.CarroNombre = u.CarroNombre
	r.PCDadaDeBaja = u.PCDadaDeBaja
	return r
}

// altaMasivaResponse dice qué pasó con cada PC del lote. PCsQueYaLaTenian
// no es un error: marcar las diez PCs del carro cuando ocho ya estaban
// cargadas tiene que funcionar, y la pantalla lo informa sin alarmar.
type altaMasivaResponse struct {
	Creadas          []licenciaResponse `json:"creadas"`
	PCsQueYaLaTenian []string           `json:"pcsQueYaLaTenian,omitempty"`
}

// renovacionResponse — SinFechaPrevia son las que no se pudieron renovar
// porque todavía no tienen vencimiento cargado. Hay que cargarlas diciendo
// cómo se sabe la fecha, no "renovarlas".
type renovacionResponse struct {
	Renovadas      []licenciaResponse `json:"renovadas"`
	SinFechaPrevia []string           `json:"sinFechaPrevia,omitempty"`
}
