//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// Migración 011 — la misma regla de unicidad para todos los nombres del
// sistema, comparando sin tildes, sin mayúsculas y sin espacios de más.
//
// Se prueba contra Postgres real porque quien la sostiene es el índice único,
// no el código: la validación de Go canoniza lo que se guarda, pero el que
// impide la segunda fila es `clave_texto()`. Un test con un fake diría que
// funciona aunque el índice no existiera.

// variantesDe son las formas de escribir lo mismo que antes entraban como
// filas distintas. Se prueban todas contra cada tabla: la regla es una sola y
// tiene que valer igual en las cinco.
func variantesDe(base string) map[string]string {
	return map[string]string{
		base:               "idéntico",
		"  " + base + "  ": "espacios en los bordes",
		base + " ":         "un espacio al final",
		" " + base:         "espacio duro adelante",
	}
}

func TestUnicidad_CarroPorNombre(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	primero, err := domain.NuevoCarro(NuevoID(), "Carro EDUTEC", "")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCarro(ctx, primero); err != nil {
		t.Fatalf("el primero tenía que entrar: %v", err)
	}

	variantes := variantesDe("Carro EDUTEC")
	variantes["CARRO EDUTEC"] = "mayúsculas"
	variantes["carro edutec"] = "minúsculas"
	variantes["Carro  EDUTEC"] = "doble espacio adentro"
	variantes["Cárro EDUTEC"] = "una tilde de más"

	for nombre, que := range variantes {
		c, err := domain.NuevoCarro(NuevoID(), nombre, "")
		if err != nil {
			t.Errorf("el dominio rechazó %q (%s), que es un nombre válido: %v", nombre, que, err)
			continue
		}
		if err := repo.CrearCarro(ctx, c); !errors.Is(err, application.ErrNombreCarroDuplicado) {
			t.Errorf("%s (%q) dio %v; esperaba ErrNombreCarroDuplicado", que, nombre, err)
		}
	}

	// Y uno que de verdad es otro carro sí entra.
	otro, err := domain.NuevoCarro(NuevoID(), "Carro EDUTEC 2", "")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCarro(ctx, otro); err != nil {
		t.Errorf("«Carro EDUTEC 2» es otro carro y tenía que entrar: %v", err)
	}
}

// El nombre sólo identifica a los equipos que NO están en un carro: los de
// carro se distinguen por su zócalo. El índice es parcial y esto lo fija.
func TestUnicidad_EquipoSueltoPorNombre(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	primero, err := domain.NuevoEquipoSuelto(NuevoID(), "Proyector", "Proyector Epson", "", false, time.Now())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearEquipo(ctx, primero); err != nil {
		t.Fatalf("el primero tenía que entrar: %v", err)
	}

	for nombre, que := range variantesDe("Proyector Epson") {
		e, err := domain.NuevoEquipoSuelto(NuevoID(), "Proyector", nombre, "", false, time.Now())
		if err != nil {
			t.Errorf("el dominio rechazó %q (%s): %v", nombre, que, err)
			continue
		}
		if err := repo.CrearEquipo(ctx, e); !errors.Is(err, application.ErrNombreDeEquipoDuplicado) {
			t.Errorf("%s (%q) dio %v; esperaba ErrNombreDeEquipoDuplicado", que, nombre, err)
		}
	}
}
