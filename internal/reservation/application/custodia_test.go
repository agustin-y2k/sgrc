package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/shared/paginacion"
)

func TestCustodia_ReservaEntregadaConservaEquipoAunqueSeDevuelva(t *testing.T) {
	for _, devolver := range []bool{false, true} {
		for _, admin := range []bool{false, true} {
			repo := nuevoFakeRepo()
			reservaDeTest(t, repo, "r", "pc1")
			svc := nuevoServicioDeTest(repo)
			ctx := context.Background()
			entrega, err := svc.EntregarPorReserva(ctx, EntregaPorReservaParams{ReservaIDs: []string{"r"}, EquiposEsperados: map[string]string{"r": "pc1"}})
			if err != nil {
				t.Fatal(err)
			}
			if devolver {
				if _, err := svc.RecibirEquipos(ctx, []string{entrega.Entregadas[0].ID}, "", ""); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := svc.CambiarEquipoDeReserva(ctx, "r", "pc9", "docente1", admin, true); !errors.Is(err, ErrReservaConEntrega) {
				t.Fatalf("devuelta=%v admin=%v: %v", devolver, admin, err)
			}
			if repo.reservas["r"].EquipoID != "pc1" {
				t.Fatal("se reescribió la PC entregada")
			}
			filas, _, err := svc.ListarReservas(ctx, FiltroReservas{Pagina: paginacion.PorDefecto()}, "docente1", admin)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range filas {
				if r.Reserva.ID == "r" && !r.TuvoEntrega {
					t.Fatal("la pantalla no recibió la protección")
				}
			}
		}
	}
}

func TestCustodia_RetiroParcialPermiteCambiarLasNoEntregadas(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := loteReservado(t, repo)
	ctx := context.Background()
	if _, err := svc.EntregarPorReserva(ctx, EntregaPorReservaParams{ReservaIDs: []string{"a"}, EquiposEsperados: map[string]string{"a": "pc-a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CambiarEquipoDeReserva(ctx, "b", "pc9", "docente1", false, true); err != nil {
		t.Fatal(err)
	}
	if repo.reservas["a"].EquipoID != "pc-a" || repo.reservas["b"].EquipoID != "pc9" {
		t.Fatal("no conservó el retiro parcial")
	}
}

func TestCustodia_SerieSeValidaCompletaYPermiteEmpezarEnLaProxima(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	ctx := context.Background()
	reservas := serieDeCincoLunes(t, svc, repo)
	svc.ahora = func() time.Time { return reservas[1].Fecha.Add(8 * time.Hour) }
	// La segunda ocurrencia ya tuvo entrega; el lote debe rechazarse antes de mover la primera.
	if _, err := svc.EntregarPorReserva(ctx, EntregaPorReservaParams{ReservaIDs: []string{reservas[1].ID}, EquiposEsperados: map[string]string{reservas[1].ID: "pc1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CambiarEquipoDeReserva(ctx, reservas[0].ID, "pc9", "docente1", false, false); !errors.Is(err, ErrReservaConEntrega) {
		t.Fatal(err)
	}
	for _, r := range reservas {
		if repo.reservas[r.ID].EquipoID != "pc1" {
			t.Fatal("se cambió parcialmente la serie")
		}
	}
	if _, err := svc.CambiarEquipoDeReserva(ctx, reservas[2].ID, "pc9", "docente1", false, false); err != nil {
		t.Fatal(err)
	}
	for i, r := range reservas {
		esperado := "pc1"
		if i >= 2 {
			esperado = "pc9"
		}
		if repo.reservas[r.ID].EquipoID != esperado {
			t.Fatal("alcance incorrecto")
		}
	}
}

func TestCustodia_EntregaExigeLosEquiposConfirmadosDeTodoElLote(t *testing.T) {
	for _, tc := range []struct {
		nombre    string
		esperados map[string]string
		err       error
	}{
		{"ausentes", nil, ErrEquiposEsperados},
		{"incompletos", map[string]string{"a": "pc-a"}, ErrEquiposEsperados},
		{"obsoletos", map[string]string{"a": "pc-a", "b": "pc-original"}, ErrEquipoReservaDesactualizado},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			repo := nuevoFakeRepo()
			svc := loteReservado(t, repo)
			r, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"a", "b"}, EquiposEsperados: tc.esperados, LiberarNoEntregadas: true})
			if !errors.Is(err, tc.err) || r != nil {
				t.Fatalf("%+v %v", r, err)
			}
			if len(repo.prestamos) != 0 {
				t.Fatal("una selección inválida confirmó parte de las entregas")
			}
			for _, r := range repo.reservas {
				if r.Estado != "CONFIRMADA" {
					t.Fatal("se liberó una reserva de un lote inválido")
				}
			}
		})
	}
}
