//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

func ptrInt(n int) *int       { return &n }
func ptrStr(s string) *string { return &s }

func nuevaPreferenciaDeTest(t *testing.T, repo *PostgresRepo, equipoID, materia string,
	modalidad *string, anio *int, division *string, prioridad int) *domain.PreferenciaDeEquipo {
	t.Helper()
	p, err := domain.NuevaPreferencia(NuevoID(), equipoID, materia, modalidad, anio, division, prioridad)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if _, err := repo.CrearPreferencia(context.Background(), p); err != nil {
		t.Fatalf("no se pudo crear la preferencia de prueba: %v", err)
	}
	return p
}

func equipoParaPreferencias(t *testing.T, repo *PostgresRepo, serie string) string {
	t.Helper()
	carro := crearCarroDeTest(t, repo, "Carro-"+serie)
	return crearEquipoDeCarroDeTest(t, repo, carro.ID, 1, serie).ID
}

func TestPostgresRepo_PreferenciaRoundTrip(t *testing.T) {
	repo := NewPostgresRepo(levantarPostgresDeTest(t))
	ctx := context.Background()
	equipoID := equipoParaPreferencias(t, repo, "SERIE-PREF-1")

	creada := nuevaPreferenciaDeTest(t, repo, equipoID, "Dibujo Técnico", ptrStr("Construcción"), ptrInt(3), ptrStr("B"), 2)

	guardada, err := repo.BuscarPreferenciaPorID(ctx, creada.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if guardada.MateriaNombre != "Dibujo Técnico" || *guardada.Anio != 3 ||
		*guardada.Division != "B" || *guardada.Modalidad != "Construcción" {
		t.Errorf("volvió distinta: %+v", guardada)
	}
	if guardada.Prioridad != 2 {
		t.Errorf("prioridad = %d, esperaba 2", guardada.Prioridad)
	}
}

// El UNIQUE de la tabla lleva NULLS NOT DISTINCT justamente por esto: con el
// UNIQUE normal de SQL dos marcas sin año se consideran distintas entre sí y
// la misma se podría cargar infinitas veces.
func TestPostgresRepo_PreferenciaSinCurso_NoSePuedeDuplicar(t *testing.T) {
	repo := NewPostgresRepo(levantarPostgresDeTest(t))
	ctx := context.Background()
	equipoID := equipoParaPreferencias(t, repo, "SERIE-PREF-2")

	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, nil, nil, 1)

	otra, _ := domain.NuevaPreferencia(NuevoID(), equipoID, "Matemática", nil, nil, nil, 3)
	creada, err := repo.CrearPreferencia(ctx, otra)

	// Igual que CrearLicencia: el duplicado vuelve como «no la creé», no como
	// error, para no abortar la transacción del lote.
	if err != nil {
		t.Fatalf("un duplicado no tendría que dar error: %v", err)
	}
	if creada {
		t.Error("la misma marca no tendría que crearse dos veces")
	}
}

// La unicidad ignora acentos y mayúsculas porque compara la columna
// generada, no el texto tal cual se escribió.
func TestPostgresRepo_PreferenciaDuplicadaSinAcentos(t *testing.T) {
	repo := NewPostgresRepo(levantarPostgresDeTest(t))
	ctx := context.Background()
	equipoID := equipoParaPreferencias(t, repo, "SERIE-PREF-3")

	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, nil, nil, 1)

	otra, _ := domain.NuevaPreferencia(NuevoID(), equipoID, "matematica", nil, nil, nil, 1)
	creada, err := repo.CrearPreferencia(ctx, otra)

	if err != nil {
		t.Fatalf("un duplicado no tendría que dar error: %v", err)
	}
	if creada {
		t.Error("'matematica' y 'Matemática' son la misma materia y no tendrían que convivir")
	}
}

// Dos alcances distintos de la misma materia SÍ conviven: uno general y uno
// acotado a un curso son marcas diferentes, y la de 3°B es la que gana cuando
// se reserva 3°B.
func TestPostgresRepo_PreferenciaDistintoAlcance_Conviven(t *testing.T) {
	repo := NewPostgresRepo(levantarPostgresDeTest(t))
	ctx := context.Background()
	equipoID := equipoParaPreferencias(t, repo, "SERIE-PREF-4")

	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, nil, nil, 1)
	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, ptrInt(3), nil, 2)
	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, ptrInt(3), ptrStr("B"), 3)

	marcas, err := repo.ListarPreferenciasPorEquipo(ctx, equipoID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(marcas) != 3 {
		t.Fatalf("esperaba 3 marcas de distinto alcance, obtuve %d", len(marcas))
	}
}

