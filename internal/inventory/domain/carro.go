// Package domain contiene las entidades y reglas de negocio puras de
// inventory — carros, PCs e incidencias.
package domain

import (
	"errors"
	"strings"
)

// ErrNombreCarroVacio: el nombre de un carro es libre (RF-03.1), pero no
// puede quedar vacío ni ser solo espacios en blanco.
var ErrNombreCarroVacio = errors.New("el nombre del carro no puede estar vacío")

// Carro es el contenedor físico de PCs. No tiene freezado — ese atributo
// es de cada equipo individual (ver equipo.go), no del carro que lo contiene.
type Carro struct {
	ID          string
	Nombre      string
	Descripcion string
}

func NuevoCarro(id, nombre, descripcion string) (*Carro, error) {
	if strings.TrimSpace(nombre) == "" {
		return nil, ErrNombreCarroVacio
	}
	return &Carro{ID: id, Nombre: nombre, Descripcion: descripcion}, nil
}

// RenombrarA valida que el nuevo nombre no quede vacío antes de aplicarlo
// (RF-03.1: el Admin puede editar el carro en cualquier momento).
func (c *Carro) RenombrarA(nuevoNombre string) error {
	if strings.TrimSpace(nuevoNombre) == "" {
		return ErrNombreCarroVacio
	}
	c.Nombre = nuevoNombre
	return nil
}
