package domain

import (
	"errors"
	"strings"
)

// ErrNombreMateriaVacio: el nombre de una materia es libre (RF-02.3), pero
// no puede quedar vacío ni ser solo espacios en blanco.
var ErrNombreMateriaVacio = errors.New("el nombre de la materia no puede estar vacío")

// Materia es propia de un Curso específico — no es un catálogo compartido
// (1°A tiene SU Matemáticas, distinta de la de 1°B).
type Materia struct {
	ID        string
	CursoID   string
	Nombre    string
	Activo    bool
	Archivado bool
}

func NuevaMateria(id, cursoID, nombre string) (*Materia, error) {
	if strings.TrimSpace(nombre) == "" {
		return nil, ErrNombreMateriaVacio
	}
	return &Materia{ID: id, CursoID: cursoID, Nombre: nombre, Activo: true}, nil
}

// RenombrarA valida que el nuevo nombre no quede vacío antes de aplicarlo.
func (m *Materia) RenombrarA(nuevoNombre string) error {
	if strings.TrimSpace(nuevoNombre) == "" {
		return ErrNombreMateriaVacio
	}
	m.Nombre = nuevoNombre
	return nil
}
