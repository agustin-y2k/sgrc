package domain

import (
	"errors"
	"strings"
	"testing"
)

func ptrInt(n int) *int       { return &n }
func ptrStr(s string) *string { return &s }

func TestNuevaPreferencia_SinAcotar_ValeParaTodaLaMateria(t *testing.T) {
	p, err := NuevaPreferencia("p1", "e1", "  Dibujo Técnico  ", nil, nil, nil, 1)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if p.MateriaNombre != "Dibujo Técnico" {
		t.Errorf("el nombre tiene que llegar recortado, quedó %q", p.MateriaNombre)
	}
	if p.Modalidad != nil || p.Anio != nil || p.Division != nil {
		t.Error("sin acotar, el alcance es toda materia con ese nombre")
	}
	if p.Alcance() != "Dibujo Técnico" {
		t.Errorf("alcance = %q", p.Alcance())
	}
}

// Los tres ejes son INDEPENDIENTES: cada uno se sostiene solo. Es lo que
// permite que la marca hable el idioma de cada institución — una técnica acota
// por modalidad, una primaria por año, cualquiera por curso puntual.
func TestNuevaPreferencia_LosTresEjesPorSeparado(t *testing.T) {
	casos := []struct {
		nombre    string
		modalidad *string
		anio      *int
		division  *string
		alcance   string
	}{
		{"solo la modalidad", ptrStr("Electromecánica"), nil, nil, "Matemática de Electromecánica"},
		{"solo el año", nil, ptrInt(3), nil, "Matemática de 3°"},
		// El año y la división se dicen juntos, como se lee un curso.
		{"año y división", nil, ptrInt(4), ptrStr("2"), "Matemática de 4°2"},
		{"año, división y modalidad", ptrStr("Construcción"), ptrInt(4), ptrStr("2"), "Matemática de 4°2, Construcción"},
		// Rara pero no incoherente: el que acota elige qué acota.
		{"solo la división", nil, nil, ptrStr("2"), "Matemática de división 2"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			p, err := NuevaPreferencia("p1", "e1", "Matemática", c.modalidad, c.anio, c.division, 1)
			if err != nil {
				t.Fatalf("no debería fallar: %v", err)
			}
			if p.Alcance() != c.alcance {
				t.Errorf("alcance = %q, esperaba %q", p.Alcance(), c.alcance)
			}
		})
	}
}

// Se recorta pero se guarda como lo escribieron: son los nombres que puso la
// institución, y forzar mayúsculas sólo desfigura lo que tipearon. El cruce
// contra el curso ignora la capitalización por su cuenta.
func TestNuevaPreferencia_RecortaSinDesfigurar(t *testing.T) {
	p, err := NuevaPreferencia("p1", "e1", "Matemática", ptrStr("  Electromecánica  "), nil, ptrStr("  2  "), 1)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if *p.Modalidad != "Electromecánica" || *p.Division != "2" {
		t.Errorf("modalidad = %q, división = %q", *p.Modalidad, *p.Division)
	}
}

// La división de la marca tiene que poder decir lo mismo que la de un curso,
// que es texto libre: hay escuelas que las nombran por letra y otras por
// número.
func TestNuevaPreferencia_DivisionDeCualquierAmbito(t *testing.T) {
	for _, division := range []string{"A", "2", "1ra", "B bis"} {
		if _, err := NuevaPreferencia("p1", "e1", "Matemática", nil, nil, ptrStr(division), 1); err != nil {
			t.Errorf("%q debería servir como alcance: %v", division, err)
		}
	}
}

// Séptimo año existe en primaria y en varias carreras: el rango es un tope de
// sanidad, no una regla de dominio.
func TestNuevaPreferencia_AnioMasAlláDelSexto(t *testing.T) {
	if _, err := NuevaPreferencia("p1", "e1", "Matemática", nil, ptrInt(7), nil, 1); err != nil {
		t.Errorf("séptimo año tiene que entrar: %v", err)
	}
}

func TestNuevaPreferencia_Invalidos(t *testing.T) {
	casos := []struct {
		nombre    string
		materia   string
		modalidad *string
		anio      *int
		division  *string
		prioridad int
		esperado  error
	}{
		{"materia vacía", "   ", nil, nil, nil, 1, ErrMateriaPreferidaVacia},
		{"año fuera de rango", "Matemática", nil, ptrInt(99), nil, 1, ErrAnioPreferenciaInvalido},
		{"año cero", "Matemática", nil, ptrInt(0), nil, 1, ErrAnioPreferenciaInvalido},
		{"modalidad en blanco", "Matemática", ptrStr("   "), nil, nil, 1, ErrModalidadPreferenciaInvalida},
		{"modalidad larguísima", "Matemática", ptrStr(strings.Repeat("a", MaxLargoModalidadPreferencia+1)), nil, nil, 1, ErrModalidadPreferenciaInvalida},
		{"división en blanco", "Matemática", nil, nil, ptrStr("  "), 1, ErrDivisionPreferenciaInvalida},
		{"división larguísima", "Matemática", nil, nil, ptrStr(strings.Repeat("a", MaxLargoDivisionPreferencia+1)), 1, ErrDivisionPreferenciaInvalida},
		{"prioridad cero", "Matemática", nil, nil, nil, 0, ErrPrioridadInvalida},
		{"prioridad fuera de rango", "Matemática", nil, nil, nil, 99, ErrPrioridadInvalida},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := NuevaPreferencia("p1", "e1", c.materia, c.modalidad, c.anio, c.division, c.prioridad)
			if !errors.Is(err, c.esperado) {
				t.Errorf("esperaba %v, obtuve %v", c.esperado, err)
			}
		})
	}
}
