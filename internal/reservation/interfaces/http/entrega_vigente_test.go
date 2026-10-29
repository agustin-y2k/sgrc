package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func TestHTTP_EntregaValidaVigenciaAunqueElClienteEnvieUnIDHistorico(t *testing.T) {
	for _, tc := range []struct {
		nombre   string
		estado   domain.EstadoReserva
		mes      time.Month
		dia, fin int
		permite  bool
	}{
		{"histórica", domain.ReservaFinalizada, time.February, 2, 13, false},
		{"vencida", domain.ReservaConfirmada, time.March, 2, 9, false},
		{"futura", domain.ReservaConfirmada, time.March, 3, 13, false},
		{"no retirada vigente", domain.ReservaNoRetirada, time.March, 2, 13, true},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			repo := nuevoFakeRepo()
			reservaEnRepo(t, repo, "res1", "pc1", "docente1")
			r := repo.reservas["res1"]
			if r.ReservaGrupoID != nil {
				repo.grupos[*r.ReservaGrupoID] = &domain.ReservaGrupo{ID: *r.ReservaGrupoID, Estado: domain.GrupoConfirmada}
			}
			r.Estado = tc.estado
			r.Fecha = time.Date(2026, tc.mes, tc.dia, 0, 0, 0, 0, time.UTC)
			r.HoraInicio, r.HoraFin = 8*time.Hour, time.Duration(tc.fin)*time.Hour
			app := nuevaAppDeTest(repo)
			codigo, cuerpo := pedirPrestamos(t, app, "POST", "/api/prestamos/por-reserva", entregarPorReservaRequest{ReservaIDs: []string{"res1"}, EquiposEsperados: map[string]string{"res1": "pc1"}, LiberarNoEntregadas: true}, "ADMIN")
			if codigo != 200 {
				t.Fatalf("estado=%d cuerpo=%s", codigo, cuerpo)
			}
			var resultado resultadoEntregaResponse
			if err := json.Unmarshal(cuerpo, &resultado); err != nil {
				t.Fatal(err)
			}
			if tc.permite {
				if len(resultado.Entregadas) != 1 {
					t.Fatalf("no retirado vigente: %s", cuerpo)
				}
			} else if len(repo.prestamos) != 0 || len(resultado.Entregadas) != 0 || len(resultado.NoEntregadas) != 1 || resultado.NoEntregadas[0].Razon != "RESERVA_NO_VIGENTE" {
				t.Fatalf("entrega indebida: %s", cuerpo)
			}
		})
	}
}
