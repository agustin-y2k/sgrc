// Package domain contiene las entidades y reglas de negocio puras de
// inventory — carros, PCs e incidencias.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ramiro/sgrc/internal/shared/texto"
)

// MaxLargoNombreCarro es el ancho de `carro.nombre` en la base. El tope es de
// sanidad, no una regla de dominio: existe para que un nombre desbordado vuelva
// como un 400 que se puede leer y no como el 500 que devuelve Postgres al
// truncar. `descripcion` no lleva tope porque su columna es TEXT y no lo tiene.
const MaxLargoNombreCarro = 100

var (
	// ErrNombreCarroVacio: el nombre de un carro es libre (RF-03.1), pero no
	// puede quedar vacío ni ser solo espacios en blanco.
	ErrNombreCarroVacio  = errors.New("el nombre del carro no puede estar vacío")
	ErrCarroYaDadoDeBaja = errors.New("el carro ya está dado de baja")
	// ErrCarroNoEstaDadoDeBaja: mismo criterio que en un equipo — reactivar algo
	// que está en circulación no hace nada, y decirlo es más útil que un 200.
	ErrCarroNoEstaDadoDeBaja = errors.New("el carro no está dado de baja")
	ErrNombreCarroLargo      = fmt.Errorf("el nombre del carro no puede tener más de %d caracteres", MaxLargoNombreCarro)
)

// Carro es el contenedor físico de PCs. No tiene freezado — ese atributo
// es de cada equipo individual (ver equipo.go), no del carro que lo contiene.
type Carro struct {
	ID          string
	Nombre      string
	Descripcion string
	// DadoDeBaja: el carro se retiró. Es una baja LÓGICA y no un DELETE porque
	// su nombre vive congelado en el histórico de uso de cada equipo que tuvo
	// adentro — borrarlo dejaría ese histórico hablando de algo que el sistema
	// ya no puede explicar.
	DadoDeBaja bool
	FechaBaja  *time.Time
}

// DarDeBaja retira el carro de circulación. Idempotente no: volver a darlo de
// baja es un error, igual que en un equipo, porque quien lo pide cree que está
// haciendo algo y no está haciendo nada.
func (c *Carro) DarDeBaja(ahora time.Time) error {
	if c.DadoDeBaja {
		return ErrCarroYaDadoDeBaja
	}
	c.DadoDeBaja = true
	c.FechaBaja = &ahora
	return nil
}

// Reactivar devuelve el carro a circulación — ver Equipo.Reactivar, que
// explica por qué deshacer una baja tiene que existir.
//
// Un carro se retira vacío (ver DarDeBajaCarro), así que reactivarlo no
// devuelve ninguna máquina con él: las que tenía adentro se movieron o se
// dieron de baja por su cuenta, y cada una se recupera por separado.
func (c *Carro) Reactivar() error {
	if !c.DadoDeBaja {
		return ErrCarroNoEstaDadoDeBaja
	}
	c.DadoDeBaja = false
	c.FechaBaja = nil
	return nil
}

// NombreDeCarroValido normaliza y valida, y devuelve el texto ya listo para
// guardar — mismo criterio que NombreDeEquipoValido en equipo.go.
//
// Que DEVUELVA el texto canonizado es el punto, y es lo que faltaba: antes esto
// validaba `TrimSpace(nombre)` y después guardaba `nombre` crudo, así que
// «  Carro 1  » entraba con los espacios puestos. Con el único sobre el texto
// exacto que tenía la tabla, eso convertía a «Carro 1» y a «Carro  1» en dos
// carros distintos, cada uno con sus equipos, indistinguibles en pantalla.
func NombreDeCarroValido(nombre string) (string, error) {
	nombre = texto.Canonizar(nombre)
	if nombre == "" {
		return "", ErrNombreCarroVacio
	}
	if len([]rune(nombre)) > MaxLargoNombreCarro {
		return "", ErrNombreCarroLargo
	}
	return nombre, nil
}

// LimpiarDescripcion recorta los bordes y NADA MÁS.
//
// A diferencia del nombre, la descripción no se canoniza: no es la identidad
// de nada —nadie compara dos carros por su descripción— y colapsar sus espacios
// le aplastaría los saltos de línea a un texto que puede tener varios renglones
// a propósito. La canonización es para lo que nombra; esto es prosa.
func LimpiarDescripcion(descripcion string) string {
	return strings.TrimSpace(descripcion)
}

func NuevoCarro(id, nombre, descripcion string) (*Carro, error) {
	nombre, err := NombreDeCarroValido(nombre)
	if err != nil {
		return nil, err
	}
	return &Carro{ID: id, Nombre: nombre, Descripcion: LimpiarDescripcion(descripcion)}, nil
}

// RenombrarA valida que el nuevo nombre no quede vacío antes de aplicarlo
// (RF-03.1: el Admin puede editar el carro en cualquier momento).
func (c *Carro) RenombrarA(nuevoNombre string) error {
	nuevoNombre, err := NombreDeCarroValido(nuevoNombre)
	if err != nil {
		return err
	}
	c.Nombre = nuevoNombre
	return nil
}

// CambiarDescripcion existe para que la descripción entre por el mismo lugar
// que el nombre y no se asigne directo desde el servicio: así el recorte pasa
// siempre, venga de donde venga.
func (c *Carro) CambiarDescripcion(nueva string) {
	c.Descripcion = LimpiarDescripcion(nueva)
}
