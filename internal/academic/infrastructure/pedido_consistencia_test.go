//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/academic/application"
	"github.com/agustin-y2k/sgrc/internal/academic/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/jackc/pgx/v5/pgxpool"
)

type escenarioPedido struct {
	repo                          *PostgresRepo
	svc                           *application.Service
	pedido, curso, docente, admin string
	avisos                        atomic.Int32
}

func sqlPedido(t *testing.T, pool *pgxpool.Pool, consulta string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), consulta, args...); err != nil {
		t.Fatal(err)
	}
}

func nuevoEscenarioPedido(t *testing.T, pool *pgxpool.Pool, nuevo bool) *escenarioPedido {
	t.Helper()
	e := &escenarioPedido{repo: NewPostgresRepo(pool), curso: uuidNuevo(), docente: insertarUsuarioDeTest(t, pool, "APROBADA"), admin: insertarUsuarioDeTest(t, pool, "APROBADA")}
	sqlPedido(t, pool, `UPDATE usuario SET rol='ADMIN' WHERE id=$1`, e.admin)
	ciclo := uuidNuevo()
	sqlPedido(t, pool, `INSERT INTO ciclo_lectivo(id,anio,activo) VALUES($1,(SELECT COALESCE(MAX(anio),2020)+1 FROM ciclo_lectivo),false)`, ciclo)
	sqlPedido(t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division) VALUES($1,$2,1,'A')`, e.curso, ciclo)
	var materia *string
	texto := "Robótica"
	if !nuevo {
		m, err := domain.NuevaMateria(uuidNuevo(), e.curso, texto)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.repo.CrearMateria(context.Background(), m); err != nil {
			t.Fatal(err)
		}
		materia = &m.ID
		texto = ""
	}
	p, err := domain.NuevoPedidoDeMateria(uuidNuevo(), e.docente, materia, "1°A", texto, "Tomo el segundo turno", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := e.repo.CrearPedido(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	e.pedido = p.ID
	bus := eventbus.NewInMemoryEventBus()
	bus.Subscribe("materia.pedido.resuelto", func(eventbus.Evento) { e.avisos.Add(1) })
	e.svc = application.NewService(e.repo, NewValidadorUsuarioPostgres(pool), nil, nil, nil, NewDatosDeUsuarioPostgres(pool), nil, uuidNuevo, time.Now, bus)
	return e
}

func verificarPedidoPendiente(t *testing.T, pool *pgxpool.Pool, e *escenarioPedido) {
	t.Helper()
	p, err := e.repo.BuscarPedidoPorID(context.Background(), e.pedido)
	if err != nil || p.Estado != domain.PedidoPendiente || p.MateriaCreadaID != nil || p.ResueltoEn != nil {
		t.Fatalf("pedido alterado: %+v %v", p, err)
	}
	var materias, asignaciones int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT COUNT(*) FROM materia WHERE curso_id=$1),(SELECT COUNT(*) FROM docente_materia WHERE usuario_id=$2)`, e.curso, e.docente).Scan(&materias, &asignaciones); err != nil {
		t.Fatal(err)
	}
	if materias != 0 || asignaciones != 0 || e.avisos.Load() != 0 {
		t.Fatalf("efectos ocultos: materias=%d asignaciones=%d avisos=%d", materias, asignaciones, e.avisos.Load())
	}
}

