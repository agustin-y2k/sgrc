package domain

import (
	"errors"
	"fmt"
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

// ErrIdentificadorInvalido / ErrNumeroSerieInvalido: ambos deben ser
// positivos — no tiene sentido un identificador o número de serie
// negativo o cero.
var (
	ErrIdentificadorInvalido = errors.New("el identificador de la PC debe ser un entero positivo")
	ErrNumeroSerieInvalido   = errors.New("el número de serie debe ser un entero positivo")
	ErrPCYaDadaDeBaja        = errors.New("la PC ya está dada de baja")
)

// PC es el equipo individual dentro de un Carro.
//
// Identificador es único solo dentro de su Carro (no globalmente) — "PC 27"
// puede existir en el Carro 1 y en el Carro 2 sin conflicto. NumeroSerie sí
// es único en toda la institución (es el de fábrica).
//
// Freezado es puramente informativo (Deep Freeze instalado) — no afecta
// ningún flujo de reservas ni de negocio.
type PC struct {
	ID                string
	CarroID           string
	Identificador     int
	NumeroSerie       int64
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

func NuevaPC(id, carroID string, identificador int, numeroSerie int64, freezado bool, fechaAlta time.Time) (*PC, error) {
	if identificador <= 0 {
		return nil, ErrIdentificadorInvalido
	}
	if numeroSerie <= 0 {
		return nil, ErrNumeroSerieInvalido
	}
	return &PC{
		ID:            id,
		CarroID:       carroID,
		Identificador: identificador,
		NumeroSerie:   numeroSerie,
		Freezado:      freezado,
		Estado:        EstadoDisponible,
		FechaAlta:     fechaAlta,
	}, nil
}

// CambiarEstado aplica una transición si es válida (ver
// EstadoPC.PuedeTransicionarA). No dispara ninguna cascada de cancelación
// de reservas acá — eso es una responsabilidad de application/ (necesita
// el puerto hacia reservation, ver el TODO en application.Service).
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
