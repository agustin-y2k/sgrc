//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authapp "github.com/agustin-y2k/sgrc/internal/auth/application"
	authdomain "github.com/agustin-y2k/sgrc/internal/auth/domain"
	authinfra "github.com/agustin-y2k/sgrc/internal/auth/infrastructure"
	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/agustin-y2k/sgrc/internal/shared/paginacion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type escenarioBaja struct {
	usuario               string
	materias, reservas    []string
	repo                  *authinfra.PostgresRepo
	avisos, reservaAvisos atomic.Int32
	bus                   eventbus.EventBus
}

func prepararBajaReal(t *testing.T, pool *pgxpool.Pool, admin bool) *escenarioBaja {
	t.Helper()
	ejecutarCierreSQL(t, pool, `TRUNCATE usuario,ciclo_lectivo,equipo CASCADE`)
	e := &escenarioBaja{usuario: uuid.NewString(), repo: authinfra.NewPostgresRepo(pool), bus: eventbus.NewInMemoryEventBus()}
	rol := "DOCENTE"
	if admin {
		rol = "ADMIN"
	}
	ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Ada','Docente','ada@test.local','hash',$2,'APROBADA'),($3,'Otro','Admin','otro@test.local','hash','ADMIN','APROBADA')`, e.usuario, rol, uuid.NewString())
	ciclo, curso := uuid.NewString(), uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO ciclo_lectivo(id,anio) VALUES($1,2026)`, ciclo)
	ejecutarCierreSQL(t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division) VALUES($1,$2,1,'A')`, curso, ciclo)
	for i := range 2 {
		materia, grupo, reserva, equipo := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		ejecutarCierreSQL(t, pool, `INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,$3)`, materia, curso, fmt.Sprintf("Materia %d", i))
		ejecutarCierreSQL(t, pool, `INSERT INTO docente_materia(id,usuario_id,materia_id,rol) VALUES($1,$2,$3,'TITULAR')`, uuid.NewString(), e.usuario, materia)
		ejecutarCierreSQL(t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,$2,'Computadora')`, equipo, fmt.Sprintf("Notebook %d", i))
		ejecutarCierreSQL(t, pool, `INSERT INTO reserva_grupo(id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,'Ada Docente','2026-05-04','08:00','09:00')`, grupo, materia, e.usuario)
		ejecutarCierreSQL(t, pool, `INSERT INTO reserva(id,reserva_grupo_id,equipo_id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,$4,$5,'Ada Docente','2026-05-04','08:00','09:00')`, reserva, grupo, equipo, materia, e.usuario)
		e.materias = append(e.materias, materia)
		e.reservas = append(e.reservas, reserva)
	}
	e.bus.Subscribe("docente.baja.materia-huerfana", func(eventbus.Evento) { e.avisos.Add(1) })
	e.bus.Subscribe("reserva.cancelada", func(eventbus.Evento) { e.reservaAvisos.Add(1) })
	return e
}

func servicioBajaReal(pool *pgxpool.Pool, e *escenarioBaja, repo authapp.Repo) *authapp.Service {
	ahora := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	reservas := reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), nil, reservationinfra.NewValidadorEquipoPostgres(pool), nil, reservationinfra.NewObtenedorNombrePostgres(pool), uuid.NewString, ahora, e.bus)
	return authapp.NewService(repo, e.bus, nil, nil, nil, uuid.NewString, nil, nil, ahora, authinfra.NewGestorMateriasDocentePostgres(pool), &authCanceladorReservasAdapter{reservationSvc: reservas}, nil, true)
}

func falloBajaSQL(t *testing.T, pool *pgxpool.Pool, tabla, operacion, condicion string) func() {
	t.Helper()
	ejecutarCierreSQL(t, pool, `CREATE FUNCTION fallo_baja() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fallo de baja inyectado'; END $$`)
	ejecutarCierreSQL(t, pool, fmt.Sprintf(`CREATE TRIGGER fallo_baja AFTER %s ON %s FOR EACH ROW %s EXECUTE FUNCTION fallo_baja()`, operacion, tabla, condicion))
	return func() {
		ejecutarCierreSQL(t, pool, fmt.Sprintf(`DROP TRIGGER fallo_baja ON %s`, tabla))
		ejecutarCierreSQL(t, pool, `DROP FUNCTION fallo_baja()`)
	}
}

