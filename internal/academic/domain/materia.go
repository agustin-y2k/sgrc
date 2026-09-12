package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/ramiro/sgrc/internal/shared/texto"
)

// MaxLargoMateria es el ancho de `materia.nombre` en la base. El tope es de
// sanidad, no una regla de dominio: existe para que un nombre desbordado
// vuelva como un 400 que se puede leer y no como el 500 que devuelve Postgres
// al truncar.
const MaxLargoMateria = 100

var (
	// ErrNombreMateriaVacio: el nombre de una materia es libre (RF-02.3), pero
	// no puede quedar vacío ni ser solo espacios en blanco.
	ErrNombreMateriaVacio = errors.New("el nombre de la materia no puede estar vacío")
	ErrNombreMateriaLargo = fmt.Errorf("el nombre de la materia no puede tener más de %d caracteres", MaxLargoMateria)
)

// Materia es propia de un Curso específico — no es un catálogo compartido
// (1°A tiene SU Matemáticas, distinta de la de 1°B).
type Materia struct {
	ID      string
	CursoID string
	Nombre  string
	// Archivado: ver domain.Curso — se enciende al archivar el ciclo, y es el
	// único estado de una materia.
	Archivado bool
}

// ValidarNombreMateria es la única fuente de verdad de lo que puede entrar como
// nombre de materia — mismo criterio que ValidarCurso: valida y limpia en un
// solo paso, y devuelve el texto ya canonizado.
//
// Que limpie importa desde que el nombre puede venir de una celda de una
// planilla: «Lengua, Matemática» partido por la coma deja un espacio adelante
// del segundo, y sin esto «Matemática» y « Matemática» son dos materias
// distintas para el UNIQUE de la base.
func ValidarNombreMateria(nombre string) (string, error) {
	// Los tres pasos van en este orden y no es intercambiable.
	//
	// 1. Recortar. Un salto de línea PEGADO AL BORDE es basura de un copiado y
	//    se limpia sin más; sólo molesta uno en el medio.
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return "", ErrNombreMateriaVacio
	}
	// 2. Mirar los caracteres de control, antes de colapsar: después, un salto
	//    de línea interno ya se habría convertido en el espacio que separa dos
	//    palabras y el texto ilegible entraría disfrazado de nombre correcto.
	if strings.ContainsFunc(nombre, unicode.IsControl) {
		return "", ErrTextoIlegible
	}
	// 3. Y recién ahí, colapsar lo que queda.
	nombre = CanonizarEspacios(nombre)
	// El largo se mide DESPUÉS de colapsar: es el texto que se va a guardar.
	if len([]rune(nombre)) > MaxLargoMateria {
		return "", ErrNombreMateriaLargo
	}
	return nombre, nil
}

// CanonizarEspacios recorta las puntas y reduce toda corrida de espacios
// interna a uno solo.
//
// Delega en shared/texto, que es donde vive la regla: la misma función estaba
// escrita acá y en inventory/domain, cada una por su lado. Se conserva el
// nombre local porque es el que usa el resto del paquete.
func CanonizarEspacios(s string) string {
	return texto.Canonizar(s)
}

func NuevaMateria(id, cursoID, nombre string) (*Materia, error) {
	nombre, err := ValidarNombreMateria(nombre)
	if err != nil {
		return nil, err
	}
	return &Materia{ID: id, CursoID: cursoID, Nombre: nombre}, nil
}

// RenombrarA valida que el nuevo nombre no quede vacío antes de aplicarlo.
func (m *Materia) RenombrarA(nuevoNombre string) error {
	nuevoNombre, err := ValidarNombreMateria(nuevoNombre)
	if err != nil {
		return err
	}
	m.Nombre = nuevoNombre
	return nil
}

// NormalizarNombre es la forma en que dos nombres se comparan cuando la
// pregunta es «¿esta materia ya está?» y no «¿se escriben igual?»: sin tildes,
// sin mayúsculas y con los espacios colapsados.
//
// Delega en shared/texto.Clave, que es el espejo exacto de `clave_texto()` —
// la función de la base que sostiene el índice único. Las dos tienen que dar lo
// mismo o el chequeo previo y el índice dejan de coincidir.
func NormalizarNombre(nombre string) string {
	return texto.Clave(nombre)
}
