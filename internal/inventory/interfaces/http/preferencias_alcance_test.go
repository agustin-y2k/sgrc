package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/inventory/application"
	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
	"github.com/gofiber/fiber/v2"
)

func TestHTTP_CatalogoPrioridades_ConservaLaRelacionAcademicaYExigeAdmin(t *testing.T) {
	repo := nuevoFakeRepo()
	tres := 3
	modalidad, division := "Construcción", "2"
	repo.catalogo = []application.MateriaParaPreferencia{{MateriaID: "m1", MateriaNombre: "Tecnología de los Materiales", ContenedorID: "c1", ContenedorNombre: "3°2", Tipo: "CURSO", Modalidad: &modalidad, Anio: &tres, Division: &division}}
	app := nuevaAppDeTest(repo)
	for _, rol := range []string{"ADMIN", "DOCENTE"} {
		req := httptest.NewRequest("GET", "/api/catalogo-prioridades", nil)
		req.Header.Set("Authorization", "Bearer "+tokenPara("u1", rol))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if rol == "DOCENTE" {
			if resp.StatusCode != fiber.StatusForbidden {
				t.Fatalf("docente recibió %d", resp.StatusCode)
			}
			resp.Body.Close()
			continue
		}
		var body struct {
			Data []materiaParaPreferenciaResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != 200 || err != nil || len(body.Data) != 1 || body.Data[0].MateriaID != "m1" || *body.Data[0].Modalidad != "Construcción" || *body.Data[0].Anio != 3 || *body.Data[0].Division != "2" {
			t.Fatalf("catálogo inválido: %+v, %v", body, err)
		}
	}
}

func TestHTTP_AlcanceInexistenteEs400YNoModificaMarcas(t *testing.T) {
	for _, metodo := range []string{"POST", "PATCH"} {
		repo := nuevoFakeRepo()
		p, _ := domain.NuevaPreferencia("p1", "pc1", "Matemática", nil, nil, nil, 2)
		repo.preferencias[p.ID] = p
		repo.validarAlcance = func(*domain.PreferenciaDeEquipo) (bool, error) { return false, nil }
		app := nuevaAppDeTest(repo)
		ruta := "/api/preferencias"
		if metodo == "PATCH" {
			ruta += "/p1"
		}
		req := httptest.NewRequest(metodo, ruta, jsonBody(map[string]any{"equipoIds": []string{"pc1", "pc2"}, "materiaNombre": "Matemática", "anio": 3, "modalidad": "Inexistente", "cursoExacto": true, "prioridad": 1}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusBadRequest || len(repo.preferencias) != 1 || repo.preferencias[p.ID] != p {
			t.Fatalf("%s: status %d, marcas %+v", metodo, resp.StatusCode, repo.preferencias)
		}
	}
}

func TestHTTP_CursoExactoRequiereAnioYDevuelveSuAlcance(t *testing.T) {
	for _, conAnio := range []bool{false, true} {
		repo := nuevoFakeRepo()
		app := nuevaAppDeTest(repo)
		body := map[string]any{"equipoIds": []string{"pc1"}, "materiaNombre": "Matemática", "cursoExacto": true}
		if conAnio {
			body["anio"] = 3
		}
		req := httptest.NewRequest("POST", "/api/preferencias", jsonBody(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if !conAnio {
			resp.Body.Close()
			if resp.StatusCode != 400 {
				t.Fatalf("sin año recibió %d", resp.StatusCode)
			}
			continue
		}
		var resultado altaDePreferenciasResponse
		err = json.NewDecoder(resp.Body).Decode(&resultado)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 201 || len(resultado.Creadas) != 1 || !resultado.Creadas[0].CursoExacto || resultado.Creadas[0].Alcance != "Matemática de 3°, sin modalidad, sin división (curso concreto)" {
			t.Fatalf("resultado: %+v, %v", resultado, err)
		}
	}
}
