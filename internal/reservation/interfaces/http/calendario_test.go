package http

import (
	"encoding/json"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTP_CalendarioCompartido_PermisosYDatosMinimos(t *testing.T) {
	repo := nuevoFakeRepo()
	propio, otro := "docente-1", "docente-2"
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	repo.reservas["r1"] = &domain.Reserva{ID: "r1", EquipoID: "pc1", CreadoPor: &propio, Fecha: fecha, HoraInicio: 8 * time.Hour, HoraFin: 9 * time.Hour, Estado: domain.ReservaConfirmada, Tipo: domain.TipoNormal}
	repo.reservas["r2"] = &domain.Reserva{ID: "r2", EquipoID: "pc2", CreadoPor: &otro, Fecha: fecha, HoraInicio: 9 * time.Hour, HoraFin: 10 * time.Hour, Estado: domain.ReservaConfirmada, Tipo: domain.TipoNormal, MotivoCancelacion: ptrStringCalendario("nota privada")}
	app := nuevaAppDeTest(repo)
	for _, ambito := range []string{"mias", "todas"} {
		req := httptest.NewRequest("GET", "/api/calendario/reservas?desde=2026-03-09&hasta=2026-03-15&ambito="+ambito, nil)
		req.Header.Set("Authorization", "Bearer "+tokenPara(propio, "DOCENTE"))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		cuerpo, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("HTTP %d: %s", resp.StatusCode, cuerpo)
		}
		var resultado struct {
			Data []reservaCalendarioResponse `json:"data"`
		}
		if err := json.Unmarshal(cuerpo, &resultado); err != nil {
			t.Fatal(err)
		}
		esperado := 1
		if ambito == "todas" {
			esperado = 2
		}
		if len(resultado.Data) != esperado {
			t.Fatalf("%s: esperaba %d, obtuvo %d", ambito, esperado, len(resultado.Data))
		}
		if strings.Contains(string(cuerpo), "nota privada") || strings.Contains(string(cuerpo), "motivoCancelacion") {
			t.Fatalf("se expusieron datos de gestión: %s", cuerpo)
		}
	}
	req := httptest.NewRequest("GET", "/api/reservas?creadoPor="+otro, nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara(propio, "DOCENTE"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if repo.filtroReservas.CreadoPor == nil || *repo.filtroReservas.CreadoPor != propio {
		t.Fatal("el calendario amplió los permisos del listado privado")
	}
}
func ptrStringCalendario(s string) *string { return &s }

func TestHTTP_CalendarioCompartido_ValidaAutenticacionYRango(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())
	for _, tc := range []struct {
		query  string
		token  bool
		estado int
	}{
		{"desde=2026-03-01&hasta=2026-03-07", false, 401},
		{"desde=2026-03-01&hasta=2026-04-12", true, 400},
		{"desde=2026-03-08&hasta=2026-03-01", true, 400},
		{"desde=2026-03-01", true, 400},
		{"desde=2026-03-01&hasta=2026-03-07&ambito=admin", true, 400},
	} {
		req := httptest.NewRequest("GET", "/api/calendario/reservas?"+tc.query, nil)
		if tc.token {
			req.Header.Set("Authorization", "Bearer "+tokenPara("docente", "DOCENTE"))
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.estado {
			t.Fatalf("%s: esperaba %d, obtuvo %d", tc.query, tc.estado, resp.StatusCode)
		}
	}
}