// Borrar la marca no toca nada más: el equipo vuelve al orden neutral.
func TestPostgresRepo_BorrarPreferencia(t *testing.T) {
	repo := NewPostgresRepo(levantarPostgresDeTest(t))
	ctx := context.Background()
	equipoID := equipoParaPreferencias(t, repo, "SERIE-PREF-5")

	p := nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, nil, nil, 1)

	if err := repo.BorrarPreferencia(ctx, p.ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if _, err := repo.BuscarPreferenciaPorID(ctx, p.ID); !errors.Is(err, domain.ErrPreferenciaNoEncontr) {
		t.Errorf("esperaba ErrPreferenciaNoEncontr, obtuve %v", err)
	}
	if err := repo.BorrarPreferencia(ctx, p.ID); !errors.Is(err, domain.ErrPreferenciaNoEncontr) {
		t.Errorf("borrar dos veces tiene que dar 404, obtuve %v", err)
	}
}

// Dar de baja el equipo se lleva sus marcas por la FK ON DELETE CASCADE.
func TestPostgresRepo_PreferenciasSeBorranConElEquipo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipoID := equipoParaPreferencias(t, repo, "SERIE-PREF-6")

	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, nil, nil, 1)

	if _, err := pool.Exec(ctx, `DELETE FROM equipo WHERE id = $1`, equipoID); err != nil {
		t.Fatal(err)
	}

	marcas, err := repo.ListarPreferenciasPorEquipo(ctx, equipoID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(marcas) != 0 {
		t.Errorf("las marcas tendrían que haberse ido con el equipo, quedaron %d", len(marcas))
	}
}

