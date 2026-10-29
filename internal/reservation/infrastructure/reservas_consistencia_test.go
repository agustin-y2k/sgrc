//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/jackc/pgx/v5/pgxpool"
)

func servicioCoordinado(pool *pgxpool.Pool, repo application.Repo, ahora time.Time) *application.Service {
	return application.NewService(repo, NewValidadorMateriaPostgres(pool), NewValidadorEquipoPostgres(pool), jornadaCoordinada{}, NewObtenedorNombrePostgres(pool), NuevoID, func() time.Time { return ahora }, eventbus.NewInMemoryEventBus())
}

type jornadaCoordinada struct{}

func (jornadaCoordinada) PermiteReserva(context.Context, time.Time, time.Duration, time.Duration) (bool, error) {
	return true, nil
}
func (jornadaCoordinada) CierreDeLaJornada(context.Context, time.Time) (application.CierreDeJornada, error) {
	return application.CierreDeJornada{Declarada: true, Abre: true, Fin: 18 * time.Hour}, nil
}

type pausaDeReservas struct {
	application.Repo
	antes         bool
	lista, seguir chan struct{}
	unaVez        *sync.Once
}

func nuevaPausa(repo application.Repo, antes bool) *pausaDeReservas {
	return &pausaDeReservas{Repo: repo, antes: antes, lista: make(chan struct{}), seguir: make(chan struct{}), unaVez: &sync.Once{}}
}
func (r *pausaDeReservas) EnTransaccion(ctx context.Context, fn func(application.Repo) error) error {
	return r.Repo.EnTransaccion(ctx, func(repo application.Repo) error {
		copia := *r
		copia.Repo = repo
		return fn(&copia)
	})
}
func (r *pausaDeReservas) pausar(ctx context.Context) error {
	var err error
	r.unaVez.Do(func() {
		close(r.lista)
		select {
		case <-r.seguir:
		case <-ctx.Done():
			err = ctx.Err()
		}
	})
	return err
}
func (r *pausaDeReservas) BuscarReservasParaActualizar(ctx context.Context, ids []string) (map[string]*domain.Reserva, error) {
	if r.antes {
		if err := r.pausar(ctx); err != nil {
			return nil, err
		}
	}
	reservas, err := r.Repo.BuscarReservasParaActualizar(ctx, ids)
	if err != nil {
		return nil, err
	}
	if !r.antes {
		if err := r.pausar(ctx); err != nil {
			return nil, err
		}
	}
	return reservas, nil
}
func (r *pausaDeReservas) ReservasAVigilar(ctx context.Context, hoy time.Time) ([]application.ReservaParaVigilar, error) {
	reservas, err := r.Repo.ReservasAVigilar(ctx, hoy)
	if err != nil {
		return nil, err
	}
	if err := r.pausar(ctx); err != nil {
		return nil, err
	}
	return reservas, nil
}

// El barrido ahora coordina la pasada completa. La entrega concurrente debe
// confirmar antes de ese bloqueo; después de adquirirlo debe esperar al barrido.
type pausaAntesDeCoordinar struct{ *pausaDeReservas }

func (r *pausaAntesDeCoordinar) Coordinar(ctx context.Context, exclusiva bool, fn func(context.Context) error) error {
	if err := r.pausar(ctx); err != nil {
		return err
	}
	return r.Repo.Coordinar(ctx, exclusiva, fn)
}

