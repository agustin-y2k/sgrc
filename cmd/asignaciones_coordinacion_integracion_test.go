//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	academicapp "github.com/agustin-y2k/sgrc/internal/academic/application"
	academicdomain "github.com/agustin-y2k/sgrc/internal/academic/domain"
	academicinfra "github.com/agustin-y2k/sgrc/internal/academic/infrastructure"
	authapp "github.com/agustin-y2k/sgrc/internal/auth/application"
	authinfra "github.com/agustin-y2k/sgrc/internal/auth/infrastructure"
	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Las pausas ocurren después de una lectura real. La segunda operación debe
// esperar en PostgreSQL, no sólo terminar con un estado favorable por azar.
type usuariosAcademicEnPausa struct {
	academicapp.ValidadorUsuario
	pausa      *pausaInstitucional
	asignacion bool
}

func (v usuariosAcademicEnPausa) ExisteYAprobado(ctx context.Context, id string) (bool, error) {
	valido, err := v.ValidadorUsuario.ExisteYAprobado(ctx, id)
	if err == nil && v.asignacion {
		err = v.pausa.detener(ctx)
	}
	return valido, err
}
func (v usuariosAcademicEnPausa) AlgunoAprobado(ctx context.Context, ids []string) (bool, error) {
	valido, err := v.ValidadorUsuario.AlgunoAprobado(ctx, ids)
	if err == nil && !v.asignacion {
		err = v.pausa.detener(ctx)
	}
	return valido, err
}

type materiasBajaEnPausa struct {
	authapp.GestorMateriasDocente
	pausa       *pausaInstitucional
	preparacion bool
}

