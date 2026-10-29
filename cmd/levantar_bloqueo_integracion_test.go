//go:build integration

package main

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	app "github.com/agustin-y2k/sgrc/internal/reservation/application"
	domain "github.com/agustin-y2k/sgrc/internal/reservation/domain"
	infra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
)

type escenarioLevantar struct {
	e                escenarioInstitucional
	actor, otraActor string
	filas, otras     []*domain.Reserva
}

func prepararLevantamiento(t *testing.T, pool *pgxpool.Pool) escenarioLevantar {
	t.Helper()
	e := prepararInstitucion(t, pool)
	actor, otro := uuid.NewString(), uuid.NewString()
	for _, id := range []string{actor, otro} {
		ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Admin','De prueba',$2,'hash','ADMIN','APROBADA')`, id, id+"@escuela.test")
	}
	pc2 := equipoDePrioridadCustodia(t, pool, "PC 2")
	pc3 := equipoDePrioridadCustodia(t, pool, "PC 3")
	fecha := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	creado, err := e.reservas.BloquearEquipos(context.Background(), []string{e.d.equipo, pc2}, &actor, fecha, 8*time.Hour, 9*time.Hour, "Evaluación")
	if err != nil {
		t.Fatal(err)
	}
	otroBloqueo, err := e.reservas.BloquearEquipos(context.Background(), []string{pc3}, &actor, fecha, 8*time.Hour, 9*time.Hour, "Otra evaluación")
	if err != nil {
		t.Fatal(err)
	}
	return escenarioLevantar{e, actor, otro, creado.Bloqueos, otroBloqueo.Bloqueos}
}
func (s escenarioLevantar) params() app.LevantarBloqueoParams {
	return app.LevantarBloqueoParams{OperacionID: *s.filas[0].BloqueoOperacionID, SolicitanteID: s.actor, EsAdmin: true, Motivo: "Terminó la evaluación", IPOrigen: "127.0.0.1"}
}
func contarAuditLevantamiento(t *testing.T, pool *pgxpool.Pool, op string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE accion='BLOQUEO_LEVANTADO' AND entidad_id=$1`, op).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLevantarBloqueo_PostgresAtomicoIdempotenteYAislado(t *testing.T) {
	pool := baseVacia(t)
	s := prepararLevantamiento(t, pool)
	ctx := context.Background()
	p := s.params()
	res, err := s.e.reservas.LevantarBloqueo(ctx, p)
	if err != nil || res.ReservasCanceladas != 2 || res.ReservasYaCanceladas != 0 {
		t.Fatalf("levantamiento: %+v %v", res, err)
	}
	var originales, ajenas int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM reserva WHERE id=$1 AND estado='CANCELADA'),(SELECT count(*) FROM reserva WHERE bloqueo_operacion_id=$2 AND estado='CONFIRMADA')`, s.e.reservaID, *s.otras[0].BloqueoOperacionID).Scan(&originales, &ajenas); err != nil || originales != 1 || ajenas != 1 {
		t.Fatalf("alteró la cascada anterior u otra operación: %d %d %v", originales, ajenas, err)
	}
	antes := huellasBase(t, ctx, pool)
	p.SolicitanteID = s.otraActor
	p.Motivo = "Reintento desde otro administrador"
	res, err = s.e.reservas.LevantarBloqueo(ctx, p)
	if err != nil || res.ReservasCanceladas != 0 || res.ReservasYaCanceladas != 2 || !reflect.DeepEqual(antes, huellasBase(t, ctx, pool)) {
		t.Fatalf("el reintento reescribió el estado o la auditoría: %+v %v", res, err)
	}
	if n := contarAuditLevantamiento(t, pool, p.OperacionID); n != 1 {
		t.Fatalf("auditorías duplicadas: %d", n)
	}
}
func TestLevantarBloqueo_PostgresCompletaUnaOperacionParcialAnterior(t *testing.T) {
	pool := baseVacia(t)
	s := prepararLevantamiento(t, pool)
	ctx := context.Background()
	if err := s.e.reservas.CancelarReserva(ctx, s.filas[0].ID, s.actor, true, "Cancelación previa"); err != nil {
		t.Fatal(err)
	}
	var antes, despues string
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(r)::text FROM reserva r WHERE id=$1`, s.filas[0].ID).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	res, err := s.e.reservas.LevantarBloqueo(ctx, s.params())
	if err != nil || res.ReservasCanceladas != 1 || res.ReservasYaCanceladas != 1 {
		t.Fatalf("no completó el lote: %+v %v", res, err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(r)::text FROM reserva r WHERE id=$1`, s.filas[0].ID).Scan(&despues); err != nil || antes != despues {
		t.Fatal("se reescribió la cancelación previa")
	}
}
func TestLevantarBloqueo_PostgresRollbackDeEscriturasYAuditoria(t *testing.T) {
	for _, etapa := range []string{"segunda-fila", "auditoria"} {
		t.Run(etapa, func(t *testing.T) {
			pool := baseVacia(t)
			s := prepararLevantamiento(t, pool)
			ctx := context.Background()
			tabla, condicion := "audit_log", "NEW.accion='BLOQUEO_LEVANTADO'"
			if etapa == "segunda-fila" {
				ids := []string{s.filas[0].ID, s.filas[1].ID}
				sort.Strings(ids)
				tabla = "reserva"
				condicion = "NEW.id='" + ids[1] + "' AND NEW.estado='CANCELADA'"
			}
			ejecutarCierreSQL(t, pool, `CREATE FUNCTION fallar_levantamiento() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF `+condicion+` THEN RAISE EXCEPTION 'fallo de prueba'; END IF; RETURN NEW; END $$`)
			ejecutarCierreSQL(t, pool, `CREATE TRIGGER fallar_levantamiento BEFORE INSERT OR UPDATE ON `+tabla+` FOR EACH ROW EXECUTE FUNCTION fallar_levantamiento()`)
			antes := huellasBase(t, ctx, pool)
			res, err := s.e.reservas.LevantarBloqueo(ctx, s.params())
			if err == nil || res != nil || !reflect.DeepEqual(antes, huellasBase(t, ctx, pool)) {
				t.Fatalf("quedaron escrituras parciales: %+v %v", res, err)
			}
			ejecutarCierreSQL(t, pool, `DROP TRIGGER fallar_levantamiento ON `+tabla)
			res, err = s.e.reservas.LevantarBloqueo(ctx, s.params())
			if err != nil || res.ReservasCanceladas != 2 {
				t.Fatalf("falló el reintento: %+v %v", res, err)
			}
		})
	}
}
func TestLevantarBloqueo_PostgresPermisosIdentidadYEstadosTerminales(t *testing.T) {
	pool := baseVacia(t)
	s := prepararLevantamiento(t, pool)
	ctx := context.Background()
	p := s.params()
	antes := huellasBase(t, ctx, pool)
	for _, tc := range []struct {
		nombre string
		p      app.LevantarBloqueoParams
		err    error
	}{
		{"docente", app.LevantarBloqueoParams{OperacionID: p.OperacionID, SolicitanteID: s.e.d.docente, Motivo: "motivo"}, app.ErrNoEsTuReserva},
		{"otro-admin-sin-motivo", app.LevantarBloqueoParams{OperacionID: p.OperacionID, SolicitanteID: s.otraActor, EsAdmin: true}, app.ErrMotivoObligatorio},
		{"identificador-invalido", app.LevantarBloqueoParams{OperacionID: "invalido", SolicitanteID: s.actor, EsAdmin: true}, app.ErrIDInvalido},
		{"operacion-ausente", app.LevantarBloqueoParams{OperacionID: uuid.NewString(), SolicitanteID: s.actor, EsAdmin: true}, app.ErrBloqueoNoEncontrado},
		{"reserva-normal", app.LevantarBloqueoParams{OperacionID: s.e.reservaID, SolicitanteID: s.actor, EsAdmin: true}, app.ErrBloqueoNoEncontrado},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			if _, err := s.e.reservas.LevantarBloqueo(ctx, tc.p); !errors.Is(err, tc.err) {
				t.Fatalf("error=%v esperado=%v", err, tc.err)
			}
			if !reflect.DeepEqual(antes, huellasBase(t, ctx, pool)) {
				t.Fatal("el rechazo dejó cambios")
			}
		})
	}
	ejecutarCierreSQL(t, pool, `UPDATE reserva SET estado='FINALIZADA' WHERE id=$1`, s.filas[0].ID)
	res, err := s.e.reservas.LevantarBloqueo(ctx, p)
	if err != nil || res.ReservasCanceladas != 1 || res.ReservasNoVigentes != 1 {
		t.Fatalf("no respetó el estado terminal: %+v %v", res, err)
	}
}

