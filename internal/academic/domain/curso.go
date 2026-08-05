package domain

import (
	"errors"
	"fmt"
	"regexp"
)

// ErrNombreCursoInvalido: el nombre de un curso no es libre — formato
// estricto {año}°{división}, ej. "1°A", "6°Z" (RF-02.2).
var ErrNombreCursoInvalido = errors.New("el nombre del curso debe tener el formato {año}°{división}, ej. 1°A")

var patronNombreCurso = regexp.MustCompile(`^[1-6]°[A-Z]$`)

// ValidarNombreCurso es la única fuente de verdad de este formato — tanto
// la creación (NuevoCurso) como la edición de un curso existente deben
// pasar por acá, para no tener el regex duplicado en dos lugares que
// puedan divergir con el tiempo.
func ValidarNombreCurso(nombre string) error {
	if !patronNombreCurso.MatchString(nombre) {
		return fmt.Errorf("%w: %q", ErrNombreCursoInvalido, nombre)
	}
	return nil
}

// Curso pertenece a un CicloLectivo. Activo/Archivado siguen al ciclo que
// lo contiene — un curso no se archiva individualmente, se archiva todo
// el ciclo de una vez (RF-02.4).
type Curso struct {
	ID             string
	CicloLectivoID string
	Nombre         string
	Activo         bool
	Archivado      bool
}

func NuevoCurso(id, cicloLectivoID, nombre string) (*Curso, error) {
	if err := ValidarNombreCurso(nombre); err != nil {
		return nil, err
	}
	return &Curso{ID: id, CicloLectivoID: cicloLectivoID, Nombre: nombre, Activo: true}, nil
}

// RenombrarA valida el nuevo nombre antes de aplicarlo — RF-02.11 permite
// editar el nombre mientras el ciclo está activo, pero siempre respetando
// el mismo formato que en la creación.
func (c *Curso) RenombrarA(nuevoNombre string) error {
	if err := ValidarNombreCurso(nuevoNombre); err != nil {
		return err
	}
	c.Nombre = nuevoNombre
	return nil
}