func (g materiasBajaEnPausa) MateriasDeDocente(ctx context.Context, id string) ([]string, error) {
	ids, err := g.GestorMateriasDocente.MateriasDeDocente(ctx, id)
	if err == nil && g.preparacion {
		err = g.pausa.detener(ctx)
	}
	return ids, err
}
func (g materiasBajaEnPausa) QuedaOtroDocenteActivo(ctx context.Context, materia, id string) (bool, error) {
	otro, err := g.GestorMateriasDocente.QuedaOtroDocenteActivo(ctx, materia, id)
	if err == nil && !g.preparacion {
		err = g.pausa.detener(ctx)
	}
	return otro, err
}
func servicioAsignacionesReal(pool *pgxpool.Pool, e *escenarioBaja, validador academicapp.ValidadorUsuario) *academicapp.Service {
	ahora := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	reservas := reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), nil, reservationinfra.NewValidadorEquipoPostgres(pool), nil, reservationinfra.NewObtenedorNombrePostgres(pool), uuid.NewString, ahora, e.bus)
	return academicapp.NewService(academicinfra.NewPostgresRepo(pool), validador, nil, nil, &authCanceladorReservasAdapter{reservationSvc: reservas}, academicinfra.NewDatosDeUsuarioPostgres(pool), nil, uuid.NewString, ahora, e.bus)
}
func servicioBajaConGestor(pool *pgxpool.Pool, e *escenarioBaja, gestor authapp.GestorMateriasDocente) *authapp.Service {
	ahora := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	reservas := reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), nil, reservationinfra.NewValidadorEquipoPostgres(pool), nil, reservationinfra.NewObtenedorNombrePostgres(pool), uuid.NewString, ahora, e.bus)
	return authapp.NewService(authinfra.NewPostgresRepo(pool), e.bus, nil, nil, nil, uuid.NewString, nil, nil, ahora, gestor, &authCanceladorReservasAdapter{reservationSvc: reservas}, nil, true)
}
func vinculoReal(t *testing.T, pool *pgxpool.Pool, usuario, materia string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM docente_materia WHERE usuario_id=$1 AND materia_id=$2`, usuario, materia).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func agregarOtroDocente(t *testing.T, pool *pgxpool.Pool, e *escenarioBaja) string {
	t.Helper()
	id := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Otro','Docente','docente2@test.local','hash','DOCENTE','APROBADA')`, id)
	for _, materia := range e.materias {
		ejecutarCierreSQL(t, pool, `INSERT INTO docente_materia(id,usuario_id,materia_id,rol) VALUES($1,$2,$3,'SUPLENTE')`, uuid.NewString(), id, materia)
	}
	return id
}
func comprobarMateriaSinDocentes(t *testing.T, pool *pgxpool.Pool, materia string) {
	t.Helper()
	var activos, confirmadas int
	err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM docente_materia dm JOIN usuario u ON u.id=dm.usuario_id WHERE dm.materia_id=$1 AND u.estado='APROBADA'),(SELECT count(*) FROM reserva WHERE materia_id=$1 AND estado='CONFIRMADA')`, materia).Scan(&activos, &confirmadas)
	if err != nil {
		t.Fatal(err)
	}
	if activos != 0 || confirmadas != 0 {
		t.Fatalf("materia sin cierre consistente: activos=%d confirmadas=%d", activos, confirmadas)
	}
}
func recibirErrorAsignaciones(t *testing.T, ctx context.Context, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		t.Fatal(ctx.Err())
		return ctx.Err()
	}
}

func TestAsignacionesCoordinacion_RemocionesYBajasConcurrentes(t *testing.T) {
	pool := baseVacia(t)
	for _, modo := range []string{"remociones", "bajas", "remocion-baja", "baja-remocion"} {
		t.Run(modo, func(t *testing.T) {
			ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancelar()
			e := prepararBajaReal(t, pool, false)
			otro := agregarOtroDocente(t, pool, e)
			id1, id2 := vinculoReal(t, pool, e.usuario, e.materias[0]), vinculoReal(t, pool, otro, e.materias[0])
			pausa := nuevaPausaInstitucional(t)
			primera, segunda := make(chan error, 1), make(chan error, 1)
			var canceladas atomic.Int32
			if modo == "remociones" || modo == "remocion-baja" {
				svc := servicioAsignacionesReal(pool, e, usuariosAcademicEnPausa{academicinfra.NewValidadorUsuarioPostgres(pool), pausa, false})
				go func() { n, err := svc.RemoverDocenteMateria(ctx, id1); canceladas.Add(int32(n)); primera <- err }()
			} else {
				svc := servicioBajaConGestor(pool, e, materiasBajaEnPausa{authinfra.NewGestorMateriasDocentePostgres(pool), pausa, false})
				go func() { primera <- svc.DarDeBaja(ctx, e.usuario) }()
			}
			pausa.esperar(t, ctx)
			if modo == "remociones" || modo == "baja-remocion" {
				svc := servicioAsignacionesReal(pool, e, academicinfra.NewValidadorUsuarioPostgres(pool))
				go func() { n, err := svc.RemoverDocenteMateria(ctx, id2); canceladas.Add(int32(n)); segunda <- err }()
			} else {
				svc := servicioBajaReal(pool, e, e.repo)
				go func() { segunda <- svc.DarDeBaja(ctx, otro) }()
			}
			esperarOperacionInstitucional(t, ctx, pool)
			close(pausa.continuar)
			for _, ch := range []<-chan error{primera, segunda} {
				if err := recibirErrorAsignaciones(t, ctx, ch); err != nil {
					t.Fatal(err)
				}
			}
			comprobarMateriaSinDocentes(t, pool, e.materias[0])
			if modo == "remociones" && (canceladas.Load() != 1 || e.reservaAvisos.Load() != 1) {
				t.Fatalf("cancelaciones repetidas: cambios=%d avisos=%d", canceladas.Load(), e.reservaAvisos.Load())
			}
			if modo == "bajas" {
				for _, materia := range e.materias {
					comprobarMateriaSinDocentes(t, pool, materia)
				}
				for _, id := range []string{e.usuario, otro} {
					p, err := e.repo.BuscarBaja(ctx, id)
					if err != nil || p == nil || !p.Completada {
						t.Fatalf("baja incompleta: %+v %v", p, err)
					}
				}
				if e.reservaAvisos.Load() != 2 {
					t.Fatalf("avisos de reserva duplicados: %d", e.reservaAvisos.Load())
				}
			}
		})
	}
}

func TestAsignacionesCoordinacion_AltaYPedidoDuranteBaja(t *testing.T) {
	pool := baseVacia(t)
	for _, pedido := range []bool{false, true} {
		for _, bajaPrimero := range []bool{false, true} {
			t.Run(fmt.Sprintf("pedido=%t/bajaPrimero=%t", pedido, bajaPrimero), func(t *testing.T) {
				ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancelar()
				e := prepararBajaReal(t, pool, false)
				materia := uuid.NewString()
				ejecutarCierreSQL(t, pool, `INSERT INTO materia(id,curso_id,nombre) SELECT $1,curso_id,'Materia nueva' FROM materia WHERE id=$2`, materia, e.materias[0])
				pedidoID := uuid.NewString()
				var admin string
				if err := pool.QueryRow(ctx, `SELECT id FROM usuario WHERE rol='ADMIN' AND estado='APROBADA'`).Scan(&admin); err != nil {
					t.Fatal(err)
				}
				if pedido {
					ejecutarCierreSQL(t, pool, `INSERT INTO pedido_de_materia(id,usuario_id,materia_id,motivo) VALUES($1,$2,$3,'Tomo esta materia')`, pedidoID, e.usuario, materia)
				}
				pausa := nuevaPausaInstitucional(t)
				validador := academicapp.ValidadorUsuario(academicinfra.NewValidadorUsuarioPostgres(pool))
				gestor := authapp.GestorMateriasDocente(authinfra.NewGestorMateriasDocentePostgres(pool))
				if bajaPrimero {
					gestor = materiasBajaEnPausa{gestor, pausa, true}
				} else {
					validador = usuariosAcademicEnPausa{validador, pausa, true}
				}
				academic := servicioAsignacionesReal(pool, e, validador)
				auth := servicioBajaConGestor(pool, e, gestor)
				altaResultado, bajaResultado := make(chan error, 1), make(chan error, 1)
				alta := func() {
					var err error
					if pedido {
						_, err = academic.ResolverPedido(ctx, pedidoID, admin, true, "Aprobado", nil, nil)
					} else {
						_, err = academic.AsignarDocente(ctx, materia, e.usuario, academicdomain.RolTitular)
					}
					altaResultado <- err
				}
				baja := func() { bajaResultado <- auth.DarDeBaja(ctx, e.usuario) }
				if bajaPrimero {
					go baja()
				} else {
					go alta()
				}
				pausa.esperar(t, ctx)
				if bajaPrimero {
					go alta()
				} else {
					go baja()
				}
				esperarOperacionInstitucional(t, ctx, pool)
				close(pausa.continuar)
				errAlta := recibirErrorAsignaciones(t, ctx, altaResultado)
				if bajaPrimero {
					if !errors.Is(errAlta, academicapp.ErrUsuarioNoValidoParaAsignar) {
						t.Fatalf("se asignó una cuenta en BAJA: %v", errAlta)
					}
				} else if errAlta != nil {
					t.Fatal(errAlta)
				}
				if err := recibirErrorAsignaciones(t, ctx, bajaResultado); err != nil {
					t.Fatal(err)
				}
				var asignaciones, tareas int
				if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM docente_materia WHERE usuario_id=$1),(SELECT count(*) FROM baja_usuario_materia WHERE usuario_id=$1 AND materia_id=$2)`, e.usuario, materia).Scan(&asignaciones, &tareas); err != nil {
					t.Fatal(err)
				}
				esperado := 1
				if bajaPrimero {
					esperado = 0
				}
				if asignaciones != 0 || tareas != esperado {
					t.Fatalf("alta escapó a la baja: vínculos=%d tareas=%d", asignaciones, tareas)
				}
				if pedido {
					p, err := academicinfra.NewPostgresRepo(pool).BuscarPedidoPorID(ctx, pedidoID)
					if err != nil {
						t.Fatal(err)
					}
					estado := academicdomain.PedidoAprobado
					if bajaPrimero {
						estado = academicdomain.PedidoPendiente
					}
					if p.Estado != estado {
						t.Fatalf("resolución incorrecta: %+v", p)
					}
				}
			})
		}
	}
}