// El selector del formulario colapsa las variantes del mismo nombre: sin
// esto, "Matemática" y "matematica" serían dos opciones y el Admin podría
// marcar cada máquina con una distinta.
//
// Las variantes van en CURSOS DISTINTOS, y eso dejó de ser un detalle del
// armado: desde la migración 011 la base no acepta dos variantes del mismo
// nombre dentro de un curso (RF-00.1). Pero entre cursos sí —una materia es
// propia de su curso, y nada obliga a 1°A y a 1°B a escribirla igual—, así que
// el caso que este test cubre sigue existiendo. Lo que cambió es que ahora es
// el ÚNICO camino por el que puede aparecer, no uno de dos.
func TestPostgresRepo_NombresDeMateriaEnUso_SinRepetirVariantes(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	sembrarMaterias(t, pool, "Matemática", "matematica", "Dibujo Técnico")

	nombres, err := repo.NombresDeMateriaEnUso(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(nombres) != 2 {
		t.Fatalf("esperaba 2 nombres distintos, obtuve %v", nombres)
	}
}

// sembrarMaterias deja cada nombre en SU PROPIO curso.
//
// Uno por curso y no todos en el mismo porque la base ya no aceptaría dos
// variantes del mismo nombre juntas, y porque así el armado describe el caso
// real: cada división escribe sus materias por su cuenta.
func sembrarMaterias(t *testing.T, pool *pgxpool.Pool, nombres ...string) {
	t.Helper()
	ctx := context.Background()
	cicloID := NuevoID()

	if _, err := pool.Exec(ctx, `INSERT INTO ciclo_lectivo (id, anio, activo) VALUES ($1, 3001, false)`, cicloID); err != nil {
		t.Fatal(err)
	}
	for i, n := range nombres {
		cursoID := NuevoID()
		if _, err := pool.Exec(ctx,
			`INSERT INTO curso (id, ciclo_lectivo_id, anio, division) VALUES ($1, $2, 3, $3)`,
			cursoID, cicloID, string(rune('A'+i))); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO materia (id, curso_id, nombre) VALUES ($1, $2, $3)`, NuevoID(), cursoID, n); err != nil {
			t.Fatal(err)
		}
	}
}

// ── Marcas huérfanas (RF-03.21) ─────────────────────────────────────────
//
// Una marca se vincula a la materia POR NOMBRE y no por referencia: es
// deliberado, para que sobrevivan al clonado anual del ciclo, cuando cada
// materia nace con otro identificador. El precio es que renombrar o borrar una
// materia deja la marca apuntando a un nombre que ya no existe —en silencio, y
// siguiendo a la vista en la ficha del equipo como si valiera—.
//
// Esta consulta es la única forma de encontrarlas.

func TestListarPreferenciasHuerfanas_RenombrarLaMateriaDejaLaMarcaColgando(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipoID := equipoParaPreferencias(t, repo, "SERIE-HUERF-1")

	sembrarMaterias(t, pool, "Matemática", "Lengua")
	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, nil, nil, 1)
	nuevaPreferenciaDeTest(t, repo, equipoID, "Lengua", nil, nil, nil, 2)

	// Con las dos materias cargadas, ninguna marca está huérfana.
	huerfanas, err := repo.ListarPreferenciasHuerfanas(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(huerfanas) != 0 {
		t.Fatalf("esperaba ninguna huérfana, obtuve %d", len(huerfanas))
	}

	// El renombre: la marca sigue diciendo «Matemática».
	if _, err := pool.Exec(ctx,
		`UPDATE materia SET nombre = 'Matemática I' WHERE nombre = 'Matemática'`); err != nil {
		t.Fatalf("renombrando: %v", err)
	}

	huerfanas, err = repo.ListarPreferenciasHuerfanas(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(huerfanas) != 1 {
		t.Fatalf("esperaba 1 huérfana, obtuve %d", len(huerfanas))
	}
	if huerfanas[0].MateriaNombre != "Matemática" {
		t.Errorf("la huérfana tiene que decir a qué nombre apunta; dijo %q", huerfanas[0].MateriaNombre)
	}
	// Trae el equipo resuelto: la marca sola no dice nada accionable, lo que
	// hace falta saber es QUÉ máquina quedó marcada para una materia que ya no
	// existe.
	if huerfanas[0].EquipoEtiqueta == "" {
		t.Error("la huérfana tiene que decir de qué equipo es")
	}
}

// Una tilde o una mayúscula de diferencia NO hacen huérfana a una marca: el
// cruce usa clave_texto, la misma función que sostiene la unicidad del sistema
// (RF-00.1). Sin eso, media escuela aparecería como huérfana.
func TestListarPreferenciasHuerfanas_NoConfundeUnaTildeConUnRenombre(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	equipoID := equipoParaPreferencias(t, repo, "SERIE-HUERF-2")

	sembrarMaterias(t, pool, "Matemática")
	// La marca se escribió sin tilde y en minúsculas.
	nuevaPreferenciaDeTest(t, repo, equipoID, "matematica", nil, nil, nil, 1)

	huerfanas, err := repo.ListarPreferenciasHuerfanas(context.Background())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(huerfanas) != 0 {
		t.Errorf("«matematica» y «Matemática» son la misma materia; salió como huérfana: %+v", huerfanas[0])
	}
}

// Y la cuenta previa, que es lo que permite AVISAR antes de renombrar en vez de
// romper en silencio.
func TestContarPreferenciasQueDejarianDeAplicar(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	e1 := equipoParaPreferencias(t, repo, "SERIE-CUENTA-1")
	e2 := equipoParaPreferencias(t, repo, "SERIE-CUENTA-2")

	sembrarMaterias(t, pool, "Matemática")
	nuevaPreferenciaDeTest(t, repo, e1, "Matemática", nil, nil, nil, 1)
	nuevaPreferenciaDeTest(t, repo, e2, "Matemática", nil, nil, nil, 1)

	n, err := repo.ContarPreferenciasQueDejarianDeAplicar(ctx, "Matemática")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 2 {
		t.Errorf("esperaba 2, obtuve %d", n)
	}

	// Sin distinguir tildes ni mayúsculas, igual que el cruce.
	n, err = repo.ContarPreferenciasQueDejarianDeAplicar(ctx, "  MATEMATICA  ")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 2 {
		t.Errorf("la cuenta tiene que ignorar tildes y mayúsculas; obtuve %d", n)
	}

	if n, _ := repo.ContarPreferenciasQueDejarianDeAplicar(ctx, "Lengua"); n != 0 {
		t.Errorf("una materia sin marcas tiene que dar 0; obtuve %d", n)
	}
}
