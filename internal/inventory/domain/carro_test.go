package domain

import (
	"errors"
	"strings"
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

// ── Canonización del nombre ─────────────────────────────────────────
// El bug que motivó todo esto: NuevoCarro validaba el nombre RECORTADO y
// después guardaba el crudo, así que «  Carro 1  » entraba con los espacios
// puestos. Con el único por texto exacto que tenía la tabla, eso hacía de
// «Carro 1» y «Carro  1» dos carros distintos, cada uno con sus equipos.
func TestNuevoCarro_GuardaElNombreCanonizado(t *testing.T) {
	casos := map[string]string{
		"Carro 1":              "Carro 1",
		"  Carro 1  ":          "Carro 1",
		"Carro  1":             "Carro 1",
		"Carro   de   Química": "Carro de Química",
		"Carro 1":              "Carro 1",
	}
	for entrada, esperado := range casos {
		c, err := NuevoCarro("id", entrada, "")
		if err != nil {
			t.Errorf("NuevoCarro(%q) devolvió error: %v", entrada, err)
			continue
		}
		if c.Nombre != esperado {
			t.Errorf("NuevoCarro(%q).Nombre = %q; esperaba %q", entrada, c.Nombre, esperado)
		}
	}
}

func TestRenombrarA_TambienCanoniza(t *testing.T) {
	c, err := NuevoCarro("id", "Carro 1", "")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if err := c.RenombrarA("  Carro   EDUTEC  "); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if c.Nombre != "Carro EDUTEC" {
		t.Errorf("quedó %q", c.Nombre)
	}
}

func TestNuevoCarro_NombreVacio(t *testing.T) {
	for _, entrada := range []string{"", "   ", "\t\n", " "} {
		if _, err := NuevoCarro("id", entrada, ""); !errors.Is(err, ErrNombreCarroVacio) {
			t.Errorf("NuevoCarro(%q) dio %v; esperaba ErrNombreCarroVacio", entrada, err)
		}
	}
}

// El tope existe para que un nombre desbordado vuelva como un 400 legible y no
// como el 500 que devuelve Postgres al pasarse de VARCHAR(100).
func TestNuevoCarro_NombreLargo(t *testing.T) {
	if _, err := NuevoCarro("id", strings.Repeat("a", MaxLargoNombreCarro+1), ""); !errors.Is(err, ErrNombreCarroLargo) {
		t.Errorf("esperaba ErrNombreCarroLargo, obtuve: %v", err)
	}
	// Y el largo se mide sobre lo que se GUARDA: un nombre que se pasa sólo por
	// espacios de más entra igual una vez colapsado.
	nombre := strings.Repeat("a", 60) + strings.Repeat(" ", 50) + strings.Repeat("b", 30)
	if _, err := NuevoCarro("id", nombre, ""); err != nil {
		t.Errorf("con los espacios colapsados entra en 91 caracteres: %v", err)
	}
}

// La descripción NO se canoniza: no es la identidad de nada y puede tener
// varios renglones a propósito. Sólo se le recortan los bordes.
func TestDescripcion_SeRecortaPeroConservaSusRenglones(t *testing.T) {
	c, err := NuevoCarro("id", "Carro 1", "  Primera línea\nSegunda  línea  ")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if c.Descripcion != "Primera línea\nSegunda  línea" {
		t.Errorf("descripción = %q; el salto de línea y el doble espacio interno tenían que quedar", c.Descripcion)
	}
}
