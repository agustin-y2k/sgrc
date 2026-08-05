package domain

import (
	"errors"
	"testing"
)

func TestNuevoCicloLectivo_OK(t *testing.T) {
	c, err := NuevoCicloLectivo("id1", 2026)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !c.Activo || c.Archivado {
		t.Errorf("un ciclo nuevo debería ser Activo=true, Archivado=false: %+v", c)
	}
}

func TestNuevoCicloLectivo_AnioInvalido(t *testing.T) {
	casos := []int{0, -1, 1999, 2101, 99999}
	for _, anio := range casos {
		_, err := NuevoCicloLectivo("id1", anio)
		if !errors.Is(err, ErrAnioInvalido) {
			t.Errorf("año %d: esperaba ErrAnioInvalido, obtuve %v", anio, err)
		}
	}
}

func TestNuevoCicloLectivo_LimitesExactos_NoFallan(t *testing.T) {
	for _, anio := range []int{2000, 2100} {
		_, err := NuevoCicloLectivo("id1", anio)
		if err != nil {
			t.Errorf("año límite %d no debería fallar: %v", anio, err)
		}
	}
}

func TestArchivar_OK(t *testing.T) {
	c, _ := NuevoCicloLectivo("id1", 2026)

	err := c.Archivar()

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if c.Activo {
		t.Error("un ciclo archivado no debería seguir activo")
	}
	if !c.Archivado {
		t.Error("Archivado debería quedar true")
	}
}

func TestArchivar_DosVeces_Error(t *testing.T) {
	c, _ := NuevoCicloLectivo("id1", 2026)
	_ = c.Archivar()

	err := c.Archivar()

	if !errors.Is(err, ErrCicloYaArchivado) {
		t.Fatalf("esperaba ErrCicloYaArchivado, obtuve %v", err)
	}
}
