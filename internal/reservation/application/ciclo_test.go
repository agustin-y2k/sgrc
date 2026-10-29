package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func TestReservaFueraDelAnio_NoEscribeNiDesplaza(t *testing.T) {
	for _, admin := range []bool{true, false} {
		for _, tipo := range []string{"puntual", "serie", "previsualizacion", "disponibles", "ocupados"} {
			t.Run(tipo+map[bool]string{true: "admin", false: "docente"}[admin], func(t *testing.T) {
				repo := nuevoFakeRepo()
				svc := nuevoServicioDeTest(repo)
				fechaInicio := fecha(2026, 12, 28)
				fechaFin := fecha(2027, 1, 4)
				var err error
				switch tipo {
				case "puntual":
					_, _, _, err = svc.CrearReserva(context.Background(), "materia", "docente", admin, fechaFin, 8*time.Hour, 9*time.Hour, []string{"pc1"}, []string{"pc1"})
				case "serie":
					_, err = svc.CrearReservaRecurrente(context.Background(), "materia", "docente", admin, domain.Lunes, 8*time.Hour, 9*time.Hour, fechaInicio, fechaFin, []string{"pc1"}, []string{"pc1"})
				case "previsualizacion":
					_, err = svc.PrevisualizarReservaRecurrente(context.Background(), "materia", "docente", admin, domain.Lunes, 8*time.Hour, 9*time.Hour, fechaInicio, fechaFin, []string{"pc1"})
				case "disponibles":
					_, err = svc.ListarEquiposDisponiblesEn(context.Background(), fechaFin, 8*time.Hour, 9*time.Hour, "materia")
				case "ocupados":
					_, err = svc.ListarEquiposOcupadosEn(context.Background(), fechaFin, 8*time.Hour, 9*time.Hour, "docente", "materia")
				}
				if !errors.Is(err, domain.ErrReservaFueraDelCiclo) {
					t.Fatalf("error: %v", err)
				}
				if len(repo.reservas) != 0 || len(repo.grupos) != 0 || len(repo.reglas) != 0 {
					t.Fatal("la solicitud fuera de año dejó escrituras")
				}
			})
		}
	}
}

func TestRecurrencia_FinDelCicloValidaLaUltimaClaseReal(t *testing.T) {
	for _, tc := range []struct {
		dia    domain.DiaSemana
		fin    time.Time
		valido bool
	}{
		{domain.Miercoles, fecha(2026, 12, 31), true},
		{domain.Jueves, fecha(2026, 12, 31), false},
		{domain.Miercoles, fecha(2027, 1, 1), false},
	} {
		repo := nuevoFakeRepo()
		svc := nuevoServicioDeTest(repo)
		_, err := svc.CrearReservaRecurrente(context.Background(), "materia", "docente", false, tc.dia, 23*time.Hour, time.Hour, fecha(2026, 12, 23), tc.fin, []string{"pc1"}, nil)
		if (err == nil) != tc.valido {
			t.Fatalf("día=%s fin=%s válido=%v err=%v", tc.dia, tc.fin, tc.valido, err)
		}
	}
}