func TestPedidosConsistencia_RollbackVinculoYReintentos(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	ctx := context.Background()
	for _, tabla := range []string{"materia", "docente_materia", "pedido_de_materia"} {
		t.Run("fallo-"+tabla, func(t *testing.T) {
			e := nuevoEscenarioPedido(t, pool, true)
			op := "INSERT"
			if tabla == "pedido_de_materia" {
				op = "UPDATE"
			}
			sqlPedido(t, pool, `CREATE FUNCTION fallo_pedido() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fallo inyectado'; END $$`)
			sqlPedido(t, pool, fmt.Sprintf(`CREATE TRIGGER fallo_pedido AFTER %s ON %s FOR EACH ROW EXECUTE FUNCTION fallo_pedido()`, op, tabla))
			res, err := e.svc.ResolverPedido(ctx, e.pedido, e.admin, true, "Aprobado", &e.curso, nil)
			sqlPedido(t, pool, fmt.Sprintf(`DROP TRIGGER fallo_pedido ON %s`, tabla))
			sqlPedido(t, pool, `DROP FUNCTION fallo_pedido()`)
			if err == nil || res != nil {
				t.Fatalf("fallo SQL no informado: %+v %v", res, err)
			}
			verificarPedidoPendiente(t, pool, e)
			res, err = e.svc.ResolverPedido(ctx, e.pedido, e.admin, true, "Aprobado", &e.curso, nil)
			if err != nil || res.MateriaCreadaID == nil || e.avisos.Load() != 1 {
				t.Fatalf("reintento: %+v %v", res, err)
			}
			m, err := e.repo.BuscarMateriaPorID(ctx, *res.MateriaCreadaID)
			if err != nil || m.CursoID != e.curso || m.Nombre != "Robótica" {
				t.Fatalf("vínculo incorrecto: %+v %v", m, err)
			}
			persistido, err := e.repo.BuscarPedidoPorID(ctx, e.pedido)
			if err != nil || !reflect.DeepEqual(res, persistido) || persistido.MateriaSolicitada != "Robótica" || !persistido.EsMateriaNueva() {
				t.Fatalf("constancia incorrecta: %+v %v", persistido, err)
			}
			listado, err := e.repo.ListarPedidosDeUsuario(ctx, e.docente)
			if err != nil || len(listado) != 1 || listado[0].MateriaNombre != "Robótica" || listado[0].CursoNombre == "" || listado[0].Pedido.MateriaCreadaID == nil {
				t.Fatalf("listado sin vínculo: %+v %v", listado, err)
			}
			listado, err = e.repo.ListarPedidos(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			var encontrado bool
			for _, d := range listado {
				if d.Pedido.ID == e.pedido {
					encontrado = d.MateriaNombre == "Robótica" && d.Pedido.MateriaCreadaID != nil
				}
			}
			if !encontrado {
				t.Fatal("el listado de administradores perdió el vínculo")
			}
			rol := domain.RolSuplente
			repetido, err := e.svc.ResolverPedido(ctx, e.pedido, insertarUsuarioDeTest(t, pool, "APROBADA"), true, "Otra respuesta", nil, &rol)
			if err != nil || !reflect.DeepEqual(res, repetido) || e.avisos.Load() != 1 {
				t.Fatalf("respuesta perdida duplicó la operación: %+v %v", repetido, err)
			}
			asignados, err := e.repo.ListarDocentesDeMateria(ctx, m.ID)
			if err != nil || len(asignados) != 1 || asignados[0].UsuarioID != e.docente || asignados[0].Rol != domain.RolTitular {
				t.Fatalf("asignación duplicada o alterada: %+v %v", asignados, err)
			}
			obsoleta := *res
			obsoleta.Estado = domain.PedidoRechazado
			if err := e.repo.GuardarPedido(ctx, &obsoleta); !errors.Is(err, domain.ErrPedidoResuelto) {
				t.Fatalf("escritura obsoleta aceptada: %v", err)
			}
		})
	}
	t.Run("respuesta-invalida", func(t *testing.T) {
		e := nuevoEscenarioPedido(t, pool, true)
		if _, err := e.svc.ResolverPedido(ctx, e.pedido, e.admin, true, strings.Repeat("a", 1001), &e.curso, nil); !errors.Is(err, domain.ErrRespuestaLarga) {
			t.Fatal(err)
		}
		verificarPedidoPendiente(t, pool, e)
	})
	t.Run("usuario-no-aprobado", func(t *testing.T) {
		e := nuevoEscenarioPedido(t, pool, true)
		sqlPedido(t, pool, `UPDATE usuario SET estado='BAJA' WHERE id=$1`, e.docente)
		if _, err := e.svc.ResolverPedido(ctx, e.pedido, e.admin, true, "", &e.curso, nil); !errors.Is(err, application.ErrUsuarioNoValidoParaAsignar) {
			t.Fatal(err)
		}
		verificarPedidoPendiente(t, pool, e)
	})
	t.Run("materia-existente", func(t *testing.T) {
		e := nuevoEscenarioPedido(t, pool, false)
		p, _ := e.repo.BuscarPedidoPorID(ctx, e.pedido)
		dm := domain.NuevoDocenteMateria(uuidNuevo(), e.admin, *p.MateriaID, domain.RolTitular)
		if err := e.repo.AsignarDocente(ctx, dm); err != nil {
			t.Fatal(err)
		}
		res, err := e.svc.ResolverPedido(ctx, e.pedido, e.admin, true, "", nil, nil)
		if err != nil || res.MateriaID == nil || res.MateriaCreadaID != nil {
			t.Fatalf("materia existente: %+v %v", res, err)
		}
		asignados, err := e.repo.ListarDocentesDeMateria(ctx, *p.MateriaID)
		if err != nil || len(asignados) != 2 {
			t.Fatalf("asignaciones: %+v %v", asignados, err)
		}
		for _, dm := range asignados {
			if dm.UsuarioID == e.docente && dm.Rol != domain.RolSuplente {
				t.Fatal("perdió el rol por defecto")
			}
		}
	})
}

// La primera resolución se pausa después de sus escrituras, antes del commit.
// La otra operación debe esperar el pedido y leer la decisión confirmada.
type repoPedidoEnPausa struct {
	*PostgresRepo
	escrito, continuar chan struct{}
	unaVez             sync.Once
}

func (r *repoPedidoEnPausa) GuardarPedido(ctx context.Context, p *domain.PedidoDeMateria) error {
	if err := r.PostgresRepo.GuardarPedido(ctx, p); err != nil {
		return err
	}
	r.unaVez.Do(func() { close(r.escrito) })
	select {
	case <-r.continuar:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPedidosConsistencia_ResolucionesSimultaneas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	for _, primera := range []bool{true, false} {
		for _, segunda := range []bool{true, false} {
			t.Run(fmt.Sprintf("%v-%v", primera, segunda), func(t *testing.T) {
				e := nuevoEscenarioPedido(t, pool, true)
				ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancelar()
				pausa := &repoPedidoEnPausa{PostgresRepo: e.repo, escrito: make(chan struct{}), continuar: make(chan struct{})}
				bus := eventbus.NewInMemoryEventBus()
				bus.Subscribe("materia.pedido.resuelto", func(eventbus.Evento) { e.avisos.Add(1) })
				svc := application.NewService(pausa, NewValidadorUsuarioPostgres(pool), nil, nil, nil, NewDatosDeUsuarioPostgres(pool), nil, uuidNuevo, time.Now, bus)
				type respuesta struct {
					p   *domain.PedidoDeMateria
					err error
				}
				a, b := make(chan respuesta, 1), make(chan respuesta, 1)
				go func() {
					p, err := svc.ResolverPedido(ctx, e.pedido, e.admin, primera, "Primera constancia", &e.curso, nil)
					a <- respuesta{p, err}
				}()
				select {
				case <-pausa.escrito:
				case <-ctx.Done():
					t.Fatal("la primera operación no llegó a guardar")
				}
				go func() {
					p, err := e.svc.ResolverPedido(ctx, e.pedido, e.admin, segunda, "Otra constancia", &e.curso, nil)
					b <- respuesta{p, err}
				}()
				bloqueada := false
				for !bloqueada && ctx.Err() == nil {
					// H-14 coordina antes de bloquear el pedido: la segunda
					// resolución espera aquí, sin adquirir todavía ninguna fila.
					if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%pg_advisory_xact_lock%')`).Scan(&bloqueada); err != nil {
						t.Fatal(err)
					}
					select {
					case temprana := <-b:
						t.Fatalf("la segunda resolución no esperó: %+v", temprana)
					default:
					}
					if !bloqueada {
						time.Sleep(10 * time.Millisecond)
					}
				}
				if !bloqueada {
					t.Fatal("no se observó la coordinación de las resoluciones")
				}
				close(pausa.continuar)
				r1, r2 := <-a, <-b
				if r1.err != nil {
					t.Fatal(r1.err)
				}
				if primera == segunda {
					if r2.err != nil || !reflect.DeepEqual(r1.p, r2.p) {
						t.Fatalf("reintento simultáneo: %+v %+v", r1, r2)
					}
				} else if !errors.Is(r2.err, domain.ErrPedidoResuelto) || r2.p != nil {
					t.Fatalf("decisión contraria aceptada: %+v", r2)
				}
				guardado, err := e.repo.BuscarPedidoPorID(ctx, e.pedido)
				if err != nil || !reflect.DeepEqual(guardado, r1.p) || e.avisos.Load() != 1 {
					t.Fatalf("constancia sobrescrita o avisos duplicados: %+v %v avisos=%d", guardado, err, e.avisos.Load())
				}
				var materias, asignaciones int
				if err := pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM materia WHERE curso_id=$1),(SELECT COUNT(*) FROM docente_materia WHERE usuario_id=$2)`, e.curso, e.docente).Scan(&materias, &asignaciones); err != nil {
					t.Fatal(err)
				}
				esperadas := 0
				if primera {
					esperadas = 1
				}
				if materias != esperadas || asignaciones != esperadas {
					t.Fatalf("efectos incompatibles con la decisión: %d %d", materias, asignaciones)
				}
			})
		}
	}
}