func TestBajaUsuario_ProgresoPersistidoRollbackYReinicio(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	for _, paso := range []string{"preparacion", "cancelacion", "avance", "remocion", "marca-final"} {
		t.Run(paso, func(t *testing.T) {
			e := prepararBajaReal(t, pool, false)
			var quitar func()
			switch paso {
			case "preparacion":
				quitar = falloBajaSQL(t, pool, "baja_usuario_materia", "INSERT", "")
			case "cancelacion":
				quitar = falloBajaSQL(t, pool, "reserva", "UPDATE", fmt.Sprintf("WHEN (NEW.materia_id='%s'::uuid)", e.materias[0]))
			case "avance":
				quitar = falloBajaSQL(t, pool, "baja_usuario_materia", "UPDATE", fmt.Sprintf("WHEN (NEW.materia_id='%s'::uuid)", e.materias[0]))
			case "remocion":
				quitar = falloBajaSQL(t, pool, "docente_materia", "DELETE", "")
			case "marca-final":
				quitar = falloBajaSQL(t, pool, "baja_usuario", "UPDATE", "")
			}
			svc := servicioBajaReal(pool, e, e.repo)
			err := svc.DarDeBaja(ctx, e.usuario)
			quitar()
			if err == nil {
				t.Fatal("se ocultó el fallo SQL")
			}
			u, err := e.repo.BuscarPorID(ctx, e.usuario)
			if err != nil {
				t.Fatal(err)
			}
			p, err := e.repo.BuscarBaja(ctx, e.usuario)
			if err != nil {
				t.Fatal(err)
			}
			if paso == "preparacion" {
				if u.Estado != authdomain.EstadoAprobada || p != nil || e.avisos.Load() != 0 || e.reservaAvisos.Load() != 0 {
					t.Fatalf("preparación parcial: %+v %+v", u, p)
				}
			} else {
				if u.Estado != authdomain.EstadoBaja || !u.BajaPendiente || p == nil || p.Completada {
					t.Fatalf("se perdió el pendiente o se reactivó la cuenta: %+v %+v", u, p)
				}
				if _, err := svc.EliminarDefinitivamente(ctx, e.usuario); !errors.Is(err, authapp.ErrBajaPendiente) {
					t.Fatalf("se permitió eliminar el pendiente: %v", err)
				}
				if _, err := e.repo.Eliminar(ctx, e.usuario); !errors.Is(err, authapp.ErrBajaPendiente) {
					t.Fatalf("el SQL permitió eliminar el pendiente: %v", err)
				}
				var asignaciones int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM docente_materia WHERE usuario_id=$1`, e.usuario).Scan(&asignaciones); err != nil || asignaciones != 2 {
					t.Fatalf("remoción parcial: %d %v", asignaciones, err)
				}
				if paso == "cancelacion" || paso == "avance" {
					var estado string
					if err := pool.QueryRow(ctx, `SELECT estado FROM reserva WHERE id=$1`, e.reservas[0]).Scan(&estado); err != nil || estado != "CONFIRMADA" {
						t.Fatalf("cancelación sin avance confirmado: %s %v", estado, err)
					}
					if len(p.MateriasPendientes) != 1 || e.avisos.Load() != 1 || e.reservaAvisos.Load() != 1 {
						t.Fatalf("avisos antes del commit o avance perdido: %+v %d %d", p, e.avisos.Load(), e.reservaAvisos.Load())
					}
				} else if len(p.MateriasPendientes) != 0 || e.avisos.Load() != 2 || e.reservaAvisos.Load() != 2 {
					t.Fatalf("progreso final perdido: %+v", p)
				}
			}
			// Reconstruir ambos servicios prueba que no se depende de memoria del proceso.
			svc = servicioBajaReal(pool, e, authinfra.NewPostgresRepo(pool))
			if err := svc.DarDeBaja(ctx, e.usuario); err != nil {
				t.Fatalf("reintento tras reinicio: %v", err)
			}
			if err := svc.DarDeBaja(ctx, e.usuario); err != nil {
				t.Fatal(err)
			}
			p, err = e.repo.BuscarBaja(ctx, e.usuario)
			if err != nil || !p.Completada || len(p.MateriasPendientes) != 0 {
				t.Fatalf("baja incompleta: %+v %v", p, err)
			}
			u, err = e.repo.BuscarPorID(ctx, e.usuario)
			if err != nil || u.Estado != authdomain.EstadoBaja || u.BajaPendiente {
				t.Fatalf("proyección incorrecta: %+v %v", u, err)
			}
			usuarios, _, err := e.repo.Listar(ctx, nil, nil, paginacion.Pagina{Numero: 1, Tamanio: 20})
			if err != nil {
				t.Fatal(err)
			}
			for _, u := range usuarios {
				if u.ID == e.usuario && u.BajaPendiente {
					t.Fatal("listado conserva pendiente ya completado")
				}
			}
			var asignaciones, canceladas int
			if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM docente_materia WHERE usuario_id=$1),(SELECT count(*) FROM reserva WHERE id=ANY($2) AND estado='CANCELADA')`, e.usuario, e.reservas).Scan(&asignaciones, &canceladas); err != nil || asignaciones != 0 || canceladas != 2 || e.avisos.Load() != 2 || e.reservaAvisos.Load() != 2 {
				t.Fatalf("resultado o efectos repetidos: asignaciones=%d canceladas=%d avisos=%d/%d %v", asignaciones, canceladas, e.avisos.Load(), e.reservaAvisos.Load(), err)
			}
		})
	}
	t.Run("admin-con-materias", func(t *testing.T) {
		e := prepararBajaReal(t, pool, true)
		if err := servicioBajaReal(pool, e, e.repo).DarDeBaja(ctx, e.usuario); err != nil || e.avisos.Load() != 2 {
			t.Fatalf("admin sin cascada: %v", err)
		}
	})
	t.Run("baja-legada", func(t *testing.T) {
		e := prepararBajaReal(t, pool, false)
		ejecutarCierreSQL(t, pool, `UPDATE usuario SET estado='BAJA' WHERE id=$1`, e.usuario)
		if err := servicioBajaReal(pool, e, e.repo).DarDeBaja(ctx, e.usuario); err != nil || e.avisos.Load() != 2 {
			t.Fatalf("no se recuperó la baja legada: %v", err)
		}
	})
	t.Run("otro-docente-activo", func(t *testing.T) {
		e := prepararBajaReal(t, pool, false)
		ejecutarCierreSQL(t, pool, `INSERT INTO docente_materia(id,usuario_id,materia_id,rol) SELECT $1,id,$2,'SUPLENTE' FROM usuario WHERE email='otro@test.local'`, uuid.NewString(), e.materias[0])
		if err := servicioBajaReal(pool, e, e.repo).DarDeBaja(ctx, e.usuario); err != nil {
			t.Fatal(err)
		}
		var estado string
		if err := pool.QueryRow(ctx, `SELECT estado FROM reserva WHERE id=$1`, e.reservas[0]).Scan(&estado); err != nil || estado != "CONFIRMADA" || e.avisos.Load() != 1 {
			t.Fatalf("canceló la materia con otro docente: %s %v", estado, err)
		}
	})
}

