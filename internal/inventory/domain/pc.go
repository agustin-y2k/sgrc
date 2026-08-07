package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Estado de una PC (RF-03.3). Ver docs/05-diagramas-estado.md — a
// diferencia de Usuario, FUERA_DE_SERVICIO es terminal en este diagrama:
// no hay transición de vuelta a DISPONIBLE ni a EN_MANTENIMIENTO. Si en la
// práctica una PC "fuera de servicio" se repara, la forma modelada de
// volver a operarla es que un Admin la edite directamente (no es una
// transición de estado, es una corrección de datos).
type EstadoPC string

const (
	EstadoDisponible      EstadoPC = "DISPONIBLE"
	EstadoEnMantenimiento EstadoPC = "EN_MANTENIMIENTO"
	EstadoFueraDeServicio EstadoPC = "FUERA_DE_SERVICIO"
)

var ErrEstadoPCInvalido = errors.New("estado de PC inválido")

func ParseEstadoPC(s string) (EstadoPC, error) {
	switch EstadoPC(s) {
	case EstadoDisponible, EstadoEnMantenimiento, EstadoFueraDeServicio:
		return EstadoPC(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrEstadoPCInvalido, s)
	}
}

// PuedeTransicionarA implementa el diagrama de estados de PC
// (docs/05-diagramas-estado.md): DISPONIBLE y EN_MANTENIMIENTO se
// alternan libremente entre sí y hacia FUERA_DE_SERVICIO; FUERA_DE_SERVICIO
// es terminal.
func (e EstadoPC) PuedeTransicionarA(nuevo EstadoPC) bool {
	switch e {
	case EstadoDisponible:
		return nuevo == EstadoEnMantenimiento || nuevo == EstadoFueraDeServicio
	case EstadoEnMantenimiento:
		return nuevo == EstadoDisponible || nuevo == EstadoFueraDeServicio
	case EstadoFueraDeServicio:
		return false
	default:
		return false
	}
}

var ErrTransicionEstadoPCInvalida = errors.New("transición de estado de PC inválida")

// MaxLargoNumeroSerie es el tope del VARCHAR(50) de la migración 011. Se
// valida acá además de en la base para que el error salga como un 400 con
// explicación y no como un 500 de Postgres.
const MaxLargoNumeroSerie = 50

var (
	// El identificador sí es un entero positivo: es la etiqueta "PC 1",
	// "PC 2" que se le pone al equipo dentro de su carro, y la elige la
	// escuela.
	ErrIdentificadorInvalido = errors.New("el identificador de la PC debe ser un entero positivo")
	// El número de serie NO es un número, aunque se llame así: es el código
	// de fábrica de la etiqueta y casi siempre trae letras ("5CD1234ABC").
	// Hasta la migración 011 la columna era BIGINT, y la primera PC que
	// alguien cargaba con el código real no entraba.
	ErrNumeroSerieInvalido = errors.New("el número de serie no puede estar vacío")
	ErrNumeroSerieLargo    = fmt.Errorf("el número de serie no puede tener más de %d caracteres", MaxLargoNumeroSerie)
	ErrPCYaDadaDeBaja      = errors.New("la PC ya está dada de baja")
)

// NormalizarNumeroSerie devuelve la forma canónica: sin espacios al borde y
// en mayúsculas. Es la misma decisión que NormalizarEmail en auth y por el
// mismo motivo — sin una forma única, "5cd1234abc" y "5CD1234ABC" son dos
// filas distintas para el UNIQUE, o sea la misma máquina cargada dos veces.
//
// Mayúsculas y no minúsculas porque es como vienen impresas las etiquetas:
// la forma canónica coincide con lo que se lee en el equipo.
func NormalizarNumeroSerie(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// PC es el equipo individual dentro de un Carro.
//
// Identificador es único solo dentro de su Carro (no globalmente) — "PC 27"
// puede existir en el Carro 1 y en el Carro 2 sin conflicto. NumeroSerie sí
// es único en toda la institución (es el de fábrica), y es texto: el nombre
// engaña, pero el código de la etiqueta lleva letras.
//
// Freezado es puramente informativo (Deep Freeze instalado) — no afecta
// ningún flujo de reservas ni de negocio.
type PC struct {
	ID                string
	CarroID           string
	Identificador     int
	NumeroSerie       string
	Freezado          bool
	CPU               string
	RAM               string
	SistemaOperativo  string
	SoftwareInstalado string
	Estado            EstadoPC
	DadaDeBaja        bool
	FechaBaja         *time.Time
	FechaAlta         time.Time
}

func NuevaPC(id, carroID string, identificador int, numeroSerie string, freezado bool, fechaAlta time.Time) (*PC, error) {
	if identificador <= 0 {
		return nil, ErrIdentificadorInvalido
	}
	// Normalizar antes de validar: si no, un número de serie de puros
	// espacios pasaría el "no vacío" y llegaría a la base a chocar contra
	// el CHECK de la 011, que responde 500 en vez de explicar qué falta.
	serie := NormalizarNumeroSerie(numeroSerie)
	if serie == "" {
		return nil, ErrNumeroSerieInvalido
	}
	if len(serie) > MaxLargoNumeroSerie {
		return nil, ErrNumeroSerieLargo
	}
	return &PC{
		ID:            id,
		CarroID:       carroID,
		Identificador: identificador,
		NumeroSerie:   serie,
		Freezado:      freezado,
		Estado:        EstadoDisponible,
		FechaAlta:     fechaAlta,
	}, nil
}

// CambiarEstado aplica una transición si es válida (ver
// EstadoPC.PuedeTransicionarA). No dispara ninguna cascada de cancelación
// de reservas acá — eso es responsabilidad de application/, que llega a
// reservation por un puerto (ValidadorReservas); el dominio no conoce
// infraestructura.
func (p *PC) CambiarEstado(nuevo EstadoPC) error {
	if !p.Estado.PuedeTransicionarA(nuevo) {
		return fmt.Errorf("%w: de %s a %s", ErrTransicionEstadoPCInvalida, p.Estado, nuevo)
	}
	p.Estado = nuevo
	return nil
}

// DarDeBaja marca la PC como dada de baja (soft delete, RF-03.4) — la fila
// se conserva para no perder el historial de incidencias y reservas.
func (p *PC) DarDeBaja(ahora time.Time) error {
	if p.DadaDeBaja {
		return ErrPCYaDadaDeBaja
	}
	p.DadaDeBaja = true
	p.FechaBaja = &ahora
	return nil
}

// MoverACarro cambia el carro al que pertenece la PC (RF-03.10). La
// unicidad del identificador en el carro destino se valida en
// infrastructure (constraint UNIQUE(carro_id, identificador)), no acá —
// el dominio no tiene visibilidad de qué otras PCs existen.
func (p *PC) MoverACarro(nuevoCarroID string) {
	p.CarroID = nuevoCarroID
}