func esperarPausa(t *testing.T, ctx context.Context, r *pausaDeReservas) {
	t.Helper()
	t.Cleanup(func() {
		select {
		case <-r.seguir:
		default:
			close(r.seguir)
		}
	})
	select {
	case <-r.lista:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
func verificarGrupo(t *testing.T, l loteDePrestamos, estado domain.EstadoReservaGrupo) {
	t.Helper()
	g, err := l.repo.BuscarReservaGrupoPorID(context.Background(), l.grupo)
	if err != nil || g.Estado != estado {
		t.Fatalf("grupo: %+v %v; esperaba %s", g, err, estado)
	}
}

type mostradorCoordinado struct{}

func (mostradorCoordinado) MostradorEn(context.Context, time.Time) (application.MostradorAtendido, error) {
	return application.MostradorAtendido{Atendido: true, Declarado: true}, nil
}
func (mostradorCoordinado) MostradorEseDia(context.Context, time.Time) (application.MostradorAtendido, error) {
	return application.MostradorAtendido{Atendido: true, Declarado: true}, nil
}

func TestReservasConsistencia_IntercalacionesYVersiones(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	adminID := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelar()
	crear := func(t *testing.T) loteDePrestamos {
		sqlDePrestamos(t, pool, `TRUNCATE reserva_grupo,reserva,prestamo CASCADE`)
		return loteDePrestamosReal(t, pool)
	}
	for _, paso := range []string{"cambio", "finalizacion", "cascada"} {
		t.Run("cancelacion-antes-de-"+paso, func(t *testing.T) {
			l := crear(t)
			pausa := nuevaPausa(l.repo, true)
			nuevo := crearEquipoDeCarroDeTest(t, pool)
			ahora := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
			svc := servicioCoordinado(pool, pausa, ahora)
			fin := make(chan error, 1)
			go func() {
				switch paso {
				case "cambio":
					_, err := svc.CambiarEquipoDeReserva(ctx, l.reservas[0], nuevo, adminID, true, true)
					fin <- err
				case "finalizacion":
					_, err := svc.FinalizarVencidas(ctx)
					fin <- err
				case "cascada":
					_, err := svc.CancelarReservasPorIDs(ctx, l.reservas, "cascada")
					fin <- err
				}
			}()
			esperarPausa(t, ctx, pausa)
			if err := servicioCoordinado(pool, l.repo, ahora).CancelarReserva(ctx, l.reservas[0], adminID, true, "manual"); err != nil {
				t.Fatal(err)
			}
			close(pausa.seguir)
			err := <-fin
			if paso == "cambio" {
				if !errors.Is(err, application.ErrReservaNoModificable) {
					t.Fatalf("cambio tras cancelación: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			r, err := l.repo.BuscarReservaPorID(ctx, l.reservas[0])
			if err != nil || r.Estado != domain.ReservaCancelada || r.EquipoID != l.equipos[0] || r.MotivoCancelacion == nil || *r.MotivoCancelacion != "manual" {
				t.Fatalf("se sobrescribió la cancelación: %+v %v", r, err)
			}
			esperado := domain.GrupoParcialmenteCancelada
			if paso == "finalizacion" {
				esperado = domain.GrupoFinalizada
			}
			if paso == "cascada" {
				esperado = domain.GrupoCancelada
			}
			verificarGrupo(t, l, esperado)
		})
	}
	t.Run("cambio-antes-de-cancelacion-conserva-el-equipo", func(t *testing.T) {
		l := crear(t)
		pausa := nuevaPausa(l.repo, true)
		fin := make(chan error, 1)
		go func() {
			fin <- servicioCoordinado(pool, pausa, ahoraDeTest()).CancelarReserva(ctx, l.reservas[0], adminID, true, "manual")
		}()
		esperarPausa(t, ctx, pausa)
		nuevo := crearEquipoDeCarroDeTest(t, pool)
		if _, err := l.svc.CambiarEquipoDeReserva(ctx, l.reservas[0], nuevo, adminID, true, true); err != nil {
			t.Fatal(err)
		}
		close(pausa.seguir)
		if err := <-fin; err != nil {
			t.Fatal(err)
		}
		r, err := l.repo.BuscarReservaPorID(ctx, l.reservas[0])
		if err != nil || r.Estado != domain.ReservaCancelada || r.EquipoID != nuevo {
			t.Fatalf("cancelación restauró el equipo viejo: %+v %v", r, err)
		}
	})
	t.Run("cascada-no-cancela-la-reserva-que-cambio-de-equipo", func(t *testing.T) {
		l := crear(t)
		pausa := nuevaPausa(l.repo, true)
		fin := make(chan error, 1)
		go func() {
			n, _, err := servicioCoordinado(pool, pausa, time.Date(2026, 3, 2, 7, 0, 0, 0, time.UTC)).CancelarReservasFuturasDeEquipo(ctx, l.equipos[0], "mantenimiento")
			if err == nil && n != 0 {
				err = fmt.Errorf("canceló %d reservas que ya no usan el equipo", n)
			}
			fin <- err
		}()
		esperarPausa(t, ctx, pausa)
		nuevo := crearEquipoDeCarroDeTest(t, pool)
		if _, err := l.svc.CambiarEquipoDeReserva(ctx, l.reservas[0], nuevo, adminID, true, true); err != nil {
			t.Fatal(err)
		}
		close(pausa.seguir)
		if err := <-fin; err != nil {
			t.Fatal(err)
		}
		r, err := l.repo.BuscarReservaPorID(ctx, l.reservas[0])
		if err != nil || r.Estado != domain.ReservaConfirmada || r.EquipoID != nuevo {
			t.Fatalf("canceló datos obsoletos: %+v %v", r, err)
		}
	})
	t.Run("cancelaciones-de-hermanas-esperan-al-grupo", func(t *testing.T) {
		l := crear(t)
		pausa := nuevaPausa(l.repo, false)
		fin := make(chan error, 1)
		go func() {
			fin <- servicioCoordinado(pool, pausa, ahoraDeTest()).CancelarReserva(ctx, l.reservas[0], adminID, true, "primera")
		}()
		esperarPausa(t, ctx, pausa)
		segunda := make(chan error, 1)
		go func() { _, err := l.svc.CancelarReservasPorIDs(ctx, l.reservas[1:], "segunda"); segunda <- err }()
		bloqueada := false
		for !bloqueada && ctx.Err() == nil {
			if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%reserva_grupo%FOR UPDATE%')`).Scan(&bloqueada); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-segunda:
				t.Fatalf("no esperó al grupo: %v", err)
			default:
			}
			if !bloqueada {
				time.Sleep(10 * time.Millisecond)
			}
		}
		if !bloqueada {
			t.Fatal("no se observó el bloqueo")
		}
		close(pausa.seguir)
		if err := <-fin; err != nil {
			t.Fatal(err)
		}
		if err := <-segunda; err != nil {
			t.Fatal(err)
		}
		verificarGrupo(t, l, domain.GrupoCancelada)
	})
	t.Run("escrituras-obsoletas-se-rechazan", func(t *testing.T) {
		l := crear(t)
		vieja, err := l.repo.BuscarReservaPorID(ctx, l.reservas[0])
		if err != nil {
			t.Fatal(err)
		}
		grupoViejo, err := l.repo.BuscarReservaGrupoPorID(ctx, l.grupo)
		if err != nil {
			t.Fatal(err)
		}
		if err := l.svc.CancelarReserva(ctx, vieja.ID, adminID, true, "vigente"); err != nil {
			t.Fatal(err)
		}
		vieja.EquipoID = l.equipos[1]
		if err := l.repo.GuardarReserva(ctx, vieja); !errors.Is(err, application.ErrReservaModificada) {
			t.Fatalf("guardó lectura vieja: %v", err)
		}
		grupoViejo.Estado = domain.GrupoCancelada
		if err := l.repo.GuardarReservaGrupo(ctx, grupoViejo); !errors.Is(err, application.ErrReservaModificada) {
			t.Fatalf("guardó grupo viejo: %v", err)
		}
		verificarGrupo(t, l, domain.GrupoParcialmenteCancelada)
	})
	t.Run("solapamiento-en-update-es-conflicto-de-negocio", func(t *testing.T) {
		l := crear(t)
		r, err := l.repo.BuscarReservaPorID(ctx, l.reservas[0])
		if err != nil {
			t.Fatal(err)
		}
		r.EquipoID = l.equipos[1]
		if err := l.repo.GuardarReserva(ctx, r); !errors.Is(err, application.ErrSolapamiento) {
			t.Fatalf("solapamiento salió como error interno: %v", err)
		}
		persistida, err := l.repo.BuscarReservaPorID(ctx, r.ID)
		if err != nil || persistida.Version != 0 || persistida.EquipoID != l.equipos[0] {
			t.Fatalf("actualización fallida dejó cambios: %+v %v", persistida, err)
		}
	})
	t.Run("cambio-de-equipo-con-pool-de-una-conexion", func(t *testing.T) {
		l := crear(t)
		cfg := pool.Config()
		cfg.MaxConns = 1
		cfg.MinConns = 0
		unico, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer unico.Close()
		ctxCambio, cancelarCambio := context.WithTimeout(ctx, 3*time.Second)
		defer cancelarCambio()
		nuevo := crearEquipoDeCarroDeTest(t, pool)
		res, err := servicioCoordinado(unico, NewPostgresRepo(unico), ahoraDeTest()).CambiarEquipoDeReserva(ctxCambio, l.reservas[0], nuevo, adminID, true, true)
		if err != nil || res.EquipoID != nuevo {
			t.Fatalf("pidió otra conexión durante el cambio: %+v %v", res, err)
		}
	})
	t.Run("desplazamiento-no-cancela-un-conflicto-que-ya-se-movio", func(t *testing.T) {
		l := crear(t)
		materia := materiaEnCursoDeTest(t, pool, 2026, "Dibujo Técnico", "3°B")
		marcarPreferencia(t, pool, l.equipos[0], "Dibujo Técnico", nil, nil, nil, 1)
		pausa := nuevaPausa(l.repo, true)
		fin := make(chan error, 1)
		ahora := time.Date(2026, 3, 2, 6, 0, 0, 0, time.UTC)
		go func() {
			_, _, impacto, err := servicioCoordinado(pool, &pausaAntesDeCoordinar{pausa}, ahora).CrearReserva(ctx, materia, adminID, true, diaDe(2026, 3, 2), 8*time.Hour, 9*time.Hour, l.equipos[:1], l.equipos[:1])
			if err == nil && impacto.ReservasCanceladas != 0 {
				err = fmt.Errorf("desplazó %d reservas que ya no estorbaban", impacto.ReservasCanceladas)
			}
			fin <- err
		}()
		esperarPausa(t, ctx, pausa)
		nuevo := crearEquipoDeCarroDeTest(t, pool)
		if _, err := l.svc.CambiarEquipoDeReserva(ctx, l.reservas[0], nuevo, adminID, true, true); err != nil {
			t.Fatal(err)
		}
		close(pausa.seguir)
		if err := <-fin; err != nil {
			t.Fatal(err)
		}
		r, err := l.repo.BuscarReservaPorID(ctx, l.reservas[0])
		if err != nil || r.Estado != domain.ReservaConfirmada || r.EquipoID != nuevo {
			t.Fatalf("canceló el conflicto anterior: %+v %v", r, err)
		}
	})
	t.Run("barrido-conserva-entrega-confirmada-antes-del-bloqueo", func(t *testing.T) {
		l := crear(t)
		usuario := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
		sqlDePrestamos(t, pool, `UPDATE reserva_grupo SET creado_por=$2 WHERE id=$1`, l.grupo, usuario)
		sqlDePrestamos(t, pool, `UPDATE reserva SET creado_por=$2 WHERE id=ANY($1)`, l.reservas, usuario)
		pausa := nuevaPausa(l.repo, true)
		ahora := time.Date(2026, 3, 2, 8, 50, 0, 0, time.UTC)
		v := application.NewVigilante(&pausaAntesDeCoordinar{pausa}, eventbus.NewInMemoryEventBus(), jornadaCoordinada{}, mostradorCoordinado{}, func() time.Time { return ahora }, application.ConfigDeVigilanciaPorDefecto())
		fin := make(chan error, 1)
		go func() {
			res, err := v.Barrer(ctx)
			if err == nil && res.Liberadas != 2 {
				err = fmt.Errorf("liberó %d; esperaba 2", res.Liberadas)
			}
			fin <- err
		}()
		esperarPausa(t, ctx, pausa)
		if _, err := l.svc.EntregarPorReserva(ctx, application.EntregaPorReservaParams{ReservaIDs: l.reservas[:1], EquiposEsperados: l.esperados(l.reservas[:1])}); err != nil {
			t.Fatal(err)
		}
		close(pausa.seguir)
		if err := <-fin; err != nil {
			t.Fatal(err)
		}
		r, err := l.repo.BuscarReservaPorID(ctx, l.reservas[0])
		if err != nil || r.Estado != domain.ReservaConfirmada {
			t.Fatalf("liberó reserva entregada: %+v %v", r, err)
		}
		verificarGrupo(t, l, domain.GrupoConfirmada)
	})
	t.Run("rollback-de-grupo-revierte-hijo", func(t *testing.T) {
		l := crear(t)
		fallarPrestamosSQL(t, pool, "reserva_grupo", "UPDATE", "id", l.grupo)
		if err := l.svc.CancelarReserva(ctx, l.reservas[0], adminID, true, "manual"); err == nil {
			t.Fatal("ocultó el fallo")
		}
		verificarLoteSinEscrituras(t, pool, l)
		verificarGrupo(t, l, domain.GrupoConfirmada)
	})
	for _, operacion := range []string{"entrega-reservada", "entrega-suelta", "devolucion"} {
		t.Run("reintento-no-duplica-respuesta-"+operacion, func(t *testing.T) {
			l := crear(t)
			var ids []string
			if operacion == "devolucion" {
				entrega, err := l.svc.EntregarPorReserva(ctx, application.EntregaPorReservaParams{ReservaIDs: l.reservas[:2], EquiposEsperados: l.esperados(l.reservas[:2])})
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range entrega.Entregadas {
					ids = append(ids, p.ID)
				}
			}
			// La secuencia no se revierte: falla la segunda fila del primer intento.
			sqlDePrestamos(t, pool, `CREATE SEQUENCE intento_del_lote`)
			sqlDePrestamos(t, pool, `CREATE FUNCTION reintento_del_lote() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF nextval('intento_del_lote')=2 THEN RAISE EXCEPTION 'deadlock inyectado' USING ERRCODE='40P01'; END IF; RETURN NEW; END $$`)
			verbo := "INSERT"
			if operacion == "devolucion" {
				verbo = "UPDATE"
			}
			sqlDePrestamos(t, pool, `CREATE TRIGGER reintento_del_lote BEFORE `+verbo+` ON prestamo FOR EACH ROW EXECUTE FUNCTION reintento_del_lote()`)
			t.Cleanup(func() {
				sqlDePrestamos(t, pool, `DROP TRIGGER reintento_del_lote ON prestamo`)
				sqlDePrestamos(t, pool, `DROP FUNCTION reintento_del_lote()`)
				sqlDePrestamos(t, pool, `DROP SEQUENCE intento_del_lote`)
			})
			if operacion == "devolucion" {
				res, err := l.svc.RecibirEquipos(ctx, ids, "", "recibido")
				if err != nil || len(res.Recibidos) != 2 || len(res.NoRecibidos) != 0 {
					t.Fatalf("respuesta de devolución: %+v %v", res, err)
				}
			} else {
				var res *application.ResultadoEntrega
				var err error
				if operacion == "entrega-reservada" {
					res, err = l.svc.EntregarPorReserva(ctx, application.EntregaPorReservaParams{ReservaIDs: l.reservas[:2], EquiposEsperados: l.esperados(l.reservas[:2])})
				} else {
					res, err = l.svc.EntregarSuelta(ctx, application.EntregaSueltaParams{EquipoIDs: l.equipos[:2], Nombre: "Ada Lovelace"})
				}
				if err != nil || len(res.Entregadas) != 2 || len(res.NoEntregadas) != 0 {
					t.Fatalf("respuesta de entrega: %+v %v", res, err)
				}
				for _, p := range res.Entregadas {
					ids = append(ids, p.ID)
				}
			}
			var persistidos int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM prestamo WHERE id=ANY($1)`, ids).Scan(&persistidos); err != nil || persistidos != 2 {
				t.Fatalf("la respuesta incluye escrituras revertidas: %d %v", persistidos, err)
			}
		})
	}
	t.Run("barrido-cierra-mezcla-cancelada-y-no-retirada", func(t *testing.T) {
		l := crear(t)
		sqlDePrestamos(t, pool, `UPDATE reserva_grupo SET creado_por=$2 WHERE id=$1`, l.grupo, adminID)
		sqlDePrestamos(t, pool, `UPDATE reserva SET creado_por=$2 WHERE id=ANY($1)`, l.reservas, adminID)
		ahora := time.Date(2026, 3, 2, 8, 50, 0, 0, time.UTC)
		if err := servicioCoordinado(pool, l.repo, ahora).CancelarReserva(ctx, l.reservas[0], adminID, true, "manual"); err != nil {
			t.Fatal(err)
		}
		v := application.NewVigilante(l.repo, eventbus.NewInMemoryEventBus(), jornadaCoordinada{}, mostradorCoordinado{}, func() time.Time { return ahora }, application.ConfigDeVigilanciaPorDefecto())
		res, err := v.Barrer(ctx)
		if err != nil || res.Liberadas != 2 {
			t.Fatalf("barrido: %+v %v", res, err)
		}
		verificarGrupo(t, l, domain.GrupoNoRetirado)
		if n, err := servicioCoordinado(pool, l.repo, ahora.Add(2*time.Hour)).FinalizarVencidas(ctx); err != nil || n != 0 {
			t.Fatalf("finalizó reservas no retiradas: %d %v", n, err)
		}
		verificarGrupo(t, l, domain.GrupoNoRetirado)
	})
}
