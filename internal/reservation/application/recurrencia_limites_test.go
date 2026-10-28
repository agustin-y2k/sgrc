package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func TestRecurrencia_LimiteExactoYSolicitudExtremaSinEscrituras(t *testing.T) {
	inicio := fecha(2026, time.January, 5)
	for _, caso := range []struct {
		nombre string
		fin    time.Time
		err    error
	}{
		{"45-clases", inicio.AddDate(0, 0, 7*(MaxOcurrenciasRecurrencia-1)+6), nil},
		{"46-clases", inicio.AddDate(0, 0, 7*MaxOcurrenciasRecurrencia), ErrDemasiadasOcurrencias},
		{"hasta-9999", fecha(9999, time.December, 31), ErrDemasiadasOcurrencias},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			repo := nuevoFakeRepo()
			svc := nuevoServicioDeTest(repo)
			svc.ahora = func() time.Time { return fecha(2026, time.January, 1) }
			res, err := svc.CrearReservaRecurrente(context.Background(), "materia1", "docente1", false,
				domain.Lunes, 8*time.Hour, 9*time.Hour, inicio, caso.fin, []string{"pc1"}, nil)
			if !errors.Is(err, caso.err) {
				t.Fatalf("esperado=%v obtenido=%v", caso.err, err)
			}
			if caso.err == nil {
				if len(res.Grupos) != MaxOcurrenciasRecurrencia || len(repo.reglas) != 1 {
					t.Fatalf("no materializó el límite exacto: %+v", res)
				}
			} else if res != nil || len(repo.reglas) != 0 || len(repo.grupos) != 0 || len(repo.reservas) != 0 {
				t.Fatal("una serie rechazada dejó escrituras parciales")
			}
		})
	}
}
