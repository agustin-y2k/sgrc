package application

import (
	"context"
	"errors"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
)

func TestMarcarPreferencia_AlcanceInexistenteNoModificaElLote(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.validarAlcance = func(p *domain.PreferenciaDeEquipo) (bool, error) {
		if !p.CursoExacto || *p.Anio != 3 || *p.Modalidad != "Construcción" {
			t.Fatalf("no se validó el alcance solicitado: %+v", p)
		}
		return false, nil
	}
	_, err := servicioSimple(repo).MarcarPreferencia(context.Background(), NuevaPreferenciaParams{
		EquipoIDs: []string{"pc1", "pc2"}, MateriaNombre: "Tecnología de los Materiales",
		Modalidad: ptrStr("Construcción"), Anio: ptrInt(3), Division: ptrStr("2"),
		Prioridad: 1, CursoExacto: true,
	})
	if !errors.Is(err, ErrAlcancePreferenciaInexistente) || len(repo.preferencias) != 0 {
		t.Fatalf("un alcance inexistente debe rechazar el lote completo: %v, %+v", err, repo.preferencias)
	}
}

func TestMarcarPreferencia_NoContinuaSiNoPuedeVerificarElCatalogo(t *testing.T) {
	repo := nuevoFakeRepo()
	fallo := errors.New("catálogo no disponible")
	repo.validarAlcance = func(*domain.PreferenciaDeEquipo) (bool, error) { return false, fallo }
	_, err := servicioSimple(repo).MarcarPreferencia(context.Background(), NuevaPreferenciaParams{
		EquipoIDs: []string{"pc1", "pc2"}, MateriaNombre: "Matemática", Prioridad: 1,
	})
	if !errors.Is(err, fallo) || len(repo.preferencias) != 0 {
		t.Fatalf("no debe marcar sin poder verificar: %v, %+v", err, repo.preferencias)
	}
}

func TestEditarPreferencia_AlcanceInexistenteConservaLaMarca(t *testing.T) {
	repo := nuevoFakeRepo()
	actual, _ := domain.NuevaPreferencia("p1", "pc1", "Matemática", nil, ptrInt(3), nil, 2, true)
	repo.preferencias[actual.ID] = actual
	repo.validarAlcance = func(*domain.PreferenciaDeEquipo) (bool, error) { return false, nil }
	_, err := servicioSimple(repo).EditarPreferencia(context.Background(), actual.ID, ptrStr("Otra modalidad"), ptrInt(4), nil, 1)
	if !errors.Is(err, ErrAlcancePreferenciaInexistente) || repo.preferencias[actual.ID] != actual {
		t.Fatalf("la edición inválida modificó la marca: %v, %+v", err, repo.preferencias[actual.ID])
	}
}

func TestEditarPreferencia_OmitirCursoExactoConservaSuAlcance(t *testing.T) {
	for _, exacto := range []bool{false, true} {
		repo := nuevoFakeRepo()
		actual, _ := domain.NuevaPreferencia("p1", "pc1", "Matemática", nil, ptrInt(3), nil, 2, exacto)
		repo.preferencias[actual.ID] = actual
		editada, err := servicioSimple(repo).EditarPreferencia(context.Background(), actual.ID, nil, ptrInt(3), nil, 1)
		if err != nil || editada.CursoExacto != exacto {
			t.Fatalf("omitir cursoExacto cambió el alcance (%v): %+v, %v", exacto, editada, err)
		}
	}
}
