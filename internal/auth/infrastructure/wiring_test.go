package infrastructure

import (
	"strings"
	"testing"
)

func TestGenerarPasswordTemporal_LongitudYAlfabeto(t *testing.T) {
	pass, err := GenerarPasswordTemporal()
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(pass) != 12 {
		t.Fatalf("esperaba 12 caracteres, obtuve %d: %q", len(pass), pass)
	}
	for _, c := range pass {
		if !strings.ContainsRune(alfabetoPasswordTemporal, c) {
			t.Errorf("carácter %q no pertenece al alfabeto esperado", c)
		}
	}
}

func TestGenerarPasswordTemporal_DosLlamadasDistintas(t *testing.T) {
	// Con 68 bits de entropía la probabilidad de colisión es astronómicamente
	// baja — si esto falla alguna vez, hay un bug real en la fuente de
	// aleatoriedad, no mala suerte.
	p1, _ := GenerarPasswordTemporal()
	p2, _ := GenerarPasswordTemporal()
	if p1 == p2 {
		t.Fatal("dos contraseñas temporales generadas por separado no deberían coincidir")
	}
}

func TestNuevoID_NoVacioYDistintoEntreLlamadas(t *testing.T) {
	id1 := NuevoID()
	id2 := NuevoID()

	if id1 == "" || id2 == "" {
		t.Fatal("NuevoID no debería devolver string vacío")
	}
	if id1 == id2 {
		t.Fatal("dos llamadas a NuevoID no deberían devolver el mismo id")
	}
	if len(id1) != 36 { // formato UUID estándar: 8-4-4-4-12
		t.Errorf("longitud de UUID inesperada: %d (%q)", len(id1), id1)
	}
}
