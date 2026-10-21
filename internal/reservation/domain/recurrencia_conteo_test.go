package domain

import (
	"testing"
	"time"
)

func TestCantidadOcurrencias_CoincideConCalendarioInclusive(t *testing.T) {
	for _, zona := range []string{"UTC", "America/New_York"} {
		loc, err := time.LoadLocation(zona)
		if err != nil {
			t.Fatal(err)
		}
		// Incluye año bisiesto, cambios de mes y las semanas de 23/25 horas.
		for _, inicio := range []time.Time{
			time.Date(2028, 2, 25, 0, 0, 0, 0, loc),
			time.Date(2026, 3, 1, 0, 0, 0, 0, loc),
			time.Date(2026, 10, 25, 0, 0, 0, 0, loc),
			time.Date(2026, 12, 28, 0, 0, 0, 0, loc),
		} {
			for dia := range diaSemanaAGoWeekday {
				for dias := 0; dias <= 320; dias++ {
					r, err := NuevaReglaRecurrencia("r", "m", "u", dia, 8*time.Hour, 9*time.Hour, inicio, inicio.AddDate(0, 0, dias))
					if err != nil {
						t.Fatal(err)
					}
					if n, calendario := r.CantidadOcurrencias(), len(r.GenerarFechas()); n != calendario {
						t.Fatalf("%s %s %s +%d: contó %d fechas, calendario=%d", zona, inicio, dia, dias, n, calendario)
					}
				}
			}
		}
	}
}

func TestCantidadOcurrencias_RangoDeMilesDeAnios(t *testing.T) {
	r, err := NuevaReglaRecurrencia("r", "m", "u", Lunes, 8*time.Hour, 9*time.Hour,
		fecha(1, time.January, 1), fecha(9999, time.December, 31))
	if err != nil {
		t.Fatal(err)
	}
	if n := r.CantidadOcurrencias(); n != 521723 {
		t.Fatalf("conteo de lunes incorrecto: %d", n)
	}
}
