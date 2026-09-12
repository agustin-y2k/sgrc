package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNuevaMateria_OK(t *testing.T) {
	m, err := NuevaMateria("id1", "curso1", "Matemáticas")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if m.Archivado {
		t.Error("una materia nueva no debería nacer archivada")
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

// «Educación Física» y «Educación  Física» se ven IDÉNTICAS en pantalla. Sin
// canonizar los espacios son dos materias distintas para el UNIQUE de la base,
// cada una con sus reservas y sus docentes, y nadie puede ver la diferencia
// para arreglarlo.
func TestValidarNombreMateria_CanonizaLosEspacios(t *testing.T) {
	casos := []struct{ entrada, esperado string }{
		{"  Educación Física  ", "Educación Física"},
		{"Educación  Física", "Educación Física"},
		{"Ciencias   Sociales:  Geografía", "Ciencias Sociales: Geografía"},
		// El espacio duro (U+00A0) viaja al copiar desde una página web y es,
		// de todos los casos, el más difícil de ver: se imprime igual que un
		// espacio común y no lo es.
		{"Educación\u00a0Física", "Educación Física"},
		{"Educación\u00a0 Física", "Educación Física"},
	}
	// El tabulador NO entra acá: es un carácter de control y lo rechaza la regla
	// que ya existía, igual que el salto de línea. Canonizarlo sería relajar esa
	// regla, que es otra discusión.
	for _, caso := range casos {
		obtenido, err := ValidarNombreMateria(caso.entrada)
		if err != nil {
			t.Errorf("ValidarNombreMateria(%q) devolvió error: %v", caso.entrada, err)
			continue
		}
		if obtenido != caso.esperado {
			t.Errorf("ValidarNombreMateria(%q) = %q; esperaba %q", caso.entrada, obtenido, caso.esperado)
		}
	}
}

// El salto de línea sigue siendo texto ilegible y no se convierte en el espacio
// que separa dos palabras: canonizar antes de mirarlo lo dejaría entrar
// disfrazado de nombre correcto.
func TestValidarNombreMateria_ElSaltoDeLineaSigueSiendoError(t *testing.T) {
	if _, err := ValidarNombreMateria("Educación\nFísica"); !errors.Is(err, ErrTextoIlegible) {
		t.Errorf("esperaba ErrTextoIlegible, obtuve: %v", err)
	}
}

// El largo se mide sobre lo que se va a GUARDAR: un nombre que se pasa de 100
// sólo por espacios de más entra igual una vez colapsado.
func TestValidarNombreMateria_MideElLargoDespuesDeColapsar(t *testing.T) {
	nombre := strings.Repeat("a", 60) + strings.Repeat(" ", 50) + strings.Repeat("b", 30)
	obtenido, err := ValidarNombreMateria(nombre)
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if len([]rune(obtenido)) != 91 {
		t.Errorf("largo %d; esperaba 91 (60 + 1 espacio + 30)", len([]rune(obtenido)))
	}
}

// Mismo criterio que ValidarCurso: el salto de línea del borde lo recorta el
// trim y eso está bien — es basura de un copiado, no texto ilegible.
func TestValidarNombreMateria_ElSaltoDeLineaDelBordeSeRecorta(t *testing.T) {
	obtenido, err := ValidarNombreMateria("Educación Física\n")
	if err != nil {
		t.Fatalf("en el borde lo recorta el trim: %v", err)
	}
	if obtenido != "Educación Física" {
		t.Errorf("obtuve %q", obtenido)
	}
}
