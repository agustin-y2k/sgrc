//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
)

// RF-02.12 contra Postgres real. Lo que se prueba acá y no se puede probar con
// un fake es la comparación por columna NORMALIZADA: `curso.division_norm`,
// `curso.modalidad_norm` y `materia.nombre_norm` son columnas GENERADAS por la
// base, así que quién decide si una materia «ya está» es la migración y no el
// código de Go.

func cicloParaEstructura(t *testing.T, repo *PostgresRepo, anio int) string {
	t.Helper()
	ciclo, err := domain.NuevoCicloLectivo(uuidNuevo(), anio)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCiclo(context.Background(), ciclo); err != nil {
		t.Fatalf("no se pudo crear el ciclo: %v", err)
	}
	return ciclo.ID
}

func contarMaterias(t *testing.T, pool *pgxpool.Pool, cicloID string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM materia m
		  JOIN curso c ON c.id = m.curso_id
		 WHERE c.ciclo_lectivo_id = $1`, cicloID).Scan(&n)
	if err != nil {
		t.Fatalf("contando materias: %v", err)
	}
	return n
}

func TestPostgresRepo_ImportarEstructura_CreaYNoDuplica(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := cicloParaEstructura(t, repo, 2026)

	archivo := []application.CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Lengua", "Matemática"}},
		{Anio: 4, Division: "1", Modalidad: "Electromecánica", Materias: []string{"Física"}},
	}

	res, err := repo.ImportarEstructura(ctx, ciclo, archivo)
	if err != nil {
		t.Fatalf("primera importación: %v", err)
	}
	if res.CursosCreados != 2 || res.MateriasCreadas != 3 {
		t.Fatalf("esperaba 2 cursos y 3 materias, obtuve %d y %d", res.CursosCreados, res.MateriasCreadas)
	}

	// La segunda pasada no crea nada: es lo que permite reintentar sin mirar
	// antes qué había entrado.
	res, err = repo.ImportarEstructura(ctx, ciclo, archivo)
	if err != nil {
		t.Fatalf("segunda importación: %v", err)
	}
	if res.CursosCreados != 0 || res.MateriasCreadas != 0 {
		t.Errorf("la segunda pasada creó %d cursos y %d materias", res.CursosCreados, res.MateriasCreadas)
	}
	if n := contarMaterias(t, pool, ciclo); n != 3 {
		t.Errorf("esperaba 3 materias en la base, hay %d", n)
	}
}

// El UNIQUE de `materia` es sensible a mayúsculas y acentos, así que sin
// comparar por `nombre_norm` la base acepta «Matemática» y «Matematica» en el
// mismo curso — y el docente que después elige su materia ve las dos.
func TestPostgresRepo_ImportarEstructura_ComparaPorNombreNormalizado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := cicloParaEstructura(t, repo, 2026)

	if _, err := repo.ImportarEstructura(ctx, ciclo, []application.CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Matemática"}},
	}); err != nil {
		t.Fatalf("primera importación: %v", err)
	}

	res, err := repo.ImportarEstructura(ctx, ciclo, []application.CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"MATEMATICA"}},
	})
	if err != nil {
		t.Fatalf("segunda importación: %v", err)
	}
	if res.MateriasCreadas != 0 {
		t.Errorf("«MATEMATICA» no debería crearse junto a «Matemática»")
	}
	if n := contarMaterias(t, pool, ciclo); n != 1 {
		t.Errorf("esperaba 1 materia, hay %d", n)
	}
}

// Lo mismo para el curso: la terna normalizada es la que tiene el índice único.
// Sin esto, «4°1 Electromecánica» y «4°1 electromecanica» chocan contra
// ux_curso_ciclo_nombre y la importación entera falla con un 500.
func TestPostgresRepo_ImportarEstructura_CursoPorTernaNormalizada(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := cicloParaEstructura(t, repo, 2026)

	if _, err := repo.ImportarEstructura(ctx, ciclo, []application.CursoConMaterias{
		{Anio: 4, Division: "1", Modalidad: "Electromecánica", Materias: []string{"Física"}},
	}); err != nil {
		t.Fatalf("primera importación: %v", err)
	}

	res, err := repo.ImportarEstructura(ctx, ciclo, []application.CursoConMaterias{
		{Anio: 4, Division: "1", Modalidad: "electromecanica", Materias: []string{"Química"}},
	})
	if err != nil {
		t.Fatalf("segunda importación: %v", err)
	}
	if res.CursosCreados != 0 {
		t.Errorf("el curso debería reconocerse como el mismo, se creó otro")
	}
	if res.MateriasCreadas != 1 {
		t.Errorf("la materia nueva debería haberse agregado al curso existente")
	}
}

// Un curso sin materias tiene que salir igual en la exportación: es justamente
// el que hay que ver al mirar lo que falta cargar.
func TestPostgresRepo_ListarEstructuraDeCiclo_IncluyeCursosVacios(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := cicloParaEstructura(t, repo, 2026)

	if _, err := repo.ImportarEstructura(ctx, ciclo, []application.CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Lengua"}},
		{Anio: 1, Division: "2"},
	}); err != nil {
		t.Fatalf("importando: %v", err)
	}

	estructura, err := repo.ListarEstructuraDeCiclo(ctx, ciclo)
	if err != nil {
		t.Fatalf("exportando: %v", err)
	}
	if len(estructura) != 2 {
		t.Fatalf("esperaba 2 cursos, obtuve %d", len(estructura))
	}

	porDivision := map[string][]string{}
	for _, c := range estructura {
		porDivision[c.Division] = c.Materias
	}
	if len(porDivision["1"]) != 1 || porDivision["1"][0] != "Lengua" {
		t.Errorf("1°1 debería traer su materia, trajo %v", porDivision["1"])
	}
	if len(porDivision["2"]) != 0 {
		t.Errorf("1°2 no tiene materias y debería venir con la lista vacía, trajo %v", porDivision["2"])
	}
}

func TestPostgresRepo_CopiarMateriasA_SalteaLasQueYaEstan(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := cicloParaEstructura(t, repo, 2026)

	if _, err := repo.ImportarEstructura(ctx, ciclo, []application.CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Lengua", "Matemática"}},
		{Anio: 1, Division: "2", Materias: []string{"lengua"}},
		{Anio: 1, Division: "3"},
	}); err != nil {
		t.Fatalf("preparando el ciclo: %v", err)
	}

	cursos, err := repo.ListarCursosPorCiclo(ctx, ciclo)
	if err != nil {
		t.Fatalf("listando cursos: %v", err)
	}
	porDivision := map[string]string{}
	for _, c := range cursos {
		porDivision[c.Division] = c.ID
	}

	res, err := repo.CopiarMateriasA(ctx, porDivision["1"],
		[]string{porDivision["2"], porDivision["3"]})
	if err != nil {
		t.Fatalf("copiando: %v", err)
	}

	// 1°2 ya tenía «lengua» —con otra grafía— así que sólo recibe Matemática;
	// 1°3 recibe las dos.
	if res.MateriasCreadas != 3 || res.MateriasExistentes != 1 {
		t.Errorf("esperaba 3 creadas y 1 existente, obtuve %d y %d",
			res.MateriasCreadas, res.MateriasExistentes)
	}
	if n := contarMaterias(t, pool, ciclo); n != 6 {
		t.Errorf("esperaba 6 materias en el ciclo, hay %d", n)
	}
}

// Si una fila falla, no tiene que quedar nada a medias: el Admin no puede tener
// que revisar la pantalla curso por curso para saber qué entró, que es
// exactamente el trabajo que la importación venía a evitar.
func TestPostgresRepo_ImportarEstructura_EsAtomica(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := cicloParaEstructura(t, repo, 2026)

	_, err := repo.ImportarEstructura(ctx, ciclo, []application.CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Lengua"}},
		// Un nombre más largo que VARCHAR(100) hace fallar la SEGUNDA fila,
		// cuando la primera ya escribió. Se fuerza desde acá porque el servicio
		// lo rechaza antes (domain.MaxLargoMateria) y nunca llegaría así.
		{Anio: 2, Division: "1", Materias: []string{strings.Repeat("x", 200)}},
	})
	if err == nil {
		t.Fatal("esperaba que la importación falle")
	}
	if n := contarMaterias(t, pool, ciclo); n != 0 {
		t.Errorf("una importación fallida no debería dejar materias, hay %d", n)
	}

	var cursos int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM curso WHERE ciclo_lectivo_id = $1`, ciclo).Scan(&cursos); err != nil {
		t.Fatalf("contando cursos: %v", err)
	}
	if cursos != 0 {
		t.Errorf("una importación fallida no debería dejar cursos, hay %d", cursos)
	}
}

