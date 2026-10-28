//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	app "github.com/agustin-y2k/sgrc/internal/reservation/application"
	domain "github.com/agustin-y2k/sgrc/internal/reservation/domain"
	infra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func reservaPosteriorDeEntrega(t *testing.T, pool *pgxpool.Pool, e escenarioInstitucional, equipos []string) []*domain.Reserva {
	t.Helper()
	materia, docente := materiaDePrioridadCustodia(t, pool, e.d.curso, "Clase posterior")
	_, rs, _, err := e.reservas.CrearReserva(context.Background(), materia, docente, false, time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC), 12*time.Hour, 13*time.Hour, equipos, nil)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}
func retirarDeEntregaAnticipada(ctx context.Context, svc *app.Service, rs []*domain.Reserva, parcial bool) (*app.ResultadoEntrega, error) {
	ids, esperados := []string{}, map[string]string{}
	for _, r := range rs {
		ids = append(ids, r.ID)
		esperados[r.ID] = r.EquipoID
	}
	return svc.EntregarPorReserva(ctx, app.EntregaPorReservaParams{ReservaIDs: ids, EquiposEsperados: esperados, LiberarNoEntregadas: parcial})
}

func TestEntregaAnticipada_PostgresRechazaYPermiteAlTerminarLaIntermedia(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatal(err)
	}
	for _, bloqueo := range []bool{false, true} {
		t.Run(fmt.Sprintf("bloqueo-%v", bloqueo), func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			rs := reservaPosteriorDeEntrega(t, pool, e, []string{e.d.equipo})
			if bloqueo {
				ejecutarCierreSQL(t, pool, `UPDATE reserva SET tipo='BLOQUEO',motivo_bloqueo='Uso institucional',materia_id=NULL,reserva_grupo_id=NULL WHERE id=$1`, e.reservaID)
			}
			ahora := time.Date(2026, 5, 4, 6, 0, 0, 0, loc)
			svc := app.NewService(infra.NewPostgresRepo(pool), nil, infra.NewValidadorEquipoPostgres(pool), nil, nil, uuid.NewString, func() time.Time { return ahora }, e.bus)
			antes := huellasBase(t, ctx, pool)
			res, err := retirarDeEntregaAnticipada(ctx, svc, rs, true)
			if err != nil || len(res.Entregadas) != 0 || len(res.NoEntregadas) != 1 || res.NoEntregadas[0].Razon != app.NoEntregadaReservaIntermedia || len(res.Liberadas) != 0 {
				t.Fatalf("retiro que perjudica otra clase: %+v %v", res, err)
			}
			if !reflect.DeepEqual(antes, huellasBase(t, ctx, pool)) {
				t.Fatal("el rechazo dejó escrituras")
			}
			ahora = time.Date(2026, 5, 4, 9, 0, 0, 0, loc)
			res, err = retirarDeEntregaAnticipada(ctx, svc, rs, false)
			if err != nil || len(res.Entregadas) != 1 {
				t.Fatalf("retiro al terminar la clase intermedia: %+v %v", res, err)
			}
		})
	}
}

func TestEntregaAnticipada_PostgresConservaPendientesEnRetiroParcial(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	for _, seleccionada := range []bool{false, true} {
		t.Run(fmt.Sprintf("impedida-seleccionada-%v", seleccionada), func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			libre := equipoDePrioridadCustodia(t, pool, "Libre")
			sobrante := equipoDePrioridadCustodia(t, pool, "Sobrante")
			rs := reservaPosteriorDeEntrega(t, pool, e, []string{libre, e.d.equipo, sobrante})
			var elegidas []*domain.Reserva
			var impedida, liberada string
			for _, r := range rs {
				switch r.EquipoID {
				case libre:
					elegidas = append(elegidas, r)
				case e.d.equipo:
					impedida = r.ID
					if seleccionada {
						elegidas = append(elegidas, r)
					}
				case sobrante:
					liberada = r.ID
				}
			}
			res, err := retirarDeEntregaAnticipada(ctx, e.reservas, elegidas, true)
			if err != nil || len(res.Entregadas) != 1 || len(res.Liberadas) != 1 || res.Liberadas[0] != liberada {
				t.Fatalf("retiro parcial: %+v %v", res, err)
			}
			var estado string
			if err := pool.QueryRow(ctx, `SELECT estado FROM reserva WHERE id=$1`, impedida).Scan(&estado); err != nil || estado != "CONFIRMADA" {
				t.Fatalf("se liberó la impedida: %s %v", estado, err)
			}
		})
	}
}

type lecturaDeIntermediasFallida struct {
	app.Repo
	equipo string
}

func (r lecturaDeIntermediasFallida) EnTransaccion(ctx context.Context, fn func(app.Repo) error) error {
	return r.Repo.EnTransaccion(ctx, func(repo app.Repo) error { return fn(lecturaDeIntermediasFallida{repo, r.equipo}) })
}
func (r lecturaDeIntermediasFallida) ListarReservasFuturasDeEquipo(ctx context.Context, id string, desde time.Time) ([]*domain.Reserva, error) {
	if id == r.equipo {
		return nil, errors.New("lectura de reservas no disponible")
	}
	return r.Repo.ListarReservasFuturasDeEquipo(ctx, id, desde)
}