type levantamientoEnPausa struct {
	app.Repo
	pausa *pausaInstitucional
}

func (r levantamientoEnPausa) EnTransaccion(ctx context.Context, fn func(app.Repo) error) error {
	return r.Repo.EnTransaccion(ctx, func(repo app.Repo) error { return fn(levantamientoEnPausa{repo, r.pausa}) })
}
func (r levantamientoEnPausa) GuardarReserva(ctx context.Context, reserva *domain.Reserva) error {
	if err := r.Repo.GuardarReserva(ctx, reserva); err != nil {
		return err
	}
	return r.pausa.detener(ctx)
}
func TestLevantarBloqueo_PostgresReintentosConcurrentes(t *testing.T) {
	pool := baseVacia(t)
	s := prepararLevantamiento(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pausa := nuevaPausaInstitucional(t)
	svc := app.NewService(levantamientoEnPausa{infra.NewPostgresRepo(pool), pausa}, nil, nil, nil, nil, uuid.NewString, time.Now, s.e.bus)
	type respuesta struct {
		r   *app.ResultadoLevantarBloqueo
		err error
	}
	primero, segundo := make(chan respuesta, 1), make(chan respuesta, 1)
	go func() { r, err := svc.LevantarBloqueo(ctx, s.params()); primero <- respuesta{r, err} }()
	pausa.esperar(t, ctx)
	go func() { r, err := s.e.reservas.LevantarBloqueo(ctx, s.params()); segundo <- respuesta{r, err} }()
	limite := time.After(3 * time.Second)
	for {
		var espera bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%bloqueo_operacion_id=$1 ORDER BY id FOR UPDATE%')`).Scan(&espera); err != nil {
			t.Fatal(err)
		}
		if espera {
			break
		}
		select {
		case <-limite:
			t.Fatal("el reintento no esperó el bloqueo de filas")
		case <-time.After(10 * time.Millisecond):
		}
	}
	close(pausa.continuar)
	a, b := <-primero, <-segundo
	if a.err != nil || b.err != nil || a.r.ReservasCanceladas != 2 || b.r.ReservasCanceladas != 0 || b.r.ReservasYaCanceladas != 2 {
		t.Fatalf("reintentos: %+v %v / %+v %v", a.r, a.err, b.r, b.err)
	}
	if n := contarAuditLevantamiento(t, pool, s.params().OperacionID); n != 1 {
		t.Fatalf("auditorías=%d", n)
	}
}

