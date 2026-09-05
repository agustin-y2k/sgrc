package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Topes de Curso. Los de largo y de rango son de sanidad, no reglas de
// dominio: existen para que un dedazo no entre en la base, no para decidir
// cómo se organiza una institución.
const (
	MaxLargoDivision  = 12
	MaxLargoModalidad = 80
	// MinAnioCurso y MaxAnioCurso: una primaria llega a 7° y una carrera de
	// grado a 6° o 7°. El tope está lejos a propósito.
	MinAnioCurso = 1
	MaxAnioCurso = 15
)

var (
	// ErrAnioCursoInvalido: el año es lo único obligatorio de un curso.
	// Primaria, secundaria, terciario y universidad organizan sus cursos por
	// año; lo que cambia entre ámbitos es si además hay división y si además
	// hay modalidad (RF-02.2).
	ErrAnioCursoInvalido = fmt.Errorf("el año tiene que estar entre %d y %d", MinAnioCurso, MaxAnioCurso)
	ErrDivisionLarga     = fmt.Errorf("la división no puede tener más de %d caracteres", MaxLargoDivision)
	ErrModalidadLarga    = fmt.Errorf("la modalidad no puede tener más de %d caracteres", MaxLargoModalidad)
	// ErrTextoIlegible: caracteres de control. No es una regla de formato — es
	// que un texto con un salto de línea adentro no se puede mostrar en
	// ninguna de las pantallas que lo imprimen.
	ErrTextoIlegible = errors.New("la división o la modalidad tienen caracteres que no se pueden mostrar")
)

// Curso pertenece a un CicloLectivo.
//
// Es un año —obligatorio— más dos datos opcionales que no todas las
// instituciones tienen: la división y la modalidad. El NOMBRE no se guarda: lo
// calcula la base a partir del año y la división (migración 009), así que no
// puede desincronizarse de ellos.
type Curso struct {
	ID             string
	CicloLectivoID string
	// Anio es obligatorio. Todos los ámbitos organizan por año.
	Anio int
	// Division es "A", "2", "1ra"… Vacía cuando la institución no divide sus
	// cursos, que es lo habitual en una universidad.
	Division string
	// Modalidad es la agrupación de más arriba: modalidad, orientación o
	// carrera. Vacía cuando la institución no tiene ninguna, o cuando este
	// curso no pertenece a una —el ciclo básico de una técnica.
	Modalidad string
	// Nombre viene de la base y no se escribe: "4°2", "2°", "1°A".
	Nombre    string
	Activo    bool
	Archivado bool
}

// ValidarCurso es la única fuente de verdad de lo que puede entrar: la
// creación y la edición pasan las dos por acá, para no tener las reglas
// duplicadas en dos lugares que puedan divergir.
//
// Devuelve los textos ya recortados, porque validar y limpiar el mismo dato en
// dos pasos es como se cuela una división con un espacio al final que el CHECK
// de la base rechaza con un 500.
func ValidarCurso(anio int, division, modalidad string) (string, string, error) {
	if anio < MinAnioCurso || anio > MaxAnioCurso {
		return "", "", ErrAnioCursoInvalido
	}

	division = strings.TrimSpace(division)
	if len([]rune(division)) > MaxLargoDivision {
		return "", "", ErrDivisionLarga
	}

	modalidad = strings.TrimSpace(modalidad)
	if len([]rune(modalidad)) > MaxLargoModalidad {
		return "", "", ErrModalidadLarga
	}

	if strings.ContainsFunc(division, unicode.IsControl) ||
		strings.ContainsFunc(modalidad, unicode.IsControl) {
		return "", "", ErrTextoIlegible
	}

	return division, modalidad, nil
}

func NuevoCurso(id, cicloLectivoID string, anio int, division, modalidad string) (*Curso, error) {
	division, modalidad, err := ValidarCurso(anio, division, modalidad)
	if err != nil {
		return nil, err
	}
	c := &Curso{
		ID:             id,
		CicloLectivoID: cicloLectivoID,
		Anio:           anio,
		Division:       division,
		Modalidad:      modalidad,
		Activo:         true,
	}
	// Se arma acá también para que quien crea el curso pueda mostrarlo sin
	// releerlo de la base. La base lo recalcula igual: es la misma regla.
	c.Nombre = ComponerNombre(anio, division)
	return c, nil
}

// Editar cambia los tres datos de una vez — RF-02.11 permite corregirlos
// mientras el ciclo está activo. Van juntos porque son un solo curso descrito
// de tres formas: cambiar la división sin poder corregir el año que le
// corresponde dejaría al curso describiéndose mal.
func (c *Curso) Editar(anio int, division, modalidad string) error {
	division, modalidad, err := ValidarCurso(anio, division, modalidad)
	if err != nil {
		return err
	}
	c.Anio = anio
	c.Division = division
	c.Modalidad = modalidad
	c.Nombre = ComponerNombre(anio, division)
	return nil
}

// ComponerNombre arma "4°2", "2°" o "1°A". Es la misma expresión que calcula
// la columna generada `curso.nombre`, y existe de este lado para poder mostrar
// un curso recién creado sin volver a leerlo.
func ComponerNombre(anio int, division string) string {
	return strconv.Itoa(anio) + "°" + division
}

// Etiqueta es cómo se nombra el curso cuando hay que distinguirlo de otro:
// "1°A · Enfermería". El nombre solo puede repetirse entre dos carreras, así
// que donde se elige o se busca un curso hace falta la modalidad al lado.
//
// Mismo criterio que nombreDeEquipo() en el mostrador, donde "PC 1" hay una
// por carro.
func (c *Curso) Etiqueta() string {
	return EtiquetaDeCurso(c.Nombre, c.Modalidad)
}

// EtiquetaDeCurso arma la etiqueta desde las dos partes sueltas, para los
// listados que las traen de un JOIN y no tienen el Curso entero.
func EtiquetaDeCurso(nombre, modalidad string) string {
	if modalidad == "" {
		return nombre
	}
	return nombre + " · " + modalidad
}
