package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func TestEntregaAnticipada_RespetaLasReservasIntermedias(t *testing.T) {
	for _, tc := range []struct {
		nombre      string
		ahora       time.Time
		fechaPrevia time.Time
		inicio, fin time.Duration
		estado      domain.EstadoReserva
		bloqueo     bool
		otroEquipo  bool
		rechaza     bool
	}{
		{"clase-pendiente", mediodiaDeTest, mediodiaDeTest, 13 * time.Hour, 14 * time.Hour, domain.ReservaConfirmada, false, false, true},
		{"clase-en-curso", mediodiaDeTest, mediodiaDeTest, 11 * time.Hour, 13 * time.Hour, domain.ReservaConfirmada, false, false, true},
		{"fin-exclusivo", mediodiaDeTest, mediodiaDeTest, 11 * time.Hour, 12 * time.Hour, domain.ReservaConfirmada, false, false, false},
		{"posterior-al-retiro", mediodiaDeTest, mediodiaDeTest, 16 * time.Hour, 17 * time.Hour, domain.ReservaConfirmada, false, false, false},
		{"cancelada", mediodiaDeTest, mediodiaDeTest, 13 * time.Hour, 14 * time.Hour, domain.ReservaCancelada, false, false, false},
		{"finalizada", mediodiaDeTest, mediodiaDeTest, 13 * time.Hour, 14 * time.Hour, domain.ReservaFinalizada, false, false, false},
		{"liberada", mediodiaDeTest, mediodiaDeTest, 13 * time.Hour, 14 * time.Hour, domain.ReservaNoRetirada, false, false, false},
		{"bloqueo", mediodiaDeTest, mediodiaDeTest, 13 * time.Hour, 14 * time.Hour, domain.ReservaConfirmada, true, false, true},
		{"otra-pc", mediodiaDeTest, mediodiaDeTest, 13 * time.Hour, 14 * time.Hour, domain.ReservaConfirmada, false, true, false},
		{"nocturna-de-ayer", fecha(2026, time.March, 2).Add(30 * time.Minute), fecha(2026, time.March, 1), 22 * time.Hour, time.Hour, domain.ReservaConfirmada, false, false, true},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			repo := nuevoFakeRepo()
			r := reservaDeTest(t, repo, "tarde", "pc1")
			r.HoraInicio, r.HoraFin = 15*time.Hour, 16*time.Hour
			previa := reservaDeTest(t, repo, "previa", "pc1")
			previa.Fecha, previa.HoraInicio, previa.HoraFin, previa.Estado = tc.fechaPrevia, tc.inicio, tc.fin, tc.estado
			grupo := "otro-grupo"
			previa.ReservaGrupoID = &grupo
			if tc.bloqueo {
				previa.Tipo, previa.MotivoBloqueo = domain.TipoBloqueo, "Evaluación"
			}
			if tc.otroEquipo {
				previa.EquipoID = "pc2"
			}
			svc := nuevoServicioDeTest(repo)
			svc.ahora = func() time.Time { return tc.ahora }
			res, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
				ReservaIDs: []string{r.ID}, EquiposEsperados: map[string]string{r.ID: r.EquipoID}, LiberarNoEntregadas: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if tc.rechaza {
				if len(res.Entregadas) != 0 || len(res.NoEntregadas) != 1 || res.NoEntregadas[0].Razon != RazonNoEntregada("RESERVA_INTERMEDIA") || len(res.Liberadas) != 0 || len(repo.prestamos) != 0 {
					t.Fatalf("entrega anticipada con conflicto: %+v", res)
				}
				if !strings.Contains(res.NoEntregadas[0].Detalle, "adelantar") {
					t.Fatalf("no explica el rechazo: %+v", res.NoEntregadas)
				}
			} else if len(res.Entregadas) != 1 || len(res.NoEntregadas) != 0 {
				t.Fatalf("preparación sin conflicto: %+v", res)
			}
		})
	}
}

func TestEntregaAnticipada_ParcialConservaLaPCBloqueada(t *testing.T) {
	for _, seleccionada := range []bool{false, true} {
		t.Run(fmt.Sprintf("rechazada-seleccionada-%v", seleccionada), func(t *testing.T) {
			repo := nuevoFakeRepo()
			for _, id := range []string{"a", "b", "c"} {
				r := reservaDeTest(t, repo, id, "pc-"+id)
				r.HoraInicio, r.HoraFin = 15*time.Hour, 16*time.Hour
			}
			previa := reservaDeTest(t, repo, "previa", "pc-b")
			previa.HoraInicio, previa.HoraFin = 13*time.Hour, 14*time.Hour
			grupo := "grupo-previo"
			previa.ReservaGrupoID = &grupo
			svc := nuevoServicioDeTest(repo)
			ids := []string{"a"}
			if seleccionada {
				ids = append(ids, "b")
			}
			res, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
				ReservaIDs: ids, EquiposEsperados: map[string]string{"a": "pc-a", "b": "pc-b"}, LiberarNoEntregadas: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Entregadas) != 1 || len(res.Liberadas) != 1 || res.Liberadas[0] != "c" || repo.reservas["b"].Estado != domain.ReservaConfirmada || previa.Estado != domain.ReservaConfirmada {
				t.Fatalf("liberó la reserva impedida por otra clase: %+v", res)
			}
		})
	}
}

type fallaLecturaDeIntermedias struct{ Repo }

func (r fallaLecturaDeIntermedias) EnTransaccion(ctx context.Context, fn func(Repo) error) error {
	return r.Repo.EnTransaccion(ctx, func(repo Repo) error { return fn(fallaLecturaDeIntermedias{repo}) })
}
func (r fallaLecturaDeIntermedias) ListarReservasFuturasDeEquipo(context.Context, string, time.Time) ([]*domain.Reserva, error) {
	return nil, errors.New("lectura no disponible")
}

func TestEntregaAnticipada_NoEntregaSiNoPuedeComprobarReservas(t *testing.T) {
	repo := nuevoFakeRepo()
	r := reservaDeTest(t, repo, "tarde", "pc1")
	r.HoraInicio, r.HoraFin = 15*time.Hour, 16*time.Hour
	svc := nuevoServicioDeTest(repo)
	svc.repo = fallaLecturaDeIntermedias{repo}
	res, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{r.ID}, EquiposEsperados: map[string]string{r.ID: r.EquipoID},
	})
	if err == nil || res != nil || len(repo.prestamos) != 0 {
		t.Fatalf("se entregó sin comprobar disponibilidad: %+v %v", res, err)
	}
}
