package http

import (
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func TestHTTP_EntregaRechazaEquiposAusentesODesactualizados(t *testing.T) {
	for _, tc := range []struct {
		equipo string
		estado int
	}{{"", 400}, {"pc-anterior", 409}} {
		repo := nuevoFakeRepo()
		r, err := domain.NuevaReservaNormal("res1", "grupo1", "pc1", "materia1", "Docente", nil, time.Now(), 8*time.Hour, 9*time.Hour, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		repo.reservas[r.ID] = r
		app := nuevaAppDeTest(repo)
		estado, cuerpo := pedirPrestamos(t, app, "POST", "/api/prestamos/por-reserva", entregarPorReservaRequest{ReservaIDs: []string{r.ID}, EquiposEsperados: map[string]string{r.ID: tc.equipo}}, "ADMIN")
		if estado != tc.estado || len(repo.prestamos) != 0 {
			t.Fatalf("estado=%d cuerpo=%s préstamos=%d", estado, cuerpo, len(repo.prestamos))
		}
	}
}