// La misma materia no entra dos veces en el mismo curso, y quien lo garantiza
// es el índice único de la migración 010 — no el código de Go. Por eso se
// prueba acá: el chequeo previo de la carga masiva puede tener un criterio y
// la base otro, y el día que se separen esto falla.
func TestPostgresRepo_MateriaUnicaPorCurso(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := cicloParaEstructura(t, repo, 2026)

	if _, err := repo.ImportarEstructura(ctx, ciclo, []application.CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Matemática"}},
		{Anio: 1, Division: "2"},
	}); err != nil {
		t.Fatalf("preparando el ciclo: %v", err)
	}

	cursos, err := repo.ListarCursosPorCiclo(ctx, ciclo)
	if err != nil {
		t.Fatalf("listando cursos: %v", err)
	}
	porDivision := map[string]string{}
	for _, c := range cursos {
		porDivision[c.Division] = c.ID
	}

	// El camino de a uno —el que se usa todos los días— con las variantes que
	// el UNIQUE por nombre exacto dejaba pasar.
	// Las variantes que produce escribir en castellano: con y sin tilde, en
	// cualquier caja. El acento GRAVE («à») queda deliberadamente afuera de la
	// normalización —la expresión de la base cubre áéíóúüñ, que es el
	// castellano— así que no se prueba acá: sería fijar como garantía algo que
	// el esquema no promete.
	// Las tres primeras son las variantes de tilde y caja. Las dos últimas son
	// el caso que NO cierra la normalización de la base y sí la canonización de
	// espacios del dominio: en pantalla se ven idénticas al original.
	for _, variante := range []string{
		"Matemática", "MATEMATICA", "matematica", "MaTeMáTiCa",
		"Matemática ", "  Matemática  ",
	} {
		m, err := domain.NuevaMateria(uuidNuevo(), porDivision["1"], variante)
		if err != nil {
			t.Fatalf("el dominio rechazó %q, que es un nombre válido: %v", variante, err)
		}
		if err := repo.CrearMateria(ctx, m); !errors.Is(err, application.ErrMateriaNombreDuplicado) {
			t.Errorf("«%s» dio %v; esperaba ErrMateriaNombreDuplicado", variante, err)
		}
	}

	// La misma materia en OTRO curso sí entra: una materia es propia de su
	// curso, la Matemática de 1°1 no es la de 1°2.
	m, err := domain.NuevaMateria(uuidNuevo(), porDivision["2"], "Matemática")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearMateria(ctx, m); err != nil {
		t.Errorf("la misma materia en otro curso tenía que entrar: %v", err)
	}
}

