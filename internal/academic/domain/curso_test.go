package domain

import (
	"errors"
	"strings"
	"testing"
)

// El año es lo ÚNICO obligatorio, y la división y la modalidad son opcionales
// porque no todos los ámbitos las tienen (RF-02.2).
func TestValidarCurso_LosCuatroAmbitos(t *testing.T) {
	casos := []struct {
		ambito    string
		anio      int
		division  string
		modalidad string
		nombre    string
	}{
		{"primaria", 7, "A", "", "7°A"},
		{"secundaria, ciclo básico", 1, "1", "", "1°1"},
		{"secundaria técnica, ciclo superior", 4, "2", "Electromecánica", "4°2"},
		{"terciario sin división", 2, "", "Tec. Sup. en Enfermería", "2°"},
		{"universidad con comisión", 3, "B", "Medicina", "3°B"},
	}

	for _, c := range casos {
		t.Run(c.ambito, func(t *testing.T) {
			curso, err := NuevoCurso("id1", "ciclo1", c.anio, c.division, c.modalidad)
			if err != nil {
				t.Fatalf("no debería fallar: %v", err)
			}
			if curso.Nombre != c.nombre {
				t.Errorf("nombre = %q, esperaba %q", curso.Nombre, c.nombre)
			}
		})
	}
}

// El rango es un tope de sanidad, no una regla: séptimo año existe en primaria
// y en varias carreras.
func TestValidarCurso_Anio(t *testing.T) {
	if _, _, err := ValidarCurso(7, "", ""); err != nil {
		t.Errorf("séptimo año tiene que entrar: %v", err)
	}
	for _, anio := range []int{0, -1, MaxAnioCurso + 1} {
		if _, _, err := ValidarCurso(anio, "", ""); !errors.Is(err, ErrAnioCursoInvalido) {
			t.Errorf("el año %d debería ser inválido, obtuve: %v", anio, err)
		}
	}
}

// Recortar y validar en un solo paso: si el recorte quedara para después, una
// división con un espacio al final llegaría al CHECK de la base como un 500.
func TestValidarCurso_RecortaLosBordes(t *testing.T) {
	division, modalidad, err := ValidarCurso(4, "  2  ", "  Electromecánica  ")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if division != "2" || modalidad != "Electromecánica" {
		t.Errorf("quedó división=%q modalidad=%q", division, modalidad)
	}
}

func TestValidarCurso_Largos(t *testing.T) {
	if _, _, err := ValidarCurso(1, strings.Repeat("a", MaxLargoDivision+1), ""); !errors.Is(err, ErrDivisionLarga) {
		t.Errorf("esperaba ErrDivisionLarga, obtuve: %v", err)
	}
	if _, _, err := ValidarCurso(1, "", strings.Repeat("a", MaxLargoModalidad+1)); !errors.Is(err, ErrModalidadLarga) {
		t.Errorf("esperaba ErrModalidadLarga, obtuve: %v", err)
	}
}

func TestValidarCurso_Ilegible(t *testing.T) {
	// Adentro, no en el borde: un salto de línea al final lo recorta el trim,
	// y eso está bien. Lo que no se puede mostrar es uno en el medio.
	if _, _, err := ValidarCurso(1, "A\nB", ""); !errors.Is(err, ErrTextoIlegible) {
		t.Errorf("esperaba ErrTextoIlegible, obtuve: %v", err)
	}
	if _, _, err := ValidarCurso(1, "A\n", ""); err != nil {
		t.Errorf("en el borde lo recorta el trim: %v", err)
	}
}

// El nombre no se guarda: lo calcula la base a partir del año y la división.
// ComponerNombre es la misma expresión de este lado, para poder mostrar un
// curso recién creado sin volver a leerlo.
func TestComponerNombre(t *testing.T) {
	casos := []struct {
		anio     int
		division string
		esperado string
	}{
		{4, "2", "4°2"},
		{1, "A", "1°A"},
		{2, "", "2°"},
		{12, "B", "12°B"},
	}
	for _, c := range casos {
		if n := ComponerNombre(c.anio, c.division); n != c.esperado {
			t.Errorf("ComponerNombre(%d, %q) = %q, esperaba %q", c.anio, c.division, n, c.esperado)
		}
	}
}

func TestNuevoCurso_OK(t *testing.T) {
	c, err := NuevoCurso("id1", "ciclo1", 4, "2", "Electromecánica")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if c.Archivado {
		t.Errorf("un curso nuevo debería ser Archivado=false: %+v", c)
	}
	if c.Anio != 4 || c.Division != "2" || c.Modalidad != "Electromecánica" {
		t.Errorf("datos del curso: %+v", c)
	}
}

func TestNuevoCurso_AnioInvalido_Error(t *testing.T) {
	if _, err := NuevoCurso("id1", "ciclo1", 0, "", ""); !errors.Is(err, ErrAnioCursoInvalido) {
		t.Fatalf("esperaba ErrAnioCursoInvalido, obtuve %v", err)
	}
}

func TestEditar_OK(t *testing.T) {
	c, _ := NuevoCurso("id1", "ciclo1", 1, "1", "")

	if err := c.Editar(4, "2", "Construcción"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if c.Anio != 4 || c.Division != "2" || c.Modalidad != "Construcción" {
		t.Errorf("no se actualizó: %+v", c)
	}
	if c.Nombre != "4°2" {
		t.Errorf("el nombre tenía que seguir a las partes, quedó %q", c.Nombre)
	}
}

// Los tres datos se editan juntos porque son un solo curso descrito de tres
// formas: si falla la validación, no se aplica ninguno.
func TestEditar_Invalido_NoModificaNada(t *testing.T) {
	c, _ := NuevoCurso("id1", "ciclo1", 1, "1", "Ciclo Básico")

	err := c.Editar(99, "2", "Construcción")

	if !errors.Is(err, ErrAnioCursoInvalido) {
		t.Fatalf("esperaba ErrAnioCursoInvalido, obtuve %v", err)
	}
	if c.Anio != 1 || c.Division != "1" || c.Modalidad != "Ciclo Básico" {
		t.Errorf("una edición fallida no debería modificar nada, quedó: %+v", c)
	}
}

// El nombre puede repetirse entre dos carreras, así que donde se elige o se
// busca un curso hace falta la modalidad al lado. Mismo criterio que
// "PC 1 · Carro 1" en el mostrador.
func TestEtiqueta(t *testing.T) {
	conModalidad, _ := NuevoCurso("id1", "ciclo1", 1, "A", "Enfermería")
	if conModalidad.Etiqueta() != "1°A · Enfermería" {
		t.Errorf("etiqueta = %q", conModalidad.Etiqueta())
	}

	sinModalidad, _ := NuevoCurso("id2", "ciclo1", 1, "1", "")
	if sinModalidad.Etiqueta() != "1°1" {
		t.Errorf("sin modalidad la etiqueta es el nombre pelado, es %q", sinModalidad.Etiqueta())
	}
}

// La división y la modalidad se canonizan igual que el nombre de una materia:
// «Ciclo Básico» y «Ciclo  Básico» se ven iguales y serían dos modalidades, o
// sea dos cursos distintos con el mismo nombre en la pantalla.
func TestValidarCurso_CanonizaLosEspacios(t *testing.T) {
	division, modalidad, err := ValidarCurso(4, "  1  ra  ", "Sector   Electromecánico")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if division != "1 ra" {
		t.Errorf("division = %q; esperaba %q", division, "1 ra")
	}
	if modalidad != "Sector Electromecánico" {
		t.Errorf("modalidad = %q; esperaba %q", modalidad, "Sector Electromecánico")
	}
}