type repoBajaEnPausa struct {
	authapp.Repo
	preparado, seguir chan struct{}
	unaVez            sync.Once
}

func (r *repoBajaEnPausa) EnBaja(ctx context.Context, id string, fn func(context.Context, authapp.Repo) error) error {
	return r.Repo.EnBaja(ctx, id, func(ctx context.Context, repo authapp.Repo) error {
		return fn(ctx, &repoBajaEnPausa{Repo: repo, preparado: r.preparado, seguir: r.seguir})
	})
}
func (r *repoBajaEnPausa) CompletarMateriaDeBaja(ctx context.Context, u, m string, ahora time.Time) error {
	if err := r.Repo.CompletarMateriaDeBaja(ctx, u, m, ahora); err != nil {
		return err
	}
	r.unaVez.Do(func() {
		select {
		case <-r.preparado:
		default:
			close(r.preparado)
		}
	})
	select {
	case <-r.seguir:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func TestBajaUsuario_ReintentosConcurrentes(t *testing.T) {
	pool := baseVacia(t)
	e := prepararBajaReal(t, pool, false)
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	pausa := &repoBajaEnPausa{Repo: e.repo, preparado: make(chan struct{}), seguir: make(chan struct{})}
	a, b := make(chan error, 1), make(chan error, 1)
	go func() { a <- servicioBajaReal(pool, e, pausa).DarDeBaja(ctx, e.usuario) }()
	select {
	case <-pausa.preparado:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() { b <- servicioBajaReal(pool, e, e.repo).DarDeBaja(ctx, e.usuario) }()
	bloqueada := false
	for !bloqueada && ctx.Err() == nil {
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%pg_advisory_xact_lock%')`).Scan(&bloqueada); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-b:
			t.Fatalf("el reintento no esperó: %v", err)
		default:
		}
		if !bloqueada {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !bloqueada {
		t.Fatal("no se observó el bloqueo")
	}
	if e.avisos.Load() != 0 || e.reservaAvisos.Load() != 0 {
		t.Fatal("aviso publicado antes del commit de la materia")
	}
	close(pausa.seguir)
	if err := <-a; err != nil {
		t.Fatal(err)
	}
	if err := <-b; err != nil {
		t.Fatal(err)
	}
	if e.avisos.Load() != 2 || e.reservaAvisos.Load() != 2 {
		t.Fatalf("avisos duplicados: %d/%d", e.avisos.Load(), e.reservaAvisos.Load())
	}
}
