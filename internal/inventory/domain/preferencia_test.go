package domain

import (
	"errors"
	"testing"
)

func ptrInt(n int) *int       { return &n }
func ptrStr(s string) *string { return &s }

func TestNuevaPreferencia_SinCurso_ValeParaTodaLaMateria(t *testing.T) {
	p, err := NuevaPreferencia("p1", "e1", "  Dibujo Técnico  ", nil, nil, 1)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if p.MateriaNombre != "Dibujo Técnico" {
		t.Errorf("el nombre tiene que llegar recortado, quedó %q", p.MateriaNombre)
	}
	if p.Anio != nil || p.Division != nil {
		t.Error("sin curso, el alcance es toda materia con ese nombre")
	}
	if p.Alcance() != "Dibujo Técnico" {
		t.Errorf("alcance = %q", p.Alcance())
	}
}

func TestNuevaPreferencia_AlcancePorAnioYDivision(t *testing.T) {
	p, err := NuevaPreferencia("p1", "e1", "Matemática", ptrInt(3), ptrStr("b"), 2)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	// La división se guarda en mayúscula: el CHECK de la base sólo acepta
	// A-Z, y un formulario que manda "b" no puede terminar en un 500.
	if *p.Division != "B" {
		t.Errorf("división = %q, esperaba B", *p.Division)
	}
	if p.Alcance() != "Matemática de 3°B" {
		t.Errorf("alcance = %q", p.Alcance())
	}
}

func TestNuevaPreferencia_SoloAnio(t *testing.T) {
	p, err := NuevaPreferencia("p1", "e1", "Matemática", ptrInt(3), nil, 1)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if p.Alcance() != "Matemática de 3°" {
		t.Errorf("alcance = %q, esperaba 'Matemática de 3°'", p.Alcance())
	}
}

// No existen "todas las divisiones B": el alcance se abre de a un nivel.
func TestNuevaPreferencia_DivisionSinAnio_Error(t *testing.T) {
	_, err := NuevaPreferencia("p1", "e1", "Matemática", nil, ptrStr("B"), 1)

	if !errors.Is(err, ErrDivisionSinAnio) {
		t.Fatalf("esperaba ErrDivisionSinAnio, obtuve %v", err)
	}
}

func TestNuevaPreferencia_Invalidos(t *testing.T) {
	casos := []struct {
		nombre    string
		materia   string
		anio      *int
		division  *string
		prioridad int
		esperado  error
	}{
		{"materia vacía", "   ", nil, nil, 1, ErrMateriaPreferidaVacia},
		{"año fuera de rango", "Matemática", ptrInt(9), nil, 1, ErrAnioPreferenciaInvalido},
		{"año cero", "Matemática", ptrInt(0), nil, 1, ErrAnioPreferenciaInvalido},
		{"división de dos letras", "Matemática", ptrInt(3), ptrStr("AB"), 1, ErrDivisionPreferenciaInvalida},
		{"prioridad cero", "Matemática", nil, nil, 0, ErrPrioridadInvalida},
		{"prioridad fuera de rango", "Matemática", nil, nil, 99, ErrPrioridadInvalida},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := NuevaPreferencia("p1", "e1", c.materia, c.anio, c.division, c.prioridad)
			if !errors.Is(err, c.esperado) {
				t.Errorf("esperaba %v, obtuve %v", c.esperado, err)
			}
		})
	}
}