func TestLevantarBloqueo_PostgresEventoDurableYConexionUnica(t *testing.T) {
	pool := baseVacia(t)
	s := prepararLevantamiento(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cfg := pool.Config()
	cfg.MaxConns = 1
	unico, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer unico.Close()
	bus := busDurableReal(t, unico)
	svc := app.NewService(infra.NewPostgresRepo(unico), nil, infra.NewValidadorEquipoPostgres(unico), nil, infra.NewObtenedorNombrePostgres(unico), uuid.NewString, time.Now, bus)
	p := s.params()
	p.SolicitanteID = s.otraActor
	ejecutarCierreSQL(t, pool, `CREATE FUNCTION impedir_aviso_bloqueo() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fallo de evento durable'; END $$`)
	ejecutarCierreSQL(t, pool, `CREATE TRIGGER impedir_aviso_bloqueo BEFORE INSERT ON evento_pendiente FOR EACH ROW EXECUTE FUNCTION impedir_aviso_bloqueo()`)
	antes := huellasBase(t, ctx, pool)
	res, err := svc.LevantarBloqueo(ctx, p)
	if err == nil || res != nil || !reflect.DeepEqual(antes, huellasBase(t, ctx, pool)) {
		t.Fatalf("el fallo del evento dejó cambios: %+v %v", res, err)
	}
	ejecutarCierreSQL(t, pool, `DROP TRIGGER impedir_aviso_bloqueo ON evento_pendiente`)
	res, err = svc.LevantarBloqueo(ctx, p)
	if err != nil || res.ReservasCanceladas != 2 {
		t.Fatalf("no completó con una conexión: %+v %v", res, err)
	}
	var eventos int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evento_pendiente WHERE tipo='reserva.cancelada'`).Scan(&eventos); err != nil || eventos != 1 {
		t.Fatalf("eventos=%d error=%v", eventos, err)
	}
	res, err = svc.LevantarBloqueo(ctx, p)
	if err != nil || res.ReservasCanceladas != 0 || contarAuditLevantamiento(t, pool, p.OperacionID) != 1 {
		t.Fatalf("repetición durable: %+v %v", res, err)
	}
	var despues int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evento_pendiente WHERE tipo='reserva.cancelada'`).Scan(&despues); err != nil || despues != eventos {
		t.Fatal("duplicó el aviso al reintentar")
	}
}
