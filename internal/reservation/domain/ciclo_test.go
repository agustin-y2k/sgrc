package domain

import (
	"errors"
	"testing"
	"time"
)

func TestReservaEnCiclo_RespetaFechasCivilesYFinExclusivo(t *testing.T) {
	for _, c := range []struct {
		nombre, dia string
		inicio, fin time.Duration
		valido      bool
	}{
		{"primero", "2026-01-01", 0, time.Hour, true},
		{"ultimo", "2026-12-31", 8 * time.Hour, 9 * time.Hour, true},
		{"hasta medianoche", "2026-12-31", 23 * time.Hour, 0, true},
		{"ocupa enero", "2026-12-31", 23 * time.Hour, time.Hour, false},
		{"siguiente año", "2027-01-01", 8 * time.Hour, 9 * time.Hour, false},
		{"año previo", "2025-12-31", 23 * time.Hour, 0, false},
		{"nocturna dentro del año", "2026-12-30", 23 * time.Hour, time.Hour, true},
	} {
		for _, desfase := range []int{-3, 14} {
			t.Run(c.nombre+time.FixedZone("local", desfase*3600).String(), func(t *testing.T) {
				fecha, err := time.ParseInLocation("2006-01-02", c.dia, time.FixedZone("local", desfase*3600))
				if err != nil {
					t.Fatal(err)
				}
				err = ValidarReservaEnCiclo(2026, fecha, c.inicio, c.fin)
				if (err == nil) != c.valido || (err != nil && !errors.Is(err, ErrReservaFueraDelCiclo)) {
					t.Fatalf("válido=%v error=%v", c.valido, err)
				}
			})
		}
	}
}
