package http

import (
	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTP_RenumerarEquipo_SoloAdmin(t *testing.T) {
	repo := nuevoFakeRepo()
	pc, _ := domain.NuevoEquipoDeCarro("pc", "carro", 4, "SERIE-FISICA", false, time.Now())
	repo.equipos[pc.ID] = pc
	app := nuevaAppDeTest(repo)
	for _, rol := range []string{"DOCENTE", "ADMIN"} {
		req := httptest.NewRequest("PATCH", "/api/equipos/pc/rotulo", jsonBody(map[string]any{"identificadorActual": 4, "identificador": 18, "motivo": "Cambio de rótulo"}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenPara("persona", rol))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		esperado := 403
		if rol == "ADMIN" {
			esperado = 204
		}
		if resp.StatusCode != esperado {
			t.Fatalf("%s: esperaba %d, obtuvo %d", rol, esperado, resp.StatusCode)
		}
	}
	if repo.equipos[pc.ID].NumeroSerie != "SERIE-FISICA" {
		t.Fatal("cambió la serie")
	}
}
