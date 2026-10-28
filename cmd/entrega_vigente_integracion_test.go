//go:build integration

package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/google/uuid"
)

func TestEntregaVigente_PostgresConservaLosRechazosYPermiteRetiroTardio(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		nombre, estado, fecha, inicio, fin string
		dia, hora, minuto                  int
		permite                            bool
	}{
		{"finalizada-anterior", "FINALIZADA", "2026-04-02", "08:00", "09:00", 4, 8, 30, false},
		{"finalizada-vigente", "FINALIZADA", "2026-05-04", "08:00", "09:00", 4, 8, 30, false},
		{"vencida-confirmada", "CONFIRMADA", "2026-05-04", "08:00", "09:00", 4, 9, 1, false},
		{"fin-exclusivo", "CONFIRMADA", "2026-05-04", "08:00", "09:00", 4, 9, 0, false},
		{"no-retirada-vencida", "NO_RETIRADA", "2026-05-04", "08:00", "09:00", 4, 9, 1, false},
		{"dia-futuro", "CONFIRMADA", "2026-05-05", "08:00", "09:00", 4, 8, 30, false},
		{"anticipada-hoy", "CONFIRMADA", "2026-05-04", "08:00", "09:00", 4, 7, 30, true},
		{"en-curso", "CONFIRMADA", "2026-05-04", "08:00", "09:00", 4, 8, 30, true},
		{"no-retirada-vigente", "NO_RETIRADA", "2026-05-04", "08:00", "09:00", 4, 8, 30, true},
		{"nocturna-de-ayer", "NO_RETIRADA", "2026-05-03", "22:00", "01:00", 4, 0, 30, true},
		{"nocturna-finalizada", "NO_RETIRADA", "2026-05-03", "22:00", "01:00", 4, 1, 0, false},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			ejecutarCierreSQL(t, pool, `UPDATE reserva SET estado=$2,fecha=$3::date,hora_inicio=$4::time,hora_fin=$5::time WHERE id=$1`, e.reservaID, tc.estado, tc.fecha, tc.inicio, tc.fin)
			var antes string
			if err := pool.QueryRow(ctx, `SELECT to_jsonb(r)::text FROM reserva r WHERE id=$1`, e.reservaID).Scan(&antes); err != nil {
				t.Fatal(err)
			}
			ahora := time.Date(2026, 5, tc.dia, tc.hora, tc.minuto, 0, 0, loc)
			svc := reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), nil, reservationinfra.NewValidadorEquipoPostgres(pool), nil, nil, uuid.NewString, func() time.Time { return ahora }, e.bus)
			resultado, err := svc.EntregarPorReserva(ctx, reservationapp.EntregaPorReservaParams{ReservaIDs: []string{e.reservaID}, EquiposEsperados: map[string]string{e.reservaID: e.d.equipo}, LiberarNoEntregadas: true})
			if err != nil {
				t.Fatal(err)
			}
			var cantidad int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM prestamo`).Scan(&cantidad); err != nil {
				t.Fatal(err)
			}
			if tc.permite {
				if cantidad != 1 || len(resultado.Entregadas) != 1 {
					t.Fatalf("retiro válido rechazado: %+v", resultado)
				}
				var entregado, vence time.Time
				if err := pool.QueryRow(ctx, `SELECT entregado_en,devolucion_estimada FROM prestamo WHERE id=$1`, resultado.Entregadas[0].ID).Scan(&entregado, &vence); err != nil || !vence.After(entregado) {
					t.Fatalf("préstamo vencido al crearse: %v %v %v", entregado, vence, err)
				}
			} else {
				if cantidad != 0 || len(resultado.Entregadas) != 0 || len(resultado.Liberadas) != 0 || len(resultado.NoEntregadas) != 1 || resultado.NoEntregadas[0].Razon != reservationapp.NoEntregadaReservaNoVigente {
					t.Fatalf("rechazo inconsistente: %+v", resultado)
				}
				var despues string
				if err := pool.QueryRow(ctx, `SELECT to_jsonb(r)::text FROM reserva r WHERE id=$1`, e.reservaID).Scan(&despues); err != nil || antes != despues {
					t.Fatal("se modificó la reserva rechazada")
				}
			}
		})
	}
}

func TestEntregaVigente_ElTiempoDeEsperaNoAutorizaEntregasVencidas(t *testing.T) {
	pool := baseVacia(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	e := prepararInstitucion(t, pool)
	var reloj atomic.Int64
	reloj.Store(time.Date(2026, 5, 4, 8, 59, 0, 0, time.UTC).UnixNano())
	svc := reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), nil, reservationinfra.NewValidadorEquipoPostgres(pool), nil, nil, uuid.NewString, func() time.Time { return time.Unix(0, reloj.Load()).UTC() }, e.bus)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `SELECT id FROM reserva WHERE id=$1 FOR UPDATE`, e.reservaID); err != nil {
		t.Fatal(err)
	}
	type entrega struct {
		resultado *reservationapp.ResultadoEntrega
		err       error
	}
	termino := make(chan entrega, 1)
	go func() {
		r, err := svc.EntregarPorReserva(ctx, reservationapp.EntregaPorReservaParams{ReservaIDs: []string{e.reservaID}, EquiposEsperados: map[string]string{e.reservaID: e.d.equipo}})
		termino <- entrega{r, err}
	}()
	limite := time.After(3 * time.Second)
	for {
		var esperando bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query ILIKE '%reserva%' AND pid<>pg_backend_pid())`).Scan(&esperando); err != nil {
			t.Fatal(err)
		}
		if esperando {
			break
		}
		select {
		case <-limite:
			t.Fatal("la entrega no esperó el bloqueo")
		case <-time.After(10 * time.Millisecond):
		}
	}
	reloj.Store(time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC).UnixNano())
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	r := <-termino
	if r.err != nil || len(r.resultado.Entregadas) != 0 || len(r.resultado.NoEntregadas) != 1 || r.resultado.NoEntregadas[0].Razon != reservationapp.NoEntregadaReservaNoVigente {
		t.Fatalf("entrega vencida tras esperar: %+v %v", r.resultado, r.err)
	}
}