// Los cursos salen ordenados por AÑO, no agrupados por modalidad.
//
// Agrupar por modalidad primero deja los años salteados —3°, 6°, 3°, 4°, 1°…—
// porque cada modalidad recorre los suyos de nuevo, y en una secundaria técnica
// con cuatro modalidades eso es la lista entera desordenada. El año es el eje
// por el que una escuela mira sus cursos.
func TestPostgresRepo_ListarCursosPorCiclo_OrdenadosPorAnio(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := cicloParaEstructura(t, repo, 2026)

	// Se cargan desordenados y con dos modalidades que alfabéticamente van al
	// revés del año, que es lo que hacía fallar el orden viejo.
	if _, err := repo.ImportarEstructura(ctx, ciclo, []application.CursoConMaterias{
		{Anio: 6, Division: "1", Modalidad: "Sector Electromecánico"},
		{Anio: 1, Division: "2", Modalidad: "Ciclo Básico"},
		{Anio: 4, Division: "3", Modalidad: "Bachiller en Economía"},
		{Anio: 1, Division: "1", Modalidad: "Ciclo Básico"},
		// Dos dígitos: ordenada como texto, "10" cae entre "1" y "2".
		{Anio: 1, Division: "10", Modalidad: "Ciclo Básico"},
	}); err != nil {
		t.Fatalf("preparando el ciclo: %v", err)
	}

	cursos, err := repo.ListarCursosPorCiclo(ctx, ciclo)
	if err != nil {
		t.Fatalf("listando cursos: %v", err)
	}

	var nombres []string
	for _, c := range cursos {
		nombres = append(nombres, c.Nombre)
	}
	esperado := []string{"1°1", "1°2", "1°10", "4°3", "6°1"}
	if len(nombres) != len(esperado) {
		t.Fatalf("esperaba %d cursos, obtuve %v", len(esperado), nombres)
	}
	for i := range esperado {
		if nombres[i] != esperado[i] {
			t.Fatalf("orden %v; esperaba %v", nombres, esperado)
		}
	}

	// La descarga de la estructura usa el mismo orden: el CSV que se baja se
	// lee como la lista que se ve en pantalla.
	estructura, err := repo.ListarEstructuraDeCiclo(ctx, ciclo)
	if err != nil {
		t.Fatalf("exportando: %v", err)
	}
	for i, c := range estructura {
		if domain.ComponerNombre(c.Anio, c.Division) != esperado[i] {
			t.Errorf("la descarga salió en otro orden que el listado: fila %d es %d°%s, esperaba %s",
				i, c.Anio, c.Division, esperado[i])
		}
	}
}

