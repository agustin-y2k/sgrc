package application

import (
	"context"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func TestGrupoConsistente_EstadosMixtos(t *testing.T) {
	for _, tc := range []struct {
		nombre   string
		estados  []domain.EstadoReserva
		esperado domain.EstadoReservaGrupo
	}{
		{"cancelada-y-no-retirada", []domain.EstadoReserva{domain.ReservaCancelada, domain.ReservaNoRetirada}, domain.GrupoNoRetirado},
		{"finalizada-y-no-retirada", []domain.EstadoReserva{domain.ReservaFinalizada, domain.ReservaNoRetirada}, domain.GrupoFinalizada},
		{"confirmada-y-no-retirada", []domain.EstadoReserva{domain.ReservaConfirmada, domain.ReservaNoRetirada}, domain.GrupoConfirmada},
		{"todas-canceladas", []domain.EstadoReserva{domain.ReservaCancelada, domain.ReservaCancelada}, domain.GrupoCancelada},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			repo := nuevoFakeRepo()
			grupoID := "g1"
			repo.grupos[grupoID] = &domain.ReservaGrupo{ID: grupoID, Estado: domain.GrupoConfirmada}
			if tc.estados[0] == domain.ReservaCancelada {
				repo.grupos[grupoID].Estado = domain.GrupoParcialmenteCancelada
			}
			for i, estado := range tc.estados {
				id := string(rune('a' + i))
				repo.reservas[id] = &domain.Reserva{ID: id, ReservaGrupoID: &grupoID, Estado: estado}
			}
			if err := nuevoServicioDeTest(repo).actualizarEstadoGrupo(context.Background(), repo, grupoID); err != nil {
				t.Fatal(err)
			}
			if repo.grupos[grupoID].Estado != tc.esperado {
				t.Fatalf("esperaba %s, obtuvo %s", tc.esperado, repo.grupos[grupoID].Estado)
			}
		})
	}
}
