package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/reservation/application"
)

func TestHTTP_PrevisualizarSerie_ConsultaAutenticadaSinReservas(t *testing.T) {
	for _, caso := range []struct {
		nombre, dia, desde string
		autenticar         bool
		codigo             int
	}{
		{"correcta", "LUNES", "2026-03-09", true, 200},
		{"sin sesión", "LUNES", "2026-03-09", false, 401},
		{"día inválido", "OTRO", "2026-03-09", true, 400},
		{"fecha inválida", "LUNES", "2026-02-30", true, 400},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			repo := nuevoFakeRepo()
			repo.pcsDisponibles = []application.EquipoDisponible{{EquipoID: "pc1", Etiqueta: "PC 1"}}
			app := nuevaAppDeTest(repo)
			req := httptest.NewRequest("POST", "/api/reservas/recurrentes/previsualizar", jsonBody(crearReservaRecurrenteRequest{MateriaID: "m1", DiaSemana: caso.dia, FechaInicio: caso.desde, FechaFin: "2026-03-23", HoraInicio: "08:00", HoraFin: "09:00", EquipoIDs: []string{"pc1"}}))
			req.Header.Set("Content-Type", "application/json")
			if caso.autenticar {
				req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != caso.codigo {
				t.Fatalf("esperaba %d, obtuvo %d", caso.codigo, resp.StatusCode)
			}
			if caso.codigo == 200 {
				var resultado application.PrevisualizacionSerie
				if err := json.NewDecoder(resp.Body).Decode(&resultado); err != nil {
					t.Fatal(err)
				}
				if !resultado.PuedeCrear || resultado.CantidadClases != 3 {
					t.Fatalf("respuesta incorrecta: %+v", resultado)
				}
				if len(repo.reservas)+len(repo.grupos) != 0 {
					t.Fatal("la consulta creó reservas")
				}
			}
		})
	}
}