func TestAsignacionesCoordinacion_RollbackYPoolUnico(t *testing.T) {
	pool := baseVacia(t)
	for _, fallo := range []string{"cancelacion", "commit"} {
		t.Run(fallo, func(t *testing.T) {
			ctx := context.Background()
			e := prepararBajaReal(t, pool, false)
			id := vinculoReal(t, pool, e.usuario, e.materias[0])
			var avisos atomic.Int32
			e.bus.Subscribe("docente.desasignado.materia-huerfana", func(eventbus.Evento) { avisos.Add(1) })
			cfg := pool.Config().Copy()
			cfg.MaxConns = 1
			cfg.MinConns = 0
			unico, err := pgxpool.NewWithConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer unico.Close()
			svc := servicioAsignacionesReal(unico, e, academicinfra.NewValidadorUsuarioPostgres(unico))
			// Una restricción diferida falla al confirmar, después de preparar todos
			// los cambios y avisos. Nada de eso debe persistir ni publicarse.
			var quitar func()
			if fallo == "cancelacion" {
				quitar = falloBajaSQL(t, pool, "reserva", "UPDATE", "")
			} else {
				ejecutarCierreSQL(t, pool, `CREATE FUNCTION fallo_asignacion_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fallo al confirmar'; END $$`)
				ejecutarCierreSQL(t, pool, `CREATE CONSTRAINT TRIGGER fallo_asignacion_commit AFTER DELETE ON docente_materia DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fallo_asignacion_commit()`)
				quitar = func() {
					ejecutarCierreSQL(t, pool, `DROP TRIGGER fallo_asignacion_commit ON docente_materia`)
					ejecutarCierreSQL(t, pool, `DROP FUNCTION fallo_asignacion_commit()`)
				}
			}
			plazo, cancelar := context.WithTimeout(ctx, 10*time.Second)
			defer cancelar()
			n, err := svc.RemoverDocenteMateria(plazo, id)
			quitar()
			if err == nil || n != 0 {
				t.Fatalf("fallo oculto: n=%d err=%v", n, err)
			}
			var estado, grupo string
			if err := pool.QueryRow(ctx, `SELECT r.estado,g.estado FROM reserva r JOIN reserva_grupo g ON g.id=r.reserva_grupo_id WHERE r.id=$1`, e.reservas[0]).Scan(&estado, &grupo); err != nil {
				t.Fatal(err)
			}
			if vinculoReal(t, pool, e.usuario, e.materias[0]) != id || estado != "CONFIRMADA" || grupo != "CONFIRMADA" || avisos.Load() != 0 || e.reservaAvisos.Load() != 0 {
				t.Fatalf("rollback incompleto: reserva=%s grupo=%s avisos=%d/%d", estado, grupo, avisos.Load(), e.reservaAvisos.Load())
			}
			n, err = svc.RemoverDocenteMateria(plazo, id)
			if err != nil || n != 1 || avisos.Load() != 1 || e.reservaAvisos.Load() != 1 {
				t.Fatalf("reintento incorrecto: %d %v avisos=%d/%d", n, err, avisos.Load(), e.reservaAvisos.Load())
			}
			comprobarMateriaSinDocentes(t, pool, e.materias[0])
			// Los demás caminos de asignación también deben reutilizar esa conexión.
			dm, err := svc.AsignarDocente(plazo, e.materias[0], e.usuario, academicdomain.RolTitular)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.CambiarRolDocente(plazo, dm.ID, academicdomain.RolSuplente); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.RemoverDocenteMateria(plazo, dm.ID); err != nil {
				t.Fatal(err)
			}
			var admin string
			if err := pool.QueryRow(ctx, `SELECT id FROM usuario WHERE rol='ADMIN'`).Scan(&admin); err != nil {
				t.Fatal(err)
			}
			pedido := uuid.NewString()
			ejecutarCierreSQL(t, pool, `INSERT INTO pedido_de_materia(id,usuario_id,materia_id,motivo) VALUES($1,$2,$3,'Retomo esta materia')`, pedido, e.usuario, e.materias[0])
			if _, err := svc.ResolverPedido(plazo, pedido, admin, true, "Aprobado", nil, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type materiaReservaEnPausa struct {
	reservationapp.ValidadorMateria
	pausa *pausaInstitucional
}

func (v materiaReservaEnPausa) DocenteEstaAsignado(ctx context.Context, materia, usuario string) (bool, error) {
	asignado, err := v.ValidadorMateria.DocenteEstaAsignado(ctx, materia, usuario)
	if err == nil {
		err = v.pausa.detener(ctx)
	}
	return asignado, err
}

func TestAsignacionesCoordinacion_ReservaEnCursoYCuentaInactiva(t *testing.T) {
	pool := baseVacia(t)
	for _, baja := range []bool{false, true} {
		t.Run(fmt.Sprintf("baja=%t", baja), func(t *testing.T) {
			ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancelar()
			e := prepararInstitucion(t, pool)
			pausa := nuevaPausaInstitucional(t)
			validador := materiaReservaEnPausa{reservationinfra.NewValidadorMateriaPostgres(pool), pausa}
			reservas := reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), validador, reservationinfra.NewValidadorEquipoPostgres(pool), &reservationValidadorJornadaAdapter{e.jornada}, reservationinfra.NewObtenedorNombrePostgres(pool), uuid.NewString, func() time.Time { return time.Date(2026, 5, 4, 6, 0, 0, 0, time.UTC) }, e.bus)
			primera, segunda := make(chan error, 1), make(chan error, 1)
			go func() {
				_, _, _, err := reservas.CrearReserva(ctx, e.d.materia, e.d.docente, false, time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour, []string{e.d.equipo}, nil)
				primera <- err
			}()
			pausa.esperar(t, ctx)
			bajaEscenario := &escenarioBaja{usuario: e.d.docente, repo: authinfra.NewPostgresRepo(pool), bus: e.bus}
			if baja {
				svc := servicioBajaReal(pool, bajaEscenario, bajaEscenario.repo)
				go func() { segunda <- svc.DarDeBaja(ctx, e.d.docente) }()
			} else {
				svc := servicioAsignacionesReal(pool, bajaEscenario, academicinfra.NewValidadorUsuarioPostgres(pool))
				id := vinculoReal(t, pool, e.d.docente, e.d.materia)
				go func() { _, err := svc.RemoverDocenteMateria(ctx, id); segunda <- err }()
			}
			esperarOperacionInstitucional(t, ctx, pool)
			close(pausa.continuar)
			for _, ch := range []<-chan error{primera, segunda} {
				if err := recibirErrorAsignaciones(t, ctx, ch); err != nil {
					t.Fatal(err)
				}
			}
			comprobarMateriaSinDocentes(t, pool, e.d.materia)
			// Una solicitud que ya había pasado por autenticación debe releer la
			// asignación después de esperar la remoción/baja.
			_, _, _, err := e.reservas.CrearReserva(ctx, e.d.materia, e.d.docente, false, time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour, []string{e.d.equipo}, nil)
			if !errors.Is(err, reservationapp.ErrDocenteNoAsignado) {
				t.Fatalf("autorización obsoleta: %v", err)
			}
		})
	}
	t.Run("baja-pendiente-conserva-vinculo", func(t *testing.T) {
		ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelar()
		e := prepararInstitucion(t, pool)
		bajaEscenario := &escenarioBaja{usuario: e.d.docente, repo: authinfra.NewPostgresRepo(pool), bus: e.bus}
		quitar := falloBajaSQL(t, pool, "reserva", "UPDATE", "")
		err := servicioBajaReal(pool, bajaEscenario, bajaEscenario.repo).DarDeBaja(ctx, e.d.docente)
		quitar()
		if !errors.Is(err, authapp.ErrBajaPendiente) {
			t.Fatalf("no quedó la baja pendiente: %v", err)
		}
		vinculoReal(t, pool, e.d.docente, e.d.materia)
		_, _, _, err = e.reservas.CrearReserva(ctx, e.d.materia, e.d.docente, false, time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour, []string{e.d.equipo}, nil)
		if !errors.Is(err, reservationapp.ErrDocenteNoAsignado) {
			t.Fatalf("el vínculo pendiente permitió reservar a una cuenta BAJA: %v", err)
		}
	})
}
