package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Gravedad string

const (
	GravedadLeve     Gravedad = "LEVE"
	GravedadModerada Gravedad = "MODERADA"
	GravedadGrave    Gravedad = "GRAVE"
)

var ErrGravedadInvalida = errors.New("gravedad inválida")

func ParseGravedad(s string) (Gravedad, error) {
	switch Gravedad(s) {
	case GravedadLeve, GravedadModerada, GravedadGrave:
		return Gravedad(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrGravedadInvalida, s)
	}
}

type EstadoIncidencia string

const (
	IncidenciaAbierta      EstadoIncidencia = "ABIERTA"
	IncidenciaEnReparacion EstadoIncidencia = "EN_REPARACION"
	IncidenciaEnviadaDGE   EstadoIncidencia = "ENVIADA_DGE"
	IncidenciaResuelta     EstadoIncidencia = "RESUELTA"
)

var ErrEstadoIncidenciaInvalido = errors.New("estado de incidencia inválido")

func ParseEstadoIncidencia(s string) (EstadoIncidencia, error) {
	switch EstadoIncidencia(s) {
	case IncidenciaAbierta, IncidenciaEnReparacion, IncidenciaEnviadaDGE, IncidenciaResuelta:
		return EstadoIncidencia(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrEstadoIncidenciaInvalido, s)
	}
}

var ErrDescripcionVacia = errors.New("la descripción de la incidencia no puede estar vacía")

// Incidencia es un reporte de falla sobre una PC puntual (RF-03.5). A
// diferencia de Usuario/PC, docs/01-requisitos.md no define una máquina de
// estados estricta para EstadoIncidencia — cualquier transición entre los
// cuatro valores es válida (un Admin puede, por ejemplo, volver de
// ENVIADA_DGE a EN_REPARACION si vuelve antes de lo esperado). Por eso acá
// no hay un PuedeTransicionarA como en Usuario/PC — solo se valida que el
// valor en sí sea uno de los cuatro conocidos (ParseEstadoIncidencia).
type Incidencia struct {
	ID            string
	PCID          string
	ReportadoPor  *string
	Descripcion   string
	Gravedad      Gravedad
	Fecha         time.Time
	EnviadoDGE    bool
	FechaEnvioDGE *time.Time
	Estado        EstadoIncidencia
}

func NuevaIncidencia(id, equipoID, reportadoPor, descripcion string, gravedad Gravedad, fecha time.Time) (*Incidencia, error) {
	if strings.TrimSpace(descripcion) == "" {
		return nil, ErrDescripcionVacia
	}
	var reportadoPorPtr *string
	if reportadoPor != "" {
		reportadoPorPtr = &reportadoPor
	}
	return &Incidencia{
		ID:           id,
		PCID:         equipoID,
		ReportadoPor: reportadoPorPtr,
		Descripcion:  descripcion,
		Gravedad:     gravedad,
		Fecha:        fecha,
		Estado:       IncidenciaAbierta,
	}, nil
}

// MarcarEnviadaDGE registra el envío a soporte técnico DGE (RF-03.6).
func (i *Incidencia) MarcarEnviadaDGE(fecha time.Time) {
	i.EnviadoDGE = true
	i.FechaEnvioDGE = &fecha
	i.Estado = IncidenciaEnviadaDGE
}
