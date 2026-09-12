//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/ramiro/sgrc/internal/shared/testdb"
	"github.com/ramiro/sgrc/internal/shared/texto"
)

// Este test existe por una sola razón: `clave_texto()` en la base y
// texto.Clave() en Go tienen que dar EXACTAMENTE lo mismo.
//
// La base es la que sostiene la unicidad, con seis índices colgando de esa
// función. Go es el que decide, antes de escribir, si una fila es un duplicado
// —y devuelve un 409 que se puede leer en vez del error crudo del índice—.
// Si las dos definiciones se separan, el síntoma no es un test en rojo sino una
// carga masiva que revienta contra un índice en el medio, o peor: un chequeo
// previo que rechaza algo que la base habría aceptado.
//
// Se prueba contra Postgres de verdad y no contra una copia de la expresión,
// porque el punto es justamente que no haya dos copias.
func TestClaveTextoCoincideEntreGoYLaBase(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)

	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatalf("no se pudo aplicar el esquema: %v", err)
	}

	entradas := []string{
		"Educación Física",
		"Educación  Física",
		"  EDUCACIÓN   FÍSICA  ",
		"Educación Física",
		"Carro 1",
		"carro  1",
		"Carro EDUTEC",
		"Tecnología de la Fabricación",
		"Ciencias Sociales: Historia-Formación Ética y Ciudadana",
		"Prácticas Profesionalizantes (*)",
		"Müller Ñandú",
		"5CD 1234 ABC",
		"Sector Electromecánico - Educación Técnico Profesional",
		"1ra",
		"ÁÉÍÓÚÜÑ",
		"a",
		"   ",
		"",
	}

	for _, entrada := range entradas {
		var enLaBase string
		if err := pool.QueryRow(ctx, `SELECT clave_texto($1)`, entrada).Scan(&enLaBase); err != nil {
			t.Errorf("clave_texto(%q) falló en la base: %v", entrada, err)
			continue
		}
		if enGo := texto.Clave(entrada); enGo != enLaBase {
			t.Errorf("clave_texto(%q): la base dice %q y Go dice %q — las dos definiciones se separaron",
				entrada, enLaBase, enGo)
		}
	}

	// STRICT: con NULL adentro devuelve NULL, que es lo que hace que los
	// índices parciales dejen convivir varias filas sin número de serie.
	var esNulo bool
	if err := pool.QueryRow(ctx, `SELECT clave_texto(NULL) IS NULL`).Scan(&esNulo); err != nil {
		t.Fatalf("no se pudo probar clave_texto(NULL): %v", err)
	}
	if !esNulo {
		t.Error("clave_texto(NULL) tendría que dar NULL: sin eso, dos equipos sin serie chocarían entre sí")
	}
}

// El curso es el último que se sumó a la regla (migración 015). Hasta ahí su
// único se apoyaba en las columnas generadas `division_norm` y `modalidad_norm`
// de la 009, que sólo bajan a minúsculas y sacan tildes: dos modalidades que se
// diferenciaban en un espacio doble eran dos cursos distintos.
//
// Se prueba contra la base porque lo que cambió es el índice, no el código: la
// validación de Go canoniza igual que antes, así que un test de dominio diría
// que todo anda aunque el índice hubiera quedado como estaba.
func TestCursoSeComparaConClaveTexto(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)

	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatalf("no se pudo aplicar el esquema: %v", err)
	}

	var cicloID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO ciclo_lectivo (id, anio, activo, archivado)
		VALUES (gen_random_uuid(), 2026, true, false) RETURNING id`).Scan(&cicloID); err != nil {
		t.Fatalf("creando ciclo: %v", err)
	}

	crear := func(anio int, division, modalidad string) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO curso (id, ciclo_lectivo_id, anio, division, modalidad)
			VALUES (gen_random_uuid(), $1, $2, $3, $4)`,
			cicloID, anio, division, modalidad)
		return err
	}

	if err := crear(1, "A", "Ciclo Básico"); err != nil {
		t.Fatalf("el primero tenía que entrar: %v", err)
	}

	// Las tres formas de escribir lo mismo. Sólo la del espacio doble es nueva:
	// las tildes y las mayúsculas ya las cubrían las columnas generadas.
	repetidos := map[string][2]string{
		"idéntico":                {"A", "Ciclo Básico"},
		"mayúsculas":              {"a", "CICLO BÁSICO"},
		"sin tildes":              {"A", "Ciclo Basico"},
		"con un espacio de más":   {"A", "Ciclo  Básico"},
		"con dos espacios de más": {"A", "Ciclo   Básico"},
	}
	for nombre, datos := range repetidos {
		t.Run(nombre, func(t *testing.T) {
			if err := crear(1, datos[0], datos[1]); err == nil {
				t.Errorf("«%s»/«%s» es el mismo curso y tenía que rechazarse", datos[0], datos[1])
			}
		})
	}

	// Y lo que SÍ es otro curso sigue entrando: es el mismo índice el que tiene
	// que dejar convivir a dos modalidades con su propio «1°A».
	distintos := map[string][3]any{
		"otra división":  {1, "B", "Ciclo Básico"},
		"otro año":       {2, "A", "Ciclo Básico"},
		"otra modalidad": {1, "A", "Ciclo Superior"},
	}
	for nombre, datos := range distintos {
		t.Run(nombre, func(t *testing.T) {
			if err := crear(datos[0].(int), datos[1].(string), datos[2].(string)); err != nil {
				t.Errorf("es un curso distinto y tenía que entrar: %v", err)
			}
		})
	}
}

// Un curso sin división ni modalidad —el caso normal de una universidad— tiene
// que seguir siendo único por año. Es el caso que rompería si el índice usara
// `clave_texto(division)` a secas: la función es STRICT, sobre NULL devuelve
// NULL, y en un índice único dos NULL no son iguales entre sí.
func TestCursoSinDivisionNiModalidadSigueSiendoUnico(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)

	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatalf("no se pudo aplicar el esquema: %v", err)
	}

	var cicloID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO ciclo_lectivo (id, anio, activo, archivado)
		VALUES (gen_random_uuid(), 2026, true, false) RETURNING id`).Scan(&cicloID); err != nil {
		t.Fatalf("creando ciclo: %v", err)
	}

	crearPelado := func(anio int) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO curso (id, ciclo_lectivo_id, anio, division, modalidad)
			VALUES (gen_random_uuid(), $1, $2, NULL, NULL)`, cicloID, anio)
		return err
	}

	if err := crearPelado(1); err != nil {
		t.Fatalf("el primero tenía que entrar: %v", err)
	}
	if err := crearPelado(1); err == nil {
		t.Error("un segundo 1° sin división ni modalidad tenía que rechazarse")
	}
	if err := crearPelado(2); err != nil {
		t.Errorf("2° es otro curso y tenía que entrar: %v", err)
	}
}
