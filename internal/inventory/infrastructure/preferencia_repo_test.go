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

func nuevaPreferenciaDeTest(t *testing.T, repo *PostgresRepo, equipoID, materia string, anio *int, division *string, prioridad int) *domain.PreferenciaDeEquipo {
	t.Helper()
	p, err := domain.NuevaPreferencia(NuevoID(), equipoID, materia, anio, division, prioridad)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearPreferencia(context.Background(), p); err != nil {
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

	creada := nuevaPreferenciaDeTest(t, repo, equipoID, "Dibujo Técnico", ptrInt(3), ptrStr("B"), 2)

	guardada, err := repo.BuscarPreferenciaPorID(ctx, creada.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if guardada.MateriaNombre != "Dibujo Técnico" || *guardada.Anio != 3 || *guardada.Division != "B" {
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

	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, nil, 1)

	otra, _ := domain.NuevaPreferencia(NuevoID(), equipoID, "Matemática", nil, nil, 3)
	err := repo.CrearPreferencia(ctx, otra)

	if !errors.Is(err, domain.ErrPreferenciaDuplicada) {
		t.Fatalf("esperaba ErrPreferenciaDuplicada, obtuve %v", err)
	}
}

// La unicidad ignora acentos y mayúsculas porque compara la columna
// generada, no el texto tal cual se escribió.
func TestPostgresRepo_PreferenciaDuplicadaSinAcentos(t *testing.T) {
	repo := NewPostgresRepo(levantarPostgresDeTest(t))
	ctx := context.Background()
	equipoID := equipoParaPreferencias(t, repo, "SERIE-PREF-3")

	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, nil, 1)

	otra, _ := domain.NuevaPreferencia(NuevoID(), equipoID, "matematica", nil, nil, 1)
	err := repo.CrearPreferencia(ctx, otra)

	if !errors.Is(err, domain.ErrPreferenciaDuplicada) {
		t.Fatalf("'matematica' y 'Matemática' son la misma materia: %v", err)
	}
}

// Dos alcances distintos de la misma materia SÍ conviven: uno general y uno
// acotado a un curso son marcas diferentes, y la de 3°B es la que gana
// cuando se reserva 3°B.
func TestPostgresRepo_PreferenciaDistintoAlcance_Conviven(t *testing.T) {
	repo := NewPostgresRepo(levantarPostgresDeTest(t))
	ctx := context.Background()
	equipoID := equipoParaPreferencias(t, repo, "SERIE-PREF-4")

	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, nil, 1)
	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", ptrInt(3), nil, 2)
	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", ptrInt(3), ptrStr("B"), 3)

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

	p := nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, nil, 1)

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

	nuevaPreferenciaDeTest(t, repo, equipoID, "Matemática", nil, nil, 1)

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

func sembrarMaterias(t *testing.T, pool *pgxpool.Pool, nombres ...string) {
	t.Helper()
	ctx := context.Background()
	cicloID, cursoID := NuevoID(), NuevoID()

	if _, err := pool.Exec(ctx, `INSERT INTO ciclo_lectivo (id, anio, activo) VALUES ($1, 3001, false)`, cicloID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO curso (id, ciclo_lectivo_id, nombre) VALUES ($1, $2, '3°B')`, cursoID, cicloID); err != nil {
		t.Fatal(err)
	}
	for _, n := range nombres {
		if _, err := pool.Exec(ctx, `INSERT INTO materia (id, curso_id, nombre) VALUES ($1, $2, $3)`, NuevoID(), cursoID, n); err != nil {
			t.Fatal(err)
		}
	}
}
