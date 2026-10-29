package domain

import (
	"testing"
	"time"
)

func TestReserva_PuedeEntregarseEn_VigenciaYCalendarioLocal(t *testing.T) {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatal(err)
	}
	for _, zona := range []*time.Location{time.UTC, loc} {
		for _, estado := range []EstadoReserva{ReservaConfirmada, ReservaNoRetirada, ReservaCancelada, ReservaFinalizada} {
			for _, tc := range []struct {
				nombre            string
				dia, hora, minuto int
				nocturna, permite bool
			}{
				{"inicio-del-dia", 2, 0, 0, false, true},
				{"antes-de-clase", 2, 7, 30, false, true},
				{"inicio", 2, 8, 0, false, true},
				{"en-curso", 2, 8, 30, false, true},
				{"fin-exclusivo", 2, 9, 0, false, false},
				{"vencida", 2, 12, 0, false, false},
				{"dia-futuro", 1, 23, 59, false, false},
				{"dia-anterior", 3, 0, 0, false, false},
				{"nocturna-de-ayer", 3, 0, 30, true, true},
				{"fin-nocturno", 3, 1, 0, true, false},
			} {
				t.Run(zona.String()+"/"+string(estado)+"/"+tc.nombre, func(t *testing.T) {
					r := &Reserva{Estado: estado, Fecha: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), HoraInicio: 8 * time.Hour, HoraFin: 9 * time.Hour}
					if tc.nocturna {
						r.HoraInicio, r.HoraFin = 22*time.Hour, time.Hour
					}
					ahora := time.Date(2026, 3, tc.dia, tc.hora, tc.minuto, 0, 0, zona)
					esperado := tc.permite && (estado == ReservaConfirmada || estado == ReservaNoRetirada)
					if r.PuedeEntregarseEn(ahora) != esperado {
						t.Fatalf("fecha=%v estado=%s ahora=%v permite=%v", r.Fecha, estado, ahora, esperado)
					}
				})
			}
		}
	}
}