func TestEntregaAnticipada_PostgresRevierteTodoElLoteSiFallaLaComprobacion(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	e := prepararInstitucion(t, pool)
	libre := equipoDePrioridadCustodia(t, pool, "Libre")
	rs := reservaPosteriorDeEntrega(t, pool, e, []string{libre, e.d.equipo})
	if rs[0].EquipoID != libre {
		rs[0], rs[1] = rs[1], rs[0]
	}
	svc := app.NewService(lecturaDeIntermediasFallida{infra.NewPostgresRepo(pool), e.d.equipo}, nil, infra.NewValidadorEquipoPostgres(pool), nil, nil, uuid.NewString, func() time.Time { return time.Date(2026, 5, 4, 6, 0, 0, 0, time.UTC) }, e.bus)
	antes := huellasBase(t, ctx, pool)
	res, err := retirarDeEntregaAnticipada(ctx, svc, rs, true)
	if err == nil || res != nil || !reflect.DeepEqual(antes, huellasBase(t, ctx, pool)) {
		t.Fatalf("el fallo no revirtió el lote: %+v %v", res, err)
	}
}

func TestEntregaAnticipada_PostgresEsperaUnaReservaConcurrente(t *testing.T) {
	pool := baseVacia(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	e := prepararInstitucion(t, pool)
	rs := reservaPosteriorDeEntrega(t, pool, e, []string{e.d.equipo})
	ejecutarCierreSQL(t, pool, `UPDATE reserva SET estado='CANCELADA' WHERE id=$1`, e.reservaID)
	pausa := nuevaPausaInstitucional(t)
	creador := e.reservasCon(pool, equipoValidadoEnPausa{infra.NewValidadorEquipoPostgres(pool), pausa})
	creada := make(chan error, 1)
	go func() {
		_, _, _, err := creador.CrearReserva(ctx, e.d.materia, e.d.docente, false, time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour, []string{e.d.equipo}, nil)
		creada <- err
	}()
	pausa.esperar(t, ctx)
	type respuesta struct {
		res *app.ResultadoEntrega
		err error
	}
	retirada := make(chan respuesta, 1)
	go func() {
		r, err := retirarDeEntregaAnticipada(ctx, e.reservas, rs, false)
		retirada <- respuesta{r, err}
	}()
	esperarOperacionInstitucional(t, ctx, pool)
	close(pausa.continuar)
	if err := <-creada; err != nil {
		t.Fatal(err)
	}
	r := <-retirada
	if r.err != nil || len(r.res.Entregadas) != 0 || len(r.res.NoEntregadas) != 1 || r.res.NoEntregadas[0].Razon != app.NoEntregadaReservaIntermedia {
		t.Fatalf("no vio la clase recién confirmada: %+v %v", r.res, r.err)
	}
}

func TestEntregaAnticipada_PostgresIntervalosNocturnos(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ nombre, fecha, inicio, fin, inicioRetiro, finRetiro string }{
		{"continua-desde-ayer", "2026-05-03", "22:00", "01:00", "12:00", "13:00"},
		{"retiro-de-mas-de-24-horas", "2026-05-04", "10:00", "11:00", "23:00", "07:00"},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			rs := reservaPosteriorDeEntrega(t, pool, e, []string{e.d.equipo})
			ejecutarCierreSQL(t, pool, `UPDATE reserva SET fecha=$2::date,hora_inicio=$3::time,hora_fin=$4::time WHERE id=$1`, e.reservaID, tc.fecha, tc.inicio, tc.fin)
			ejecutarCierreSQL(t, pool, `UPDATE reserva_grupo SET fecha=$2::date,hora_inicio=$3::time,hora_fin=$4::time WHERE id=$1`, e.d.grupo, tc.fecha, tc.inicio, tc.fin)
			ejecutarCierreSQL(t, pool, `UPDATE reserva SET hora_inicio=$2::time,hora_fin=$3::time WHERE id=$1`, rs[0].ID, tc.inicioRetiro, tc.finRetiro)
			ejecutarCierreSQL(t, pool, `UPDATE reserva_grupo SET hora_inicio=$2::time,hora_fin=$3::time WHERE id=$1`, *rs[0].ReservaGrupoID, tc.inicioRetiro, tc.finRetiro)
			svc := app.NewService(infra.NewPostgresRepo(pool), nil, infra.NewValidadorEquipoPostgres(pool), nil, nil, uuid.NewString, func() time.Time { return time.Date(2026, 5, 4, 0, 30, 0, 0, loc) }, e.bus)
			antes := huellasBase(t, ctx, pool)
			res, err := retirarDeEntregaAnticipada(ctx, svc, rs, true)
			if err != nil || len(res.Entregadas) != 0 || len(res.NoEntregadas) != 1 || res.NoEntregadas[0].Razon != app.NoEntregadaReservaIntermedia || !reflect.DeepEqual(antes, huellasBase(t, ctx, pool)) {
				t.Fatalf("intervalo nocturno sin protección: %+v %v", res, err)
			}
		})
	}
}
