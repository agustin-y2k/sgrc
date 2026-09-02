package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Estado de una PC (RF-03.3).
type EstadoEquipo string

const (
	EstadoDisponible      EstadoEquipo = "DISPONIBLE"
	EstadoEnMantenimiento EstadoEquipo = "EN_MANTENIMIENTO"
	EstadoFueraDeServicio EstadoEquipo = "FUERA_DE_SERVICIO"
)

var ErrEstadoEquipoInvalido = errors.New("estado de equipo inválido")

// Legible es el estado escrito como se lo nombra en la escuela, para los
// textos que lee una persona: el aviso de que se le canceló una reserva sale
// al buzón y al correo de un docente, y "FUERA_DE_SERVICIO" ahí es el valor de
// un enum gritando en un mensaje que ya trae una mala noticia.
//
// En minúscula y sin sujeto porque se arma en frases más largas ("el equipo
// quedó fuera de servicio"). Es el equivalente de ETIQUETA_ESTADO_EQUIPO en el
// frontend, que resuelve lo mismo para las pantallas.
//
// El default devuelve el valor crudo: un estado que no esté en la lista no
// puede existir —ParseEstadoEquipo es la única puerta de entrada— y si algún
// día existiera, un texto feo es mejor que uno vacío.
func (e EstadoEquipo) Legible() string {
	switch e {
	case EstadoDisponible:
		return "disponible"
	case EstadoEnMantenimiento:
		return "en mantenimiento"
	case EstadoFueraDeServicio:
		return "fuera de servicio"
	default:
		return string(e)
	}
}

func ParseEstadoEquipo(s string) (EstadoEquipo, error) {
	switch EstadoEquipo(s) {
	case EstadoDisponible, EstadoEnMantenimiento, EstadoFueraDeServicio:
		return EstadoEquipo(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrEstadoEquipoInvalido, s)
	}
}

// PuedeTransicionarA implementa el diagrama de estados de equipo
// (docs/05-diagramas-estado.md): los tres estados se alternan libremente entre
// sí, y lo único de lo que no se vuelve es la baja del inventario, que es otra
// cosa (el flag dado_de_baja, no un valor de este enum).
//
// FUERA_DE_SERVICIO NO es terminal, aunque este código dijera que sí y citara
// al diagrama —que dice lo contrario— para justificarlo. La diferencia con
// EN_MANTENIMIENTO no es "roto para siempre" contra "roto por un rato", sino
// si la institución puede repararlo con lo que tiene; y eso cambia:
// una máquina sin batería vuelve a andar en cuanto aparece una batería. Lo
// irreversible es darla de baja, que la saca del inventario y libera su
// nombre.
func (e EstadoEquipo) PuedeTransicionarA(nuevo EstadoEquipo) bool {
	switch e {
	case EstadoDisponible:
		return nuevo == EstadoEnMantenimiento || nuevo == EstadoFueraDeServicio
	case EstadoEnMantenimiento:
		return nuevo == EstadoDisponible || nuevo == EstadoFueraDeServicio
	case EstadoFueraDeServicio:
		return nuevo == EstadoDisponible || nuevo == EstadoEnMantenimiento
	default:
		return false
	}
}

var ErrTransicionEstadoEquipoInvalida = errors.New("transición de estado de equipo inválida")

// MaxLargoNumeroSerie es el tope del VARCHAR(50) de la columna.
const MaxLargoNumeroSerie = 50

var (
	// El identificador sí es un entero positivo: es la etiqueta "PC 1", "PC 2"
	// que se le pone al equipo dentro de su carro, y la elige la escuela.
	ErrIdentificadorInvalido = errors.New("el identificador del equipo debe ser un entero positivo")
	// El número de serie NO es un número, aunque se llame así: es el código de
	// fábrica de la etiqueta y casi siempre trae letras ("5CD1234ABC").
	ErrNumeroSerieInvalido = errors.New("el número de serie no puede estar vacío")
	ErrNumeroSerieLargo    = fmt.Errorf("el número de serie no puede tener más de %d caracteres", MaxLargoNumeroSerie)
	ErrEquipoYaDadoDeBaja  = errors.New("el equipo ya está dado de baja")
)

