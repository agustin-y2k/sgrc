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
	IncidenciaAbierta         EstadoIncidencia = "ABIERTA"
	IncidenciaEnReparacion    EstadoIncidencia = "EN_REPARACION"
	IncidenciaEnviadaASoporte EstadoIncidencia = "ENVIADA_A_SOPORTE"
	IncidenciaResuelta        EstadoIncidencia = "RESUELTA"
)

var ErrEstadoIncidenciaInvalido = errors.New("estado de incidencia inválido")

func ParseEstadoIncidencia(s string) (EstadoIncidencia, error) {
	switch EstadoIncidencia(s) {
	case IncidenciaAbierta, IncidenciaEnReparacion, IncidenciaEnviadaASoporte, IncidenciaResuelta:
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
// ENVIADA_A_SOPORTE a EN_REPARACION si vuelve antes de lo esperado). Por eso acá
// no hay un PuedeTransicionarA como en Usuario/PC — solo se valida que el
// valor en sí sea uno de los cuatro conocidos (ParseEstadoIncidencia).
type Incidencia struct {
	ID                 string
	EquipoID           string
	ReportadoPor       *string
	Descripcion        string
	Gravedad           Gravedad
	Fecha              time.Time
	EnviadoASoporte    bool
	FechaEnvioASoporte *time.Time
	Estado             EstadoIncidencia
	// Categoria es QUÉ tipo de falla es, en texto libre ("batería",
	// "pantalla"). Vacía mientras no se haya podido diagnosticar, que es un
	// estado real y no un dato faltante: una máquina que no enciende tiene
	// una falla perfectamente concreta y ninguna categoría todavía.
	//
	// Es texto libre y no una lista cerrada por lo mismo que el tipo de
	// equipo: cada institución rompe y repara cosas distintas, y una lista
	// fija haría que la primera falla no prevista pida tocar el sistema. Lo
	// que evita que la estadística se fragmente es que el formulario sugiere
	// las ya usadas y que los reportes agrupan sin distinguir mayúsculas.
	Categoria string
}

// MaxLargoCategoriaFalla lo fija la columna (VARCHAR(50) en la 017).
const MaxLargoCategoriaFalla = 50

// ErrCategoriaFallaLarga: el largo se valida acá y no solo en la base para
// que el error sea un 400 con explicación y no un 500 de Postgres.
var ErrCategoriaFallaLarga = fmt.Errorf("la categoría no puede tener más de %d caracteres", MaxLargoCategoriaFalla)

// CategoriaDeFallaValida normaliza y valida. La categoría vacía es válida y
// significa "sin clasificar": ver el comentario del campo.
func CategoriaDeFallaValida(categoria string) (string, error) {
	categoria = strings.Join(strings.Fields(categoria), " ")
	if len([]rune(categoria)) > MaxLargoCategoriaFalla {
		return "", ErrCategoriaFallaLarga
	}
	return categoria, nil
}

func NuevaIncidencia(id, equipoID, reportadoPor, descripcion, categoria string, gravedad Gravedad, fecha time.Time) (*Incidencia, error) {
	if strings.TrimSpace(descripcion) == "" {
		return nil, ErrDescripcionVacia
	}
	categoria, err := CategoriaDeFallaValida(categoria)
	if err != nil {
		return nil, err
	}
	var reportadoPorPtr *string
	if reportadoPor != "" {
		reportadoPorPtr = &reportadoPor
	}
	return &Incidencia{
		ID:           id,
		EquipoID:     equipoID,
		ReportadoPor: reportadoPorPtr,
		Descripcion:  descripcion,
		Categoria:    categoria,
		Gravedad:     gravedad,
		Fecha:        fecha,
		Estado:       IncidenciaAbierta,
	}, nil
}

// MarcarEnviadaASoporte registra que el equipo se mandó a reparar afuera
// (RF-03.6). A dónde depende de la institución: un organismo educativo, un
// proveedor, un taller.
func (i *Incidencia) MarcarEnviadaASoporte(fecha time.Time) {
	i.EnviadoASoporte = true
	i.FechaEnvioASoporte = &fecha
	i.Estado = IncidenciaEnviadaASoporte
}
