package application

import (
	"context"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

func TestBorrarLicencia_UltimaPendienteCierraElAviso(t *testing.T) {
	repo := repoConCarroYEquipos(2)
	licenciaCargada(t, repo, "lic-1", "equipo-1", dia(2025, time.December, 30))
	licenciaCargada(t, repo, "lic-2", "equipo-2", dia(2025, time.December, 31))
	svc, bus := servicioConBus(repo)
	var pendientes []int
	bus.Subscribe("licencia.pendientes", func(e eventbus.Evento) {
		pendientes = append(pendientes, e.Payload.(eventbus.PendientesDeLicencia).Pendientes)
	})
	for _, id := range []string{"lic-1", "lic-2"} {
		if err := svc.BorrarLicencia(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if len(pendientes) != 2 || pendientes[0] != 1 || pendientes[1] != 0 {
		t.Fatalf("el borrado dejó un aviso sin causa: %v", pendientes)
	}
}
