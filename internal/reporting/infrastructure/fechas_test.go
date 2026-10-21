package infrastructure

import (
	"testing"
	"time"
)

// Un día civil puede durar 23 o 25 horas. El límite superior debe ser la
// medianoche siguiente de la escuela, sin sumar una duración fija de 24h.
func TestFiltroInstantes_RespetaDiasConCambioDeHora(t *testing.T) {
	zona, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	repo := &PostgresRepo{zona: zona}
	for _, caso := range []struct {
		dia   time.Time
		horas time.Duration
	}{
		{time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC), 23 * time.Hour},
		{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), 25 * time.Hour},
	} {
		_, args := repo.filtroInstantes("fecha", &caso.dia, &caso.dia, nil)
		inicio, fin := args[0].(time.Time), args[1].(time.Time)
		if fin.Sub(inicio) != caso.horas || inicio.Hour() != 0 || fin.Hour() != 0 || inicio.Location() != zona || fin.Day() == inicio.Day() {
			t.Fatalf("día mal delimitado: %s a %s", inicio, fin)
		}
	}
}