// El doble espacio interno es el caso que la normalización de la BASE no ve:
// `nombre_norm` saca tildes y mayúsculas, no colapsa espacios. Lo cierra la
// canonización del dominio, que guarda el nombre ya con un solo espacio, y por
// eso hay que probarlo de punta a punta —dominio + base— y no sólo en uno.
//
// Importa porque «Educación Física» y «Educación  Física» se imprimen iguales
// en toda pantalla del sistema: si entran las dos, nadie puede ver por qué la
// materia aparece duplicada ni cuál de las dos borrar.
func TestPostgresRepo_MateriaNoSeRepitePorEspaciosDeMas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := cicloParaEstructura(t, repo, 2026)

	if _, err := repo.ImportarEstructura(ctx, ciclo, []application.CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Educación Física"}},
	}); err != nil {
		t.Fatalf("preparando el ciclo: %v", err)
	}
	cursos, err := repo.ListarCursosPorCiclo(ctx, ciclo)
	if err != nil {
		t.Fatalf("listando cursos: %v", err)
	}
	cursoID := cursos[0].ID

	for _, variante := range []string{
		"Educación  Física",     // dos espacios
		"Educación   Física",    // tres
		"  Educación  Física  ", // y además en los bordes
		"Educación Física",      // espacio duro, el de copiar desde una web
	} {
		m, err := domain.NuevaMateria(uuidNuevo(), cursoID, variante)
		if err != nil {
			t.Fatalf("el dominio rechazó %q, que es un nombre válido: %v", variante, err)
		}
		if err := repo.CrearMateria(ctx, m); !errors.Is(err, application.ErrMateriaNombreDuplicado) {
			t.Errorf("«%s» dio %v; esperaba ErrMateriaNombreDuplicado", variante, err)
		}
	}

	// Y la carga masiva hace lo mismo: la planilla es justamente de donde salen
	// los dobles espacios.
	res, err := repo.ImportarEstructura(ctx, ciclo, []application.CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Educación Física"}},
	})
	if err != nil {
		t.Fatalf("recargando: %v", err)
	}
	if res.MateriasCreadas != 0 {
		t.Errorf("la carga masiva creó %d materias; no tenía que crear ninguna", res.MateriasCreadas)
	}
	if n := contarMaterias(t, pool, ciclo); n != 1 {
		t.Errorf("el curso quedó con %d materias, esperaba 1", n)
	}
}
