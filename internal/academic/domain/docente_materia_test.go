package domain

import (
	"errors"
	"testing"
)

func TestParseRolDocente_Validos(t *testing.T) {
	casos := map[string]RolDocente{"TITULAR": RolTitular, "SUPLENTE": RolSuplente}
	for entrada, esperado := range casos {
		got, err := ParseRolDocente(entrada)
		if err != nil {
			t.Errorf("ParseRolDocente(%q) no debería fallar: %v", entrada, err)
		}
		if got != esperado {
			t.Errorf("ParseRolDocente(%q) = %q, esperaba %q", entrada, got, esperado)
		}
	}
}

func TestParseRolDocente_Invalido(t *testing.T) {
	casos := []string{"", "titular", "SUPLENTE ", "PROFESOR"}
	for _, c := range casos {
		_, err := ParseRolDocente(c)
		if !errors.Is(err, ErrRolDocenteInvalido) {
			t.Errorf("ParseRolDocente(%q): esperaba ErrRolDocenteInvalido, obtuve %v", c, err)
		}
	}
}

func TestNuevoDocenteMateria(t *testing.T) {
	dm := NuevoDocenteMateria("id1", "usuario1", "materia1", RolTitular)

	if dm.UsuarioID != "usuario1" || dm.MateriaID != "materia1" || dm.Rol != RolTitular {
		t.Errorf("campos incorrectos: %+v", dm)
	}
}
