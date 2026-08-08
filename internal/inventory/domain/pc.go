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
	ID string
	// CarroID vacío = no está en ningún carro. Desde la 015 esto es
	// legítimo: un proyector o un cargador no pertenecen a ninguno.
	CarroID string
	// Identificador 0 y NumeroSerie vacío = no aplica. "PC 3" no significa
	// nada para un cargador, y un cargador puede no traer serie.
	Identificador int
	NumeroSerie   string
	// Tipo es texto libre ("PC", "PROYECTOR", "CARGADOR"): la lista de cosas
	// que presta una escuela no es la misma que la de otra.
	Tipo string
	// Nombre es cómo se lo llama cuando no tiene número de carro.
	Nombre string
	// Reservable: si aparece en la lista de equipos libres al reservar. Un
	// proyector sí; un cargador se presta en el momento y nadie planifica
	// con él.
	Reservable        bool
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
		Tipo:          TipoPC,
		Reservable:    true,
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

// ── Equipos que no son PCs de un carro (RF-03.15) ───────────────────────
//
// La escuela también presta un proyector, cargadores y notebooks sueltas.
// Viven en esta misma entidad y no en una aparte porque "qué hay afuera del
// laboratorio" tiene que ser UNA sola lista: con dos tipos de cosa, el
// préstamo necesitaría dos referencias, el mostrador dos consultas y el
// barrido dos recorridos. Ver la migración 015.

const (
	// TipoPC es el tipo por defecto: una computadora de un carro.
	TipoPC = "PC"
	// MaxLargoTipoEquipo y MaxLargoNombreEquipo coinciden con la 015.
	MaxLargoTipoEquipo   = 50
	MaxLargoNombreEquipo = 100
)

var (
	ErrTipoEquipoVacio   = errors.New("hay que indicar de qué tipo es el equipo")
	ErrTipoEquipoLargo   = fmt.Errorf("el tipo no puede tener más de %d caracteres", MaxLargoTipoEquipo)
	ErrNombreEquipoVacio = errors.New("un equipo que no está en un carro necesita un nombre")
	ErrNombreEquipoLargo = fmt.Errorf("el nombre no puede tener más de %d caracteres", MaxLargoNombreEquipo)
)

// NormalizarTextoDeEquipo recorta los bordes y colapsa los espacios
// internos, sin tocar la caja: "Proyector Epson" se muestra tal cual se
// escribió. La unicidad sin distinguir mayúsculas la da el índice funcional
// de la 015, igual que con el nombre de una licencia.
func NormalizarTextoDeEquipo(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// TipoDeEquipoValido y NombreDeEquipoValido normalizan y validan, y devuelven
// el texto ya listo para guardar.
//
// Están separadas de NuevoEquipo porque el alta no es el único lugar que los
// escribe: el PATCH de una PC también puede cambiarlos, y sin pasar por acá
// un `"nombre": ""` llegaría hasta el CHECK de la 015 y volvería como un 500
// en vez de como el 400 que es.
func TipoDeEquipoValido(tipo string) (string, error) {
	tipo = NormalizarTextoDeEquipo(tipo)
	if tipo == "" {
		return "", ErrTipoEquipoVacio
	}
	if len([]rune(tipo)) > MaxLargoTipoEquipo {
		return "", ErrTipoEquipoLargo
	}
	return tipo, nil
}

func NombreDeEquipoValido(nombre string) (string, error) {
	nombre = NormalizarTextoDeEquipo(nombre)
	if nombre == "" {
		return "", ErrNombreEquipoVacio
	}
	if len([]rune(nombre)) > MaxLargoNombreEquipo {
		return "", ErrNombreEquipoLargo
	}
	return nombre, nil
}

// NuevoEquipo crea algo prestable que NO está en un carro: un proyector, un
// cargador, una notebook suelta.
//
// No tiene identificador ni número de serie —"PC 3" no significa nada para
// un cargador, y un cargador puede no traer serie de fábrica— así que lo que
// lo identifica es el nombre, y por eso es obligatorio.
//
// `reservable` separa el proyector de los cargadores: solo lo reservable
// aparece en la lista de equipos libres cuando un docente va a reservar.
func NuevoEquipo(id, tipo, nombre string, reservable bool, fechaAlta time.Time) (*PC, error) {
	tipo, err := TipoDeEquipoValido(tipo)
	if err != nil {
		return nil, err
	}

	nombre, err = NombreDeEquipoValido(nombre)
	if err != nil {
		return nil, err
	}

	return &PC{
		ID:         id,
		Tipo:       tipo,
		Nombre:     nombre,
		Reservable: reservable,
		Estado:     EstadoDisponible,
		FechaAlta:  fechaAlta,
	}, nil
}

// EstaEnUnCarro distingue una computadora de laboratorio de un equipo
// suelto. Es la condición que decide cómo se lo nombra y dónde se lo lista.
func (p *PC) EstaEnUnCarro() bool {
	return p.CarroID != ""
}

// Etiqueta es cómo se llama a este equipo en cualquier pantalla o correo:
// "PC 3" si está en un carro, su nombre si no.
//
// Vive en el dominio y no en cada pantalla porque si no, la misma máquina se
// vería distinta según por dónde se la mire — y porque un proyector rotulado
// "PC 0" es lo que sale de formatear un identificador que no existe.
func (p *PC) Etiqueta() string {
	if p.Nombre != "" {
		return p.Nombre
	}
	if p.Identificador > 0 {
		return fmt.Sprintf("PC %d", p.Identificador)
	}
	return "Equipo sin nombre"
}
