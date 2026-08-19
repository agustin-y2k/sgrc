package application

import (
	"context"
	"errors"
	"testing"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

func ptrInt(n int) *int       { return &n }
func ptrStr(s string) *string { return &s }

func TestMarcarPreferencia_MarcaTodoElLote(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioSimple(repo)

	resultado, err := svc.MarcarPreferencia(context.Background(), NuevaPreferenciaParams{
		EquipoIDs:     []string{"e1", "e2", "e3"},
		MateriaNombre: "Dibujo Técnico",
		Prioridad:     1,
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Creadas) != 3 {
		t.Errorf("esperaba 3 marcas, obtuve %d", len(resultado.Creadas))
	}
	if len(resultado.EquiposQueYaTeni) != 0 {
		t.Errorf("ninguna estaba marcada de antes: %v", resultado.EquiposQueYaTeni)
	}
}

// Marcar un carro entero cuando alguna máquina ya estaba marcada no es un
// error del lote: se informa y se sigue.
func TestMarcarPreferencia_LasYaMarcadasNoCortanElLote(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioSimple(repo)
	ctx := context.Background()

	if _, err := svc.MarcarPreferencia(ctx, NuevaPreferenciaParams{
		EquipoIDs: []string{"e2"}, MateriaNombre: "Dibujo Técnico", Prioridad: 1,
	}); err != nil {
		t.Fatal(err)
	}

	resultado, err := svc.MarcarPreferencia(ctx, NuevaPreferenciaParams{
		EquipoIDs: []string{"e1", "e2", "e3"}, MateriaNombre: "Dibujo Técnico", Prioridad: 1,
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Creadas) != 2 {
		t.Errorf("esperaba 2 marcas nuevas, obtuve %d", len(resultado.Creadas))
	}
	if len(resultado.EquiposQueYaTeni) != 1 || resultado.EquiposQueYaTeni[0] != "e2" {
		t.Errorf("esperaba que e2 saliera como ya marcado, obtuve %v", resultado.EquiposQueYaTeni)
	}
}

// El mismo equipo puede ser preferente de la misma materia con dos alcances
// distintos: uno general y uno acotado a un curso. No son duplicados.
func TestMarcarPreferencia_MismoEquipoDistintoAlcance_NoEsDuplicado(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioSimple(repo)
	ctx := context.Background()

	if _, err := svc.MarcarPreferencia(ctx, NuevaPreferenciaParams{
		EquipoIDs: []string{"e1"}, MateriaNombre: "Matemática", Prioridad: 1,
	}); err != nil {
		t.Fatal(err)
	}

	resultado, err := svc.MarcarPreferencia(ctx, NuevaPreferenciaParams{
		EquipoIDs: []string{"e1"}, MateriaNombre: "Matemática",
		Anio: ptrInt(3), Division: ptrStr("B"), Prioridad: 2,
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Creadas) != 1 {
		t.Fatalf("el alcance acotado es otra marca, no un duplicado: %+v", resultado)
	}
}

func TestMarcarPreferencia_SinEquipos_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.MarcarPreferencia(context.Background(), NuevaPreferenciaParams{
		MateriaNombre: "Matemática", Prioridad: 1,
	})

	if !errors.Is(err, domain.ErrSinEquiposParaPreferi) {
		t.Fatalf("esperaba ErrSinEquiposParaPreferi, obtuve %v", err)
	}
}

// La materia y el alcance son los mismos para todo el lote: si no validan,
// no validan para ninguno y no se crea nada.
func TestMarcarPreferencia_MateriaInvalida_NoCreaNada(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioSimple(repo)

	_, err := svc.MarcarPreferencia(context.Background(), NuevaPreferenciaParams{
		EquipoIDs: []string{"e1", "e2"}, MateriaNombre: "   ", Prioridad: 1,
	})

	if !errors.Is(err, domain.ErrMateriaPreferidaVacia) {
		t.Fatalf("esperaba ErrMateriaPreferidaVacia, obtuve %v", err)
	}
	if len(repo.preferencias) != 0 {
		t.Errorf("no tendría que haber quedado ninguna marca, quedaron %d", len(repo.preferencias))
	}
}

func TestEditarPreferencia_CambiaElAlcanceYLaPrioridad(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioSimple(repo)
	ctx := context.Background()

	creadas, err := svc.MarcarPreferencia(ctx, NuevaPreferenciaParams{
		EquipoIDs: []string{"e1"}, MateriaNombre: "Matemática", Prioridad: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	original := creadas.Creadas[0]

	editada, err := svc.EditarPreferencia(ctx, original.ID, ptrInt(5), ptrStr("A"), 3)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if editada.Alcance() != "Matemática de 5°A" {
		t.Errorf("alcance = %q", editada.Alcance())
	}
	if editada.Prioridad != 3 {
		t.Errorf("prioridad = %d, esperaba 3", editada.Prioridad)
	}
	// La materia no se edita: apuntar a otra es otra marca.
	if editada.MateriaNombre != "Matemática" {
		t.Errorf("la materia no tenía que cambiar, quedó %q", editada.MateriaNombre)
	}
}

// El alcance editado pasa por las mismas validaciones que el nuevo.
func TestEditarPreferencia_AlcanceInvalido_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioSimple(repo)
	ctx := context.Background()

	creadas, _ := svc.MarcarPreferencia(ctx, NuevaPreferenciaParams{
		EquipoIDs: []string{"e1"}, MateriaNombre: "Matemática", Prioridad: 1,
	})

	_, err := svc.EditarPreferencia(ctx, creadas.Creadas[0].ID, nil, ptrStr("B"), 1)

	if !errors.Is(err, domain.ErrDivisionSinAnio) {
		t.Fatalf("esperaba ErrDivisionSinAnio, obtuve %v", err)
	}
}

func TestEditarPreferencia_NoExiste_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.EditarPreferencia(context.Background(), "no-existe", nil, nil, 1)

	if !errors.Is(err, domain.ErrPreferenciaNoEncontr) {
		t.Fatalf("esperaba ErrPreferenciaNoEncontr, obtuve %v", err)
	}
}

func TestBorrarPreferencia_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioSimple(repo)
	ctx := context.Background()

	creadas, _ := svc.MarcarPreferencia(ctx, NuevaPreferenciaParams{
		EquipoIDs: []string{"e1"}, MateriaNombre: "Matemática", Prioridad: 1,
	})

	if err := svc.BorrarPreferencia(ctx, creadas.Creadas[0].ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(repo.preferencias) != 0 {
		t.Error("la marca tendría que haberse borrado")
	}
}
