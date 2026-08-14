package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// ── Marcas de preferencia de materia (RF-03.21) ────────────────────────

func TestHTTP_MarcarPreferencia_EnVariosEquipos_201(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/preferencias", jsonBody(marcarPreferenciaRequest{
		EquipoIDs: []string{"e1", "e2"}, MateriaNombre: "Dibujo Técnico",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}

	var body altaDePreferenciasResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Creadas) != 2 {
		t.Fatalf("esperaba 2 marcas, obtuve %d", len(body.Creadas))
	}
	// Sin prioridad explícita queda en la más fuerte: es lo que espera quien
	// marca una máquina para una materia y no piensa en escalones.
	if body.Creadas[0].Prioridad != 1 {
		t.Errorf("prioridad por defecto = %d, esperaba 1", body.Creadas[0].Prioridad)
	}
	if body.Creadas[0].Alcance != "Dibujo Técnico" {
		t.Errorf("alcance = %q", body.Creadas[0].Alcance)
	}
}

func TestHTTP_MarcarPreferencia_ConAnioYDivision_ArmaElAlcance(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	tres, be := 3, "B"
	req := httptest.NewRequest("POST", "/api/inventory/preferencias", jsonBody(marcarPreferenciaRequest{
		EquipoIDs: []string{"e1"}, MateriaNombre: "Matemática", Anio: &tres, Division: &be,
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	var body altaDePreferenciasResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Creadas) != 1 || body.Creadas[0].Alcance != "Matemática de 3°B" {
		t.Fatalf("alcance inesperado: %+v", body.Creadas)
	}
}

// Una división sin año no significa nada y tiene que salir como 400 con
// explicación, no como el 500 de un CHECK de la base.
func TestHTTP_MarcarPreferencia_DivisionSinAnio_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	be := "B"
	req := httptest.NewRequest("POST", "/api/inventory/preferencias", jsonBody(marcarPreferenciaRequest{
		EquipoIDs: []string{"e1"}, MateriaNombre: "Matemática", Division: &be,
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_MarcarPreferencia_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/preferencias", jsonBody(marcarPreferenciaRequest{
		EquipoIDs: []string{"e1"}, MateriaNombre: "Matemática",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

// Leer las marcas SÍ lo puede hacer un docente: son la explicación del orden
// que ya ve al reservar.
func TestHTTP_ListarPreferenciasDeEquipo_ComoDocente_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	p, _ := domain.NuevaPreferencia("p1", "e1", "Matemática", nil, nil, 1)
	repo.preferencias["p1"] = p
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/inventory/equipos/e1/preferencias", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body struct {
		Data []preferenciaResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].MateriaNombre != "Matemática" {
		t.Errorf("respuesta inesperada: %+v", body.Data)
	}
}

func TestHTTP_EditarPreferencia_CambiaElAlcance(t *testing.T) {
	repo := nuevoFakeRepo()
	p, _ := domain.NuevaPreferencia("p1", "e1", "Matemática", nil, nil, 1)
	repo.preferencias["p1"] = p
	app := nuevaAppDeTest(repo)

	cinco, a := 5, "A"
	req := httptest.NewRequest("PATCH", "/api/inventory/preferencias/p1", jsonBody(editarPreferenciaRequest{
		Anio: &cinco, Division: &a,
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body preferenciaResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Alcance != "Matemática de 5°A" {
		t.Errorf("alcance = %q", body.Alcance)
	}
}

func TestHTTP_EditarPreferencia_NoExiste_404(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("PATCH", "/api/inventory/preferencias/no-existe", jsonBody(editarPreferenciaRequest{}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_BorrarPreferencia_204(t *testing.T) {
	repo := nuevoFakeRepo()
	p, _ := domain.NuevaPreferencia("p1", "e1", "Matemática", nil, nil, 1)
	repo.preferencias["p1"] = p
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("DELETE", "/api/inventory/preferencias/p1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("esperaba 204, obtuve %d", resp.StatusCode)
	}
	if len(repo.preferencias) != 0 {
		t.Error("la marca tendría que haberse borrado")
	}
}

func TestHTTP_ListarMateriasEnUso_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.nombresDeMateria = []string{"Dibujo Técnico", "Matemática"}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/inventory/materias-en-uso", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	var body struct {
		Data []string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 {
		t.Errorf("esperaba 2 nombres, obtuve %v", body.Data)
	}
}
