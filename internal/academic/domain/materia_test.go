package domain

import (
	"errors"
	"testing"
)

func TestNuevaMateria_OK(t *testing.T) {
	m, err := NuevaMateria("id1", "curso1", "Matemáticas")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !m.Activo {
		t.Error("una materia nueva debería ser Activo=true")
	}
}

func TestNuevaMateria_NombreVacio_Error(t *testing.T) {
	casos := []string{"", "   ", "\t", "\n"}
	for _, c := range casos {
		_, err := NuevaMateria("id1", "curso1", c)
		if !errors.Is(err, ErrNombreMateriaVacio) {
			t.Errorf("nombre %q: esperaba ErrNombreMateriaVacio, obtuve %v", c, err)
		}
	}
}

func TestRenombrarMateria_OK(t *testing.T) {
	m, _ := NuevaMateria("id1", "curso1", "Matemáticas")

	err := m.RenombrarA("Física")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if m.Nombre != "Física" {
		t.Errorf("el nombre no se actualizó: %s", m.Nombre)
	}
}

func TestRenombrarMateria_Vacio_NoModifica(t *testing.T) {
	m, _ := NuevaMateria("id1", "curso1", "Matemáticas")

	err := m.RenombrarA("   ")

	if !errors.Is(err, ErrNombreMateriaVacio) {
		t.Fatalf("esperaba ErrNombreMateriaVacio, obtuve %v", err)
	}
	if m.Nombre != "Matemáticas" {
		t.Errorf("un renombre fallido no debería modificar el nombre original, quedó: %s", m.Nombre)
	}
}
