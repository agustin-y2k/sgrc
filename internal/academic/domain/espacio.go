package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// MaxLargoNombreEspacio es un tope de sanidad, no una regla: existe para que un
// dedazo no entre en la base.
const MaxLargoNombreEspacio = 80

var (
	// ErrNombreEspacioVacio: un espacio es SOLO su nombre. Sin él no hay nada
	// que guardar — al revés de un curso, que se llama por su año aunque no le
	// escribas nada.
	ErrNombreEspacioVacio = errors.New("el espacio necesita un nombre")
	ErrNombreEspacioLargo = fmt.Errorf(
		"el nombre del espacio no puede tener más de %d caracteres", MaxLargoNombreEspacio)
)

// Espacio es un lugar de la institución donde se trabaja y que NO es un curso:
// la Biblioteca, Dirección, Preceptoría, un laboratorio.
//
// Existe porque en una escuela trabaja gente que no da clase frente a un curso,
// y el sistema no tenía dónde ponerla: todo colgaba de `curso`, y un curso es
// un año obligatorio (RF-02.2).
//
// **No se modeló aflojando el curso.** La salida corta era hacer el año
// opcional y cargar "Biblioteca" como un curso sin año; se descartó porque esa
// regla es deliberada —todos los ámbitos tienen año— y relajarla para meter
// algo que no es un curso abre la puerta a crear cursos sin año por error. El
// concepto nuevo se nombra, no se disfraza.
//
// Lo que sí comparte con un curso: **tiene materias**. En la Biblioteca puede
// haber «Apoyo escolar», y se reserva para eso igual que para Matemática — una
// reserva sigue teniendo una materia detrás, y el módulo de reservas no cambió.
type Espacio struct {
	ID             string
	CicloLectivoID string
	// Nombre libre: es lo único que lo identifica. No hay año, división ni
	// modalidad que componer.
	Nombre    string
	Archivado bool
}

// NuevoEspacio valida y normaliza. El nombre se guarda sin espacios al borde,
// por el mismo motivo que el de una materia: dos nombres que sólo difieren en
// un espacio final son el mismo lugar, y el índice único de la base los trata
// como tal (migración 011).
func NuevoEspacio(id, cicloLectivoID, nombre string) (*Espacio, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return nil, ErrNombreEspacioVacio
	}
	if len([]rune(nombre)) > MaxLargoNombreEspacio {
		return nil, ErrNombreEspacioLargo
	}
	if strings.ContainsFunc(nombre, unicode.IsControl) {
		return nil, ErrTextoIlegible
	}
	return &Espacio{
		ID:             id,
		CicloLectivoID: cicloLectivoID,
		Nombre:         nombre,
	}, nil
}

// Renombrar aplica las mismas validaciones que el alta.
func (e *Espacio) Renombrar(nombre string) error {
	nuevo, err := NuevoEspacio(e.ID, e.CicloLectivoID, nombre)
	if err != nil {
		return err
	}
	e.Nombre = nuevo.Nombre
	return nil
}
