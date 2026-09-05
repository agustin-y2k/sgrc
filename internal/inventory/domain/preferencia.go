package domain

import (
	"errors"
	"fmt"
	"strings"
)

// Topes de PreferenciaDeEquipo. Acompañan a los de un curso, porque los tres
// ejes de una marca son los tres datos que un curso tiene (RF-02.2).
const (
	MaxLargoNombreMateriaPreferida = 100
	MaxLargoModalidadPreferencia   = 80
	MaxLargoDivisionPreferencia    = 12
	// MinAnioPreferencia y MaxAnioPreferencia son topes de sanidad, no reglas
	// de dominio: una primaria llega a 7° y una carrera de grado también.
	MinAnioPreferencia = 1
	MaxAnioPreferencia = 15
	// MaxPrioridadPreferencia deja nueve escalones.
	MaxPrioridadPreferencia = 9
)

var (
	ErrMateriaPreferidaVacia = errors.New("hay que decir para qué materia es preferente")
	ErrMateriaPreferidaLarga = fmt.Errorf("el nombre de la materia no puede tener más de %d caracteres",
		MaxLargoNombreMateriaPreferida)
	ErrAnioPreferenciaInvalido = fmt.Errorf("el año tiene que estar entre %d y %d",
		MinAnioPreferencia, MaxAnioPreferencia)
	ErrModalidadPreferenciaInvalida = fmt.Errorf("la modalidad no puede estar vacía ni tener más de %d caracteres",
		MaxLargoModalidadPreferencia)
	ErrDivisionPreferenciaInvalida = fmt.Errorf("la división no puede estar vacía ni tener más de %d caracteres",
		MaxLargoDivisionPreferencia)
	ErrPrioridadInvalida     = fmt.Errorf("la prioridad tiene que estar entre 1 y %d", MaxPrioridadPreferencia)
	ErrPreferenciaDuplicada  = errors.New("este equipo ya tiene una marca para esa materia y ese alcance")
	ErrPreferenciaNoEncontr  = errors.New("la marca de preferencia no existe")
	ErrSinEquiposParaPreferi = errors.New("hay que indicar al menos un equipo")
)

// PreferenciaDeEquipo es la marca que dice que una máquina es preferente para
// una materia (RF-03.21).
type PreferenciaDeEquipo struct {
	ID       string
	EquipoID string
	// MateriaNombre se guarda con la capitalización que eligió el Admin porque
	// es lo que se muestra ("Preferente para Dibujo Técnico").
	MateriaNombre string

	// Modalidad, Anio y Division acotan el alcance. Son los tres datos que
	// tiene un curso (RF-02.2) y son INDEPENDIENTES entre sí: cada uno se
	// sostiene solo y se pueden combinar. "Matemática de Electromecánica" es
	// un alcance válido sin decir de qué año, y para acotar a 4°2 se ponen el
	// año y la división.
	//
	// Los tres en nil = toda materia con ese nombre, en cualquier curso.
	Modalidad *string
	Anio      *int
	Division  *string

	// Prioridad ordena entre varias marcas del mismo equipo: 1 es la más fuerte.
	Prioridad int
}

// textoAcotado valida uno de los ejes de texto: se recorta pero NO se pasa a
// mayúsculas, porque es el nombre que escribió la institución y forzarlo sólo
// desfigura lo que tipearon. El cruce contra el curso ignora la
// capitalización por su cuenta.
func textoAcotado(valor *string, maxLargo int, invalido error) (*string, error) {
	if valor == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*valor)
	if v == "" || len([]rune(v)) > maxLargo {
		return nil, invalido
	}
	return &v, nil
}

// NuevaPreferencia valida y arma la marca.
func NuevaPreferencia(id, equipoID, materiaNombre string, modalidad *string, anio *int,
	division *string, prioridad int) (*PreferenciaDeEquipo, error) {
	// Normalizar antes de validar: un nombre de puros espacios pasaría el
	// "no vacío" y chocaría contra el CHECK de la base como un 500.
	materiaNombre = strings.TrimSpace(materiaNombre)
	if materiaNombre == "" {
		return nil, ErrMateriaPreferidaVacia
	}
	if len([]rune(materiaNombre)) > MaxLargoNombreMateriaPreferida {
		return nil, ErrMateriaPreferidaLarga
	}
	if anio != nil && (*anio < MinAnioPreferencia || *anio > MaxAnioPreferencia) {
		return nil, ErrAnioPreferenciaInvalido
	}

	modalidad, err := textoAcotado(modalidad, MaxLargoModalidadPreferencia, ErrModalidadPreferenciaInvalida)
	if err != nil {
		return nil, err
	}
	division, err = textoAcotado(division, MaxLargoDivisionPreferencia, ErrDivisionPreferenciaInvalida)
	if err != nil {
		return nil, err
	}

	if prioridad < 1 || prioridad > MaxPrioridadPreferencia {
		return nil, ErrPrioridadInvalida
	}

	return &PreferenciaDeEquipo{
		ID:            id,
		EquipoID:      equipoID,
		MateriaNombre: materiaNombre,
		Modalidad:     modalidad,
		Anio:          anio,
		Division:      division,
		Prioridad:     prioridad,
	}, nil
}

// Alcance describe en palabras a qué llega la marca, para mostrarla en el
// inventario: "Dibujo Técnico", "Dibujo Técnico de 3°", "Dibujo Técnico de
// Electromecánica" o "Dibujo Técnico de 4°2, Electromecánica".
//
// El año y la división se dicen juntos, como se lee un curso ("4°2"), y la
// modalidad va después. Una división sin año no se puede escribir así, y se
// enuncia aparte.
func (p *PreferenciaDeEquipo) Alcance() string {
	var partes []string
	switch {
	case p.Anio != nil:
		curso := fmt.Sprintf("%d°", *p.Anio)
		if p.Division != nil {
			curso += *p.Division
		}
		partes = append(partes, curso)
	case p.Division != nil:
		partes = append(partes, "división "+*p.Division)
	}
	if p.Modalidad != nil {
		partes = append(partes, *p.Modalidad)
	}
	if len(partes) == 0 {
		return p.MateriaNombre
	}
	return p.MateriaNombre + " de " + strings.Join(partes, ", ")
}
