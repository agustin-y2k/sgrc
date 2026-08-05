package domain

import (
	"errors"
	"testing"
)

func TestNuevoCarro_OK(t *testing.T) {
	c, err := NuevoCarro("id1", "Carro 1", "Notebooks del laboratorio")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if c.Nombre != "Carro 1" {
		t.Errorf("nombre incorrecto: %s", c.Nombre)
	}
}

func TestNuevoCarro_NombreVacio_Error(t *testing.T) {
	casos := []string{"", "   ", "\t"}
	for _, c := range casos {
		_, err := NuevoCarro("id1", c, "")
		if !errors.Is(err, ErrNombreCarroVacio) {
			t.Errorf("nombre %q: esperaba ErrNombreCarroVacio, obtuve %v", c, err)
		}
	}
}

func TestRenombrarCarro_OK(t *testing.T) {
	c, _ := NuevoCarro("id1", "Carro 1", "")

	err := c.RenombrarA("Carro Norte")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if c.Nombre != "Carro Norte" {
		t.Errorf("el nombre no se actualizó: %s", c.Nombre)
	}
}

func TestRenombrarCarro_Vacio_NoModifica(t *testing.T) {
	c, _ := NuevoCarro("id1", "Carro 1", "")

	err := c.RenombrarA("   ")

	if !errors.Is(err, ErrNombreCarroVacio) {
		t.Fatalf("esperaba ErrNombreCarroVacio, obtuve %v", err)
	}
	if c.Nombre != "Carro 1" {
		t.Errorf("un renombre fallido no debería modificar el nombre original, quedó: %s", c.Nombre)
	}
}
