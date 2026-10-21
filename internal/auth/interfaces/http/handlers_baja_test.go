package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/auth/application"
	"github.com/agustin-y2k/sgrc/internal/auth/domain"
	"github.com/gofiber/fiber/v2"
)

func TestHTTP_BajaPendientePuedeCompletarseSinReactivar(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Rol: domain.RolDocente, Estado: domain.EstadoBaja, BajaPendiente: true}
	repo.bajas = map[string]*application.ProcesoBaja{"d1": {MateriasPendientes: []string{"materia-a"}}}
	app := nuevaAppDeTest(repo)
	for _, estado := range []string{"BAJA", "BAJA", "APROBADA"} {
		req := httptest.NewRequest("PATCH", "/api/auth/usuarios/d1/estado", jsonBody(cambiarEstadoRequest{Estado: estado}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		esperado := fiber.StatusOK
		if estado == "APROBADA" {
			esperado = fiber.StatusConflict
		}
		if resp.StatusCode != esperado {
			t.Fatalf("%s: esperaba %d, obtuvo %d", estado, esperado, resp.StatusCode)
		}
	}
	if !repo.bajas["d1"].Completada || repo.usuarios["d1"].Estado != domain.EstadoBaja || repo.usuarios["d1"].BajaPendiente {
		t.Fatal("baja incompleta o reactivada")
	}
}

func TestHTTP_BajaPendienteSeListaYBloqueaElBorrado(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Estado: domain.EstadoBaja, BajaPendiente: true}
	repo.bajas = map[string]*application.ProcesoBaja{"d1": {}}
	app := nuevaAppDeTest(repo)
	req := httptest.NewRequest("GET", "/api/auth/usuarios", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var listado struct {
		Data []usuarioResponse `json:"data"`
	}
	err = json.NewDecoder(resp.Body).Decode(&listado)
	resp.Body.Close()
	if err != nil || resp.StatusCode != fiber.StatusOK || len(listado.Data) != 1 || !listado.Data[0].BajaPendiente {
		t.Fatalf("pendiente ausente del listado: %+v %v", listado, err)
	}
	req = httptest.NewRequest("DELETE", "/api/auth/usuarios/d1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict || repo.usuarios["d1"] == nil {
		t.Fatalf("borrado permitido: %d", resp.StatusCode)
	}
}