// NormalizarNumeroSerie devuelve la forma canónica: sin espacios al borde y
// en mayúsculas.
func NormalizarNumeroSerie(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// Equipo es el equipo individual dentro de un Carro.
type Equipo struct {
	ID string
	// CarroID vacío = no está en ningún carro. Esto es
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
	// Reservable: si aparece en la lista de equipos libres al reservar.
	Reservable bool
	// EsComputadora decide si los cinco campos de abajo —y las cuentas de
	// acceso (RF-03.22)— tienen sentido para este equipo. Un cargador no
	// tiene CPU ni forma de entrar; una notebook suelta tiene las dos cosas.
	//
	// No reemplaza a Tipo ni lo duplica: Tipo dice QUÉ ES y sigue siendo
	// texto libre; esto dice QUÉ SE LE PREGUNTA. Las de un carro son
	// computadoras siempre.
	EsComputadora     bool
	Freezado          bool
	CPU               string
	RAM               string
	SistemaOperativo  string
	SoftwareInstalado string
	Estado            EstadoEquipo
	DadoDeBaja        bool
	FechaBaja         *time.Time
	FechaAlta         time.Time
	// TieneCuentas: si este equipo tiene anotada al menos una cuenta de
	// usuario (RF-03.22). Lo resuelven las consultas que listan equipos, y
	// solo sirve para decidir si vale la pena ofrecer "Cómo entrar": un
	// cargador no tiene con qué entrar y nadie debería ver ese botón.
	//
	// No es parte del estado del equipo —quien lo crea lo deja en false— y
	// por eso no se persiste: se calcula al leer.
	TieneCuentas bool
}

func NuevoEquipoDeCarro(id, carroID string, identificador int, numeroSerie string, freezado bool, fechaAlta time.Time) (*Equipo, error) {
	if identificador <= 0 {
		return nil, ErrIdentificadorInvalido
	}
	// Normalizar antes de validar: si no, un número de serie de puros espacios
	// pasaría el "no vacío" y llegaría a la base a chocar contra el CHECK
	// `chk_equipo_identificable`, que responde 500 en vez de explicar qué falta.
	serie := NormalizarNumeroSerie(numeroSerie)
	if serie == "" {
		return nil, ErrNumeroSerieInvalido
	}
	if len(serie) > MaxLargoNumeroSerie {
		return nil, ErrNumeroSerieLargo
	}
	return &Equipo{
		ID:            id,
		CarroID:       carroID,
		Identificador: identificador,
		NumeroSerie:   serie,
		Freezado:      freezado,
		EsComputadora: true,
		Tipo:          TipoPC,
		Reservable:    true,
		Estado:        EstadoDisponible,
		FechaAlta:     fechaAlta,
	}, nil
}

// CambiarEstado aplica una transición si es válida (ver
// EstadoEquipo.PuedeTransicionarA).
func (p *Equipo) CambiarEstado(nuevo EstadoEquipo) error {
	if !p.Estado.PuedeTransicionarA(nuevo) {
		return fmt.Errorf("%w: de %s a %s", ErrTransicionEstadoEquipoInvalida, p.Estado, nuevo)
	}
	p.Estado = nuevo
	return nil
}

// DarDeBaja marca la PC como dada de baja (soft delete, RF-03.4) — la fila
// se conserva para no perder el historial de incidencias y reservas.
func (p *Equipo) DarDeBaja(ahora time.Time) error {
	if p.DadoDeBaja {
		return ErrEquipoYaDadoDeBaja
	}
	p.DadoDeBaja = true
	p.FechaBaja = &ahora
	return nil
}

// MoverACarro cambia el carro al que pertenece la PC (RF-03.10).
func (p *Equipo) MoverACarro(nuevoCarroID string) {
	p.CarroID = nuevoCarroID
}

// ── Equipos que no son PCs de un carro (RF-03.15) ───────────────────────
// Una institución también presta proyectores, cargadores y notebooks sueltas.

const (
	// TipoPC es el tipo por defecto: una computadora de un carro.
	TipoPC = "PC"
	// MaxLargoTipoEquipo y MaxLargoNombreEquipo coinciden con los VARCHAR de la tabla.
	MaxLargoTipoEquipo   = 50
	MaxLargoNombreEquipo = 100
)

var (
	ErrTipoEquipoVacio   = errors.New("hay que indicar de qué tipo es el equipo")
	ErrTipoEquipoLargo   = fmt.Errorf("el tipo no puede tener más de %d caracteres", MaxLargoTipoEquipo)
	ErrNombreEquipoVacio = errors.New("un equipo que no está en un carro necesita un nombre")
	ErrNombreEquipoLargo = fmt.Errorf("el nombre no puede tener más de %d caracteres", MaxLargoNombreEquipo)
)

// NormalizarTextoDeEquipo recorta los bordes y colapsa los espacios internos,
// sin tocar la caja: "Proyector Epson" se muestra tal cual se escribió.
func NormalizarTextoDeEquipo(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// TipoDeEquipoValido y NombreDeEquipoValido normalizan y validan, y devuelven
// el texto ya listo para guardar.
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

// NumeroSerieOpcionalValido normaliza y valida un número de serie que PUEDE
// no venir. Es la diferencia con un equipo de carro, donde la serie es
// obligatoria: lo que presta una escuela fuera del laboratorio no siempre la
// tiene —un cargador no trae ninguna— y exigirla obligaría a inventar valores,
// que es peor que no tenerla. El vacío devuelve vacío, y el repositorio lo
// guarda como NULL: la columna es UNIQUE, y en Postgres eso permite tantos
// NULL como haga falta pero un solo número repetido.
func NumeroSerieOpcionalValido(numeroSerie string) (string, error) {
	serie := NormalizarNumeroSerie(numeroSerie)
	if serie == "" {
		return "", nil
	}
	if len(serie) > MaxLargoNumeroSerie {
		return "", ErrNumeroSerieLargo
	}
	return serie, nil
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

// NuevoEquipoSuelto crea algo prestable que NO está en un carro: un
// proyector, un cargador, una notebook suelta.
//
// numeroSerie es opcional y vale para cualquier tipo, no solo para las
// notebooks: un proyector tiene serie y es de lo que más se extravía, y un
// cargador no tiene ninguna. Por eso es un campo que se llena o no, y no dos
// categorías de equipo — la lista de lo que presta una escuela es texto libre
// justamente para no tener que decidir de antemano qué entra en cada una.
//
// Nace sin ser computadora (EsComputadora en false), que es lo que corresponde
// a la mayoría de lo que se presta fuera del laboratorio. Quien lo crea lo
// marca cuando corresponde, y ahí recién tienen sentido la ficha técnica y las
// cuentas de acceso.
func NuevoEquipoSuelto(id, tipo, nombre, numeroSerie string, reservable bool, fechaAlta time.Time) (*Equipo, error) {
	tipo, err := TipoDeEquipoValido(tipo)
	if err != nil {
		return nil, err
	}

	nombre, err = NombreDeEquipoValido(nombre)
	if err != nil {
		return nil, err
	}

	serie, err := NumeroSerieOpcionalValido(numeroSerie)
	if err != nil {
		return nil, err
	}

	return &Equipo{
		ID:          id,
		Tipo:        tipo,
		Nombre:      nombre,
		NumeroSerie: serie,
		Reservable:  reservable,
		Estado:      EstadoDisponible,
		FechaAlta:   fechaAlta,
	}, nil
}

// EstaEnUnCarro distingue una computadora de laboratorio de un equipo
// suelto. Es la condición que decide cómo se lo nombra y dónde se lo lista.
func (p *Equipo) EstaEnUnCarro() bool {
	return p.CarroID != ""
}

// Etiqueta es cómo se llama a este equipo en cualquier pantalla o correo: "PC
// 3" si está en un carro, su nombre si no.
func (p *Equipo) Etiqueta() string {
	if p.Nombre != "" {
		return p.Nombre
	}
	if p.Identificador > 0 {
		return fmt.Sprintf("PC %d", p.Identificador)
	}
	return "Equipo sin nombre"
}
