//go:build integration

package infrastructure

import (
	"context"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
)

func TestCatalogoPrioridades_ValidaRelacionesYConservaAlcances(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := NuevoID()
	if _, err := pool.Exec(ctx, `INSERT INTO ciclo_lectivo (id,anio,activo) VALUES ($1,2026,true)`, ciclo); err != nil {
		t.Fatal(err)
	}
	var materiaSinAtributos string
	for i, modalidad := range []*string{nil, ptrStr("Construcción"), ptrStr("Electromecánica")} {
		curso := NuevoID()
		var division *string
		if i > 0 {
			division = ptrStr("2")
		}
		if _, err := pool.Exec(ctx, `INSERT INTO curso (id,ciclo_lectivo_id,anio,division,modalidad) VALUES ($1,$2,3,$3,$4)`, curso, ciclo, division, modalidad); err != nil {
			t.Fatal(err)
		}
		materia := NuevoID()
		if i == 0 {
			materiaSinAtributos = materia
		}
		if _, err := pool.Exec(ctx, `INSERT INTO materia (id,curso_id,nombre) VALUES ($1,$2,'Matemática')`, materia, curso); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if _, err := pool.Exec(ctx, `INSERT INTO materia (id,curso_id,nombre) VALUES ($1,$2,'Tecnología de los Materiales')`, NuevoID(), curso); err != nil {
				t.Fatal(err)
			}
		}
	}
	filas, err := repo.CatalogoDePrioridades(ctx)
	if err != nil || len(filas) != 4 {
		t.Fatalf("catálogo: %+v, %v", filas, err)
	}
	for _, tc := range []struct {
		nombre         string
		modalidad      *string
		anio           *int
		division       *string
		exacto, existe bool
	}{
		{"tecnologia de los materiales", ptrStr("construccion"), ptrInt(3), ptrStr("2"), true, true},
		{"Tecnología de los Materiales", ptrStr("Electromecánica"), ptrInt(3), ptrStr("2"), true, false},
		{"Tecnología de los Materiales", ptrStr("Construcción"), ptrInt(4), ptrStr("2"), true, false},
		{"Tecnología de los Materiales", ptrStr("Construcción"), ptrInt(3), ptrStr("1"), true, false},
		{"Tecnología de los Materiales", nil, ptrInt(3), nil, true, false},
		{"Tecnología de los Materiales", nil, ptrInt(3), nil, false, true},
		{"Matemática", nil, ptrInt(3), nil, true, true},
	} {
		p, err := domain.NuevaPreferencia("", "", tc.nombre, tc.modalidad, tc.anio, tc.division, 1, tc.exacto)
		if err != nil {
			t.Fatal(err)
		}
		existe, err := repo.ExisteAlcancePreferencia(ctx, p)
		if err != nil || existe != tc.existe {
			t.Errorf("%+v: existe=%v, err=%v", tc, existe, err)
		}
	}
	equipo := equipoParaPreferencias(t, repo, "ALCANCE-EXACTO")
	general := nuevaPreferenciaDeTest(t, repo, equipo, "Matemática", nil, ptrInt(3), nil, 1)
	exacta, err := domain.NuevaPreferencia(NuevoID(), equipo, "Matemática", nil, ptrInt(3), nil, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if creada, err := repo.CrearPreferencia(ctx, exacta); err != nil || !creada {
		t.Fatalf("exacta debe convivir con general: %v, %v", creada, err)
	}
	for _, p := range []*domain.PreferenciaDeEquipo{general, exacta} {
		leida, err := repo.BuscarPreferenciaPorID(ctx, p.ID)
		if err != nil || leida.CursoExacto != p.CursoExacto {
			t.Fatalf("roundtrip: %+v, %v", leida, err)
		}
	}
	cuantas, err := repo.ContarPreferenciasQueDejarianDeAplicar(ctx, materiaSinAtributos, "Matemática")
	if err != nil || cuantas != 1 {
		t.Fatalf("sólo la exacta pierde su materia: %d, %v", cuantas, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM materia WHERE id=$1`, materiaSinAtributos); err != nil {
		t.Fatal(err)
	}
	huerfanas, err := repo.ListarPreferenciasHuerfanas(ctx)
	if err != nil || len(huerfanas) != 1 || huerfanas[0].ID != exacta.ID {
		t.Fatalf("huérfanas deben respetar ausencia de atributos: %+v, %v", huerfanas, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ciclo_lectivo SET activo=false WHERE id=$1`, ciclo); err != nil {
		t.Fatal(err)
	}
	filas, err = repo.CatalogoDePrioridades(ctx)
	if err != nil || len(filas) != 0 {
		t.Fatalf("el catálogo no debe ofrecer ciclos inactivos: %+v, %v", filas, err)
	}
	if existe, err := repo.ExisteAlcancePreferencia(ctx, general); err != nil || existe {
		t.Fatalf("el alcance debe existir en el ciclo activo: %v, %v", existe, err)
	}
}
