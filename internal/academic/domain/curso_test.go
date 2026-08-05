package domain

import (
	"errors"
	"testing"
)

func TestValidarNombreCurso_Validos(t *testing.T) {
	casos := []string{"1°A", "6°Z", "3°M", "1°Z", "6°A"}
	for _, c := range casos {
		if err := ValidarNombreCurso(c); err != nil {
			t.Errorf("%q debería ser válido, obtuve error: %v", c, err)
		}
	}
}

func TestValidarNombreCurso_Invalidos(t *testing.T) {
	casos := []string{
		"",
		"1A",   // falta el °
		"0°A",  // año fuera de rango (0)
		"7°A",  // año fuera de rango (7)
		"1°a",  // división en minúscula
		"1° A", // espacio de más
		"1°AB", // dos letras
		"1°Ñ",  // fuera del rango A-Z
		"11°A", // dos dígitos de año
		"1°A ", // espacio al final
		" 1°A", // espacio al principio
		"1-A",  // separador incorrecto
	}
	for _, c := range casos {
		if err := ValidarNombreCurso(c); !errors.Is(err, ErrNombreCursoInvalido) {
			t.Errorf("%q debería ser inválido, obtuve: %v", c, err)
		}
	}
}

func TestNuevoCurso_OK(t *testing.T) {
	c, err := NuevoCurso("id1", "ciclo1", "1°A")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !c.Activo || c.Archivado {
		t.Errorf("un curso nuevo debería ser Activo=true, Archivado=false: %+v", c)
	}
}

func TestNuevoCurso_NombreInvalido_Error(t *testing.T) {
	_, err := NuevoCurso("id1", "ciclo1", "primero A")

	if !errors.Is(err, ErrNombreCursoInvalido) {
		t.Fatalf("esperaba ErrNombreCursoInvalido, obtuve %v", err)
	}
}

func TestRenombrarA_OK(t *testing.T) {
	c, _ := NuevoCurso("id1", "ciclo1", "1°A")

	err := c.RenombrarA("2°B")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if c.Nombre != "2°B" {
		t.Errorf("el nombre no se actualizó: %s", c.Nombre)
	}
}

func TestRenombrarA_NombreInvalido_NoModificaElNombre(t *testing.T) {
	c, _ := NuevoCurso("id1", "ciclo1", "1°A")

	err := c.RenombrarA("nombre invalido")

	if !errors.Is(err, ErrNombreCursoInvalido) {
		t.Fatalf("esperaba ErrNombreCursoInvalido, obtuve %v", err)
	}
	if c.Nombre != "1°A" {
		t.Errorf("un renombre fallido no debería modificar el nombre original, quedó: %s", c.Nombre)
	}
}
