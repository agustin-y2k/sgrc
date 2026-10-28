//go:build integration

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationdomain "github.com/agustin-y2k/sgrc/internal/reservation/domain"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type cambioDeEquipoEnPausa struct {
	reservationapp.Repo
	pausa *pausaInstitucional
}

func (r cambioDeEquipoEnPausa) EnTransaccion(ctx context.Context, fn func(reservationapp.Repo) error) error {
	return r.Repo.EnTransaccion(ctx, func(repo reservationapp.Repo) error {
		return fn(cambioDeEquipoEnPausa{repo, r.pausa})
	})
}

func (r cambioDeEquipoEnPausa) GuardarReserva(ctx context.Context, reserva *reservationdomain.Reserva) error {
	if err := r.Repo.GuardarReserva(ctx, reserva); err != nil {
		return err
	}
	return r.pausa.detener(ctx)
}

func TestCustodiaReservas_ConcurrenciaEIntegridad(t *testing.T) {
	pool := baseVacia(t)
	for _, entregaPrimero := range []bool{true, false} {
		nombre := "cambio-primero"
		if entregaPrimero {
			nombre = "entrega-primero"
		}
		t.Run(nombre, func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			nuevo := uuid.NewString()
			ejecutarCierreSQL(t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,'PC de reemplazo','Computadora')`, nuevo)
			ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancelar()
			pausa := nuevaPausaInstitucional(t)
			entregador := e.reservas
			cambiador := e.reservas
			if entregaPrimero {
				entregador = e.reservasCon(pool, equipoValidadoEnPausa{reservationinfra.NewValidadorEquipoPostgres(pool), pausa})
			} else {
				cambiador = reservationapp.NewService(cambioDeEquipoEnPausa{reservationinfra.NewPostgresRepo(pool), pausa}, nil, reservationinfra.NewValidadorEquipoPostgres(pool), nil, nil, uuid.NewString, time.Now, e.bus)
			}
			entregar := func() error {
				_, err := entregador.EntregarPorReserva(ctx, reservationapp.EntregaPorReservaParams{ReservaIDs: []string{e.reservaID}, EquiposEsperados: map[string]string{e.reservaID: e.d.equipo}})
				return err
			}
			cambiar := func() error {
				_, err := cambiador.CambiarEquipoDeReserva(ctx, e.reservaID, nuevo, e.d.docente, false, true)
				return err
			}
			primera, segunda := make(chan error, 1), make(chan error, 1)
			if entregaPrimero {
				go func() { primera <- entregar() }()
			} else {
				go func() { primera <- cambiar() }()
			}
			pausa.esperar(t, ctx)
			if entregaPrimero {
				go func() { segunda <- cambiar() }()
			} else {
				go func() { segunda <- entregar() }()
			}
			esperarOperacionInstitucional(t, ctx, pool)
			close(pausa.continuar)
			if err := <-primera; err != nil {
				t.Fatal(err)
			}
			esperado := reservationapp.ErrEquipoReservaDesactualizado
			if entregaPrimero {
				esperado = reservationapp.ErrReservaConEntrega
			}
			if err := <-segunda; !errors.Is(err, esperado) {
				t.Fatalf("esperaba %v, obtuve %v", esperado, err)
			}
			var inconsistentes, prestamos int
			if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM prestamo p JOIN reserva r ON r.id=p.reserva_id WHERE p.equipo_id<>r.equipo_id),(SELECT count(*) FROM prestamo)`).Scan(&inconsistentes, &prestamos); err != nil {
				t.Fatal(err)
			}
			if inconsistentes != 0 || (entregaPrimero && prestamos != 1) || (!entregaPrimero && prestamos != 0) {
				t.Fatalf("inconsistentes=%d préstamos=%d", inconsistentes, prestamos)
			}
		})
	}
	t.Run("base-protege-historial-y-admite-cierre", func(t *testing.T) {
		e := prepararInstitucion(t, pool)
		ctx := context.Background()
		nuevo := uuid.NewString()
		ejecutarCierreSQL(t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,'Otra PC','Computadora')`, nuevo)
		ret, err := e.entregar(ctx, e.reservas, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.reservas.RecibirEquipos(ctx, []string{ret.Entregadas[0].ID}, "", ""); err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `UPDATE reserva SET equipo_id=$2 WHERE id=$1`, e.reservaID, nuevo)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != "reserva_equipo_entregado" {
			t.Fatalf("se pudo reescribir el historial: %v", err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO prestamo(equipo_id,reserva_id,entregado_a_nombre) VALUES($1,$2,'Prueba')`, nuevo, e.reservaID)
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != "prestamo_equipo_reservado" {
			t.Fatalf("se pudo vincular otra PC: %v", err)
		}
		academico, _ := serviciosCierre(pool, "")
		if _, err := academico.ArchivarYClonar(ctx, e.d.ciclo, nil); err != nil {
			t.Fatal(err)
		}
		var reserva *string
		if err := pool.QueryRow(ctx, `SELECT reserva_id FROM prestamo WHERE id=$1`, ret.Entregadas[0].ID).Scan(&reserva); err != nil || reserva != nil {
			t.Fatalf("el cierre no conservó el préstamo: %v", err)
		}
	})
}

func esperadosDeTest(t *testing.T, pool *pgxpool.Pool, ids []string) map[string]string {
	t.Helper()
	filas, err := pool.Query(context.Background(), `SELECT id,equipo_id FROM reserva WHERE id=ANY($1)`, ids)
	if err != nil {
		t.Fatal(err)
	}
	defer filas.Close()
	esperados := make(map[string]string)
	for filas.Next() {
		var id, equipo string
		if err := filas.Scan(&id, &equipo); err != nil {
			t.Fatal(err)
		}
		esperados[id] = equipo
	}
	if err := filas.Err(); err != nil {
		t.Fatal(err)
	}
	return esperados
}
