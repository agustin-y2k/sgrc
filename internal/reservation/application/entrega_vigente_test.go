package application

import (
	"context"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func TestEntregaVigente_RechazosNoCreanPrestamosNiLiberanOtrasReservas(t *testing.T) {
	for _, tc := range []struct {
		nombre      string
		estado      domain.EstadoReserva
		mes         time.Month
		dia         int
		inicio, fin time.Duration
		permite     bool
	}{
		{"finalizada-del-mes-anterior", domain.ReservaFinalizada, time.February, 2, 12 * time.Hour, 13 * time.Hour, false},
		{"confirmada-vencida", domain.ReservaConfirmada, time.March, 2, 8 * time.Hour, 9 * time.Hour, false},
		{"no-retirada-vencida", domain.ReservaNoRetirada, time.March, 2, 8 * time.Hour, 9 * time.Hour, false},
		{"dia-futuro", domain.ReservaConfirmada, time.March, 3, 12 * time.Hour, 13 * time.Hour, false},
		{"no-retirada-vigente", domain.ReservaNoRetirada, time.March, 2, 12 * time.Hour, 13 * time.Hour, true},
		{"confirmada-por-empezar-hoy", domain.ReservaConfirmada, time.March, 2, 15 * time.Hour, 16 * time.Hour, true},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			repo := nuevoFakeRepo()
			svc := loteReservado(t, repo)
			r := repo.reservas["a"]
			r.Estado = tc.estado
			r.Fecha = fecha(2026, tc.mes, tc.dia)
			r.HoraInicio, r.HoraFin = tc.inicio, tc.fin
			resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"a"}, EquiposEsperados: map[string]string{"a": "pc-a"}, LiberarNoEntregadas: true})
			if err != nil {
				t.Fatal(err)
			}
			if tc.permite {
				if len(resultado.Entregadas) != 1 {
					t.Fatalf("no entregó la reserva vigente: %+v", resultado)
				}
				p := resultado.Entregadas[0]
				if p.DevolucionEstimada == nil || !p.DevolucionEstimada.After(p.EntregadoEn) {
					t.Fatal("creó una entrega vencida")
				}
			} else if len(repo.prestamos) != 0 || len(resultado.Liberadas) != 0 || len(resultado.NoEntregadas) != 1 || resultado.NoEntregadas[0].Razon != NoEntregadaReservaNoVigente {
				t.Fatalf("rechazo con efectos laterales: %+v", resultado)
			}
		})
	}
}

func TestEntregaVigente_LoteMixtoConservaLaReservaVencida(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := loteReservado(t, repo)
	antigua := repo.reservas["a"]
	antigua.Fecha = fecha(2026, time.February, 2)
	otroGrupo := "grupo-anterior"
	antigua.ReservaGrupoID = &otroGrupo
	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"a", "b"}, EquiposEsperados: map[string]string{"a": "pc-a", "b": "pc-b"}, LiberarNoEntregadas: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(resultado.Entregadas) != 1 || resultado.Entregadas[0].EquipoID != "pc-b" || len(resultado.NoEntregadas) != 1 || len(resultado.Liberadas) != 1 || resultado.Liberadas[0] != "c" {
		t.Fatalf("lote incorrecto: %+v", resultado)
	}
	if antigua.Estado != domain.ReservaConfirmada {
		t.Fatal("modificó la reserva vencida")
	}
}

type reservasLeidasConDemora struct {
	Repo
	ahora *time.Time
}

func (r reservasLeidasConDemora) EnTransaccion(ctx context.Context, fn func(Repo) error) error {
	return r.Repo.EnTransaccion(ctx, func(Repo) error { return fn(r) })
}
func (r reservasLeidasConDemora) BuscarReservasParaEntregar(ctx context.Context, ids []string) (map[string]*domain.Reserva, error) {
	reservas, err := r.Repo.BuscarReservasParaEntregar(ctx, ids)
	*r.ahora = mediodiaDeTest.Add(time.Hour)
	return reservas, err
}
func TestEntregaVigente_ReleeElRelojDespuesDeEsperarLosBloqueos(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(t, repo, "a", "pc-a")
	ahora := mediodiaDeTest
	svc := nuevoServicioDeTest(reservasLeidasConDemora{repo, &ahora})
	svc.ahora = func() time.Time { return ahora }
	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"a"}, EquiposEsperados: map[string]string{"a": "pc-a"}})
	if err != nil || len(repo.prestamos) != 0 || len(resultado.NoEntregadas) != 1 {
		t.Fatalf("se usó el reloj anterior a la espera: %+v %v", resultado, err)
	}
}
