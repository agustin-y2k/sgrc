package domain

import (
	"errors"
	"fmt"
	"strings"
)

// Topes de PreferenciaDeEquipo.
const (
	MaxLargoNombreMateriaPreferida = 100
	// MinAnioPreferencia y MaxAnioPreferencia son los mismos años que admite el
	// nombre de un curso ('^[1-6]°[A-Z]$').
	MinAnioPreferencia = 1
	MaxAnioPreferencia = 6
	// MaxPrioridadPreferencia deja nueve escalones.
	MaxPrioridadPreferencia = 9
)

var (
	ErrMateriaPreferidaVacia = errors.New("hay que decir para qué materia es preferente")
	ErrMateriaPreferidaLarga = fmt.Errorf("el nombre de la materia no puede tener más de %d caracteres",
		MaxLargoNombreMateriaPreferida)
	ErrAnioPreferenciaInvalido = fmt.Errorf("el año tiene que estar entre %d y %d",
		MinAnioPreferencia, MaxAnioPreferencia)
	ErrDivisionPreferenciaInvalida = errors.New("la división tiene que ser una sola letra de la A a la Z")
	// ErrDivisionSinAnio: no existen "todas las divisiones B". El alcance se
	// abre de a un nivel: toda la materia, un año entero, o un año y división.
	ErrDivisionSinAnio       = errors.New("para acotar por división hay que indicar también el año")
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

	// Anio y Division acotan el alcance, de menos a más específico: nil, nil →
	// toda materia con ese nombre, en cualquier curso 3, nil → sólo las de
	// tercer año 3, "B" → sólo 3°B
	Anio     *int
	Division *string

	// Prioridad ordena entre varias marcas del mismo equipo: 1 es la más fuerte.
	Prioridad int
}

// NuevaPreferencia valida y arma la marca.
func NuevaPreferencia(id, equipoID, materiaNombre string, anio *int, division *string, prioridad int) (*PreferenciaDeEquipo, error) {
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
	if division != nil {
		d := strings.ToUpper(strings.TrimSpace(*division))
		if len([]rune(d)) != 1 || d < "A" || d > "Z" {
			return nil, ErrDivisionPreferenciaInvalida
		}
		if anio == nil {
			return nil, ErrDivisionSinAnio
		}
		division = &d
	}
	if prioridad < 1 || prioridad > MaxPrioridadPreferencia {
		return nil, ErrPrioridadInvalida
	}

	return &PreferenciaDeEquipo{
		ID:            id,
		EquipoID:      equipoID,
		MateriaNombre: materiaNombre,
		Anio:          anio,
		Division:      division,
		Prioridad:     prioridad,
	}, nil
}

// Alcance describe en palabras a qué llega la marca, para mostrarla en el
// inventario: "Dibujo Técnico", "Dibujo Técnico de 3°" o "Dibujo Técnico de
// 3°B".
func (p *PreferenciaDeEquipo) Alcance() string {
	if p.Anio == nil {
		return p.MateriaNombre
	}
	alcance := fmt.Sprintf("%s de %d°", p.MateriaNombre, *p.Anio)
	if p.Division != nil {
		alcance += *p.Division
	}
	return alcance
}
