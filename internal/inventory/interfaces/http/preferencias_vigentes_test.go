package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/inventory/application"
)

func TestHTTP_PreferenciasSinVigencia_DistingueArchivoYSoloAdmin(t *testing.T) {
	for _, historica := range []bool{false, true} {
		repo := nuevoFakeRepo()
		repo.preferenciasHuerfanas = []*application.PreferenciaHuerfana{{ID: "marca", MateriaNombre: "Matemática", EquipoEtiqueta: "PC 4", Prioridad: 1, CoincideConArchivadas: historica}}
		app := nuevaAppDeTest(repo)
		for _, tc := range []struct {
			rol    string
			estado int
		}{{"", 401}, {"DOCENTE", 403}, {"ADMIN", 200}} {
			req := httptest.NewRequest("GET", "/api/preferencias/huerfanas", nil)
			if tc.rol != "" {
				req.Header.Set("Authorization", "Bearer "+tokenPara("prueba-vigencia", tc.rol))
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.estado {
				t.Fatalf("rol=%s estado=%d esperado=%d", tc.rol, resp.StatusCode, tc.estado)
			}
			if tc.estado == 200 {
				var cuerpo struct {
					Data []map[string]any `json:"data"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
					t.Fatal(err)
				}
				if len(cuerpo.Data) != 1 || cuerpo.Data[0]["coincideConArchivadas"] != historica {
					t.Fatalf("indicador histórico incorrecto o ausente: %+v", cuerpo)
				}
			}
			resp.Body.Close()
		}
	}
}
