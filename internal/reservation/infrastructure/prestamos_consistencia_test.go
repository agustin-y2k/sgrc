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

type loteDePrestamos struct {
	svc               *application.Service
	repo              *PostgresRepo
	grupo             string
	reservas, equipos []string
}

func loteDePrestamosReal(t *testing.T, pool *pgxpool.Pool) loteDePrestamos {
	t.Helper()
	ctx := context.Background()
	repo := NewPostgresRepo(pool)
	materia := crearMateriaDeTest(t, pool, 2026)
	g := nuevoReservaGrupoDeTest(t, materia, diaDe(2026, 3, 2), 8*time.Hour, 9*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, g); err != nil {
		t.Fatal(err)
	}
	lote := loteDePrestamos{repo: repo, grupo: g.ID}
	for range 3 {
		equipo := crearEquipoDeCarroDeTest(t, pool)
		r, err := domain.NuevaReservaNormal(NuevoID(), g.ID, equipo, materia, "Ada Lovelace", nil, g.Fecha, g.HoraInicio, g.HoraFin, g.CreadaEn)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.CrearReserva(ctx, r); err != nil {
			t.Fatal(err)
		}
		lote.reservas = append(lote.reservas, r.ID)
		lote.equipos = append(lote.equipos, equipo)
	}
	lote.svc = application.NewService(repo, nil, NewValidadorEquipoPostgres(pool), nil, nil, NuevoID, func() time.Time { return time.Date(2026, 3, 2, 8, 30, 0, 0, time.UTC) }, eventbus.NewInMemoryEventBus())
	return lote
}

func sqlDePrestamos(t *testing.T, pool *pgxpool.Pool, consulta string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), consulta, args...); err != nil {
		t.Fatalf("SQL de prueba: %v", err)
	}
}

func verificarLoteSinEscrituras(t *testing.T, pool *pgxpool.Pool, l loteDePrestamos) {
	t.Helper()
	var prestamos, modificadas int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT COUNT(*) FROM prestamo WHERE reserva_id=ANY($1)),(SELECT COUNT(*) FROM reserva WHERE id=ANY($1) AND estado<>'CONFIRMADA')`, l.reservas).Scan(&prestamos, &modificadas); err != nil {
		t.Fatal(err)
	}
	if prestamos != 0 || modificadas != 0 {
		t.Fatalf("quedaron escrituras ocultas: préstamos=%d reservas=%d", prestamos, modificadas)
	}
}

func fallarPrestamosSQL(t *testing.T, pool *pgxpool.Pool, tabla, operacion, columna, id string) {
	t.Helper()
	sqlDePrestamos(t, pool, `CREATE FUNCTION fallo_del_lote() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fallo inyectado del lote'; END $$`)
	sqlDePrestamos(t, pool, fmt.Sprintf(`CREATE TRIGGER fallo_del_lote BEFORE %s ON %s FOR EACH ROW WHEN (NEW.%s='%s'::uuid) EXECUTE FUNCTION fallo_del_lote()`, operacion, tabla, columna, id))
	t.Cleanup(func() {
		sqlDePrestamos(t, pool, fmt.Sprintf(`DROP TRIGGER fallo_del_lote ON %s`, tabla))
		sqlDePrestamos(t, pool, `DROP FUNCTION fallo_del_lote()`)
	})
}

func TestPrestamosConsistencia_LotesAtomicosYParciales(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	ctx := context.Background()
	for _, devuelta := range []bool{false, true} {
		t.Run(fmt.Sprintf("segunda-tanda-devuelta-%v", devuelta), func(t *testing.T) {
			l := loteDePrestamosReal(t, pool)
			a, err := l.svc.EntregarPorReserva(ctx, application.EntregaPorReservaParams{ReservaIDs: l.reservas[:1], EquiposEsperados: l.esperados(l.reservas[:1])})
			if err != nil {
				t.Fatal(err)
			}
			if devuelta {
				if _, err := l.svc.RecibirEquipos(ctx, []string{a.Entregadas[0].ID}, "", ""); err != nil {
					t.Fatal(err)
				}
			}
			b, err := l.svc.EntregarPorReserva(ctx, application.EntregaPorReservaParams{ReservaIDs: l.reservas[1:2], EquiposEsperados: l.esperados(l.reservas[1:2]), LiberarNoEntregadas: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(b.Liberadas) != 1 || b.Liberadas[0] != l.reservas[2] {
				t.Fatalf("liberación incorrecta: %v", b.Liberadas)
			}
			for _, id := range l.reservas[:2] {
				r, err := l.repo.BuscarReservaPorID(ctx, id)
				if err != nil || r.Estado != domain.ReservaConfirmada {
					t.Fatalf("se liberó una reserva retirada: %+v %v", r, err)
				}
			}
		})
	}
	t.Run("conflicto-previsto-no-aborta-la-transaccion", func(t *testing.T) {
		l := loteDePrestamosReal(t, pool)
		crearPrestamoDeTest(t, l.repo, entregaDeTest(l.equipos[1]), ahoraDeTest())
		r, err := l.svc.EntregarPorReserva(ctx, application.EntregaPorReservaParams{ReservaIDs: l.reservas, EquiposEsperados: l.esperados(l.reservas)})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Entregadas) != 2 || len(r.NoEntregadas) != 1 || r.NoEntregadas[0].Razon != application.NoEntregadaYaPrestada {
			t.Fatalf("respuesta parcial incorrecta: %+v", r)
		}
		for _, p := range r.Entregadas {
			if _, err := l.repo.BuscarPrestamoPorID(ctx, p.ID); err != nil {
				t.Fatalf("la respuesta informó una entrega no confirmada: %v", err)
			}
		}
	})
	t.Run("colision-de-id-no-se-confunde-con-custodia", func(t *testing.T) {
		l := loteDePrestamosReal(t, pool)
		id := NuevoID()
		svc := application.NewService(l.repo, nil, NewValidadorEquipoPostgres(pool), nil, nil, func() string { return id }, l.svc.Ahora, eventbus.NewInMemoryEventBus())
		r, err := svc.EntregarSuelta(ctx, application.EntregaSueltaParams{EquipoIDs: l.equipos[:2], Nombre: "Ada"})
		if err == nil || r != nil {
			t.Fatalf("una colisión de identificador se ocultó como rechazo previsto: %+v %v", r, err)
		}
		for _, equipo := range l.equipos[:2] {
			if _, err := l.repo.BuscarPrestamoAbiertoDeEquipo(ctx, equipo); !errors.Is(err, application.ErrPrestamoNoEncontrado) {
				t.Fatalf("quedó un préstamo oculto: %v", err)
			}
		}
	})
	t.Run("reserva-inexistente", func(t *testing.T) {
		l := loteDePrestamosReal(t, pool)
		r, err := l.svc.EntregarPorReserva(ctx, application.EntregaPorReservaParams{ReservaIDs: []string{l.reservas[0], NuevoID()}, EquiposEsperados: l.esperados([]string{l.reservas[0], NuevoID()}), LiberarNoEntregadas: true})
		if !errors.Is(err, application.ErrReservaNoEncontrada) || r != nil {
			t.Fatalf("resultado: %+v %v", r, err)
		}
		verificarLoteSinEscrituras(t, pool, l)
	})
	for _, tipo := range []string{"reserva", "suelta", "liberacion"} {
		t.Run("fallo-"+tipo, func(t *testing.T) {
			l := loteDePrestamosReal(t, pool)
			if tipo == "liberacion" {
				fallarPrestamosSQL(t, pool, "reserva", "UPDATE", "id", l.reservas[2])
			} else {
				fallarPrestamosSQL(t, pool, "prestamo", "INSERT", "equipo_id", l.equipos[1])
			}
			var r *application.ResultadoEntrega
			var err error
			if tipo == "suelta" {
				r, err = l.svc.EntregarSuelta(ctx, application.EntregaSueltaParams{EquipoIDs: l.equipos, Nombre: "Ada"})
			} else {
				ids := l.reservas
				if tipo == "liberacion" {
					ids = ids[:1]
				}
				r, err = l.svc.EntregarPorReserva(ctx, application.EntregaPorReservaParams{ReservaIDs: ids, EquiposEsperados: l.esperados(ids), LiberarNoEntregadas: tipo == "liberacion"})
			}
			if err == nil || r != nil {
				t.Fatalf("resultado ante fallo: %+v %v", r, err)
			}
			verificarLoteSinEscrituras(t, pool, l)
			for _, id := range l.equipos {
				if _, err := l.repo.BuscarPrestamoAbiertoDeEquipo(ctx, id); !errors.Is(err, application.ErrPrestamoNoEncontrado) {
					t.Fatalf("quedó custodia oculta: %v", err)
				}
			}
		})
	}
	for _, tipo := range []string{"inexistente", "escritura"} {
		t.Run("devolucion-"+tipo, func(t *testing.T) {
			l := loteDePrestamosReal(t, pool)
			entrega, err := l.svc.EntregarSuelta(ctx, application.EntregaSueltaParams{EquipoIDs: l.equipos, Nombre: "Ada"})
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{entrega.Entregadas[0].ID, entrega.Entregadas[1].ID}
			if tipo == "inexistente" {
				ids[1] = NuevoID()
			} else {
				fallarPrestamosSQL(t, pool, "prestamo", "UPDATE", "id", ids[1])
			}
			var eventos int
			l.svc = application.NewService(l.repo, nil, NewValidadorEquipoPostgres(pool), nil, nil, NuevoID, ahoraDeTest, eventbus.NewInMemoryEventBus())
			bus := eventbus.NewInMemoryEventBus()
			bus.Subscribe("prestamo.cierre.pendientes", func(eventbus.Evento) { eventos++ })
			l.svc = application.NewService(l.repo, nil, NewValidadorEquipoPostgres(pool), nil, nil, NuevoID, ahoraDeTest, bus)
			r, err := l.svc.RecibirEquipos(ctx, ids, "", "fallo")
			if err == nil || r != nil {
				t.Fatalf("resultado ante fallo: %+v %v", r, err)
			}
			if eventos != 0 {
				t.Fatal("se publicó una devolución que no se confirmó")
			}
			for _, p := range entrega.Entregadas {
				guardado, err := l.repo.BuscarPrestamoPorID(ctx, p.ID)
				if err != nil || !guardado.EstaAbierto() {
					t.Fatalf("devolución parcial oculta: %+v %v", guardado, err)
				}
			}
		})
	}
	t.Run("grupo-recalculado-en-la-misma-transaccion", func(t *testing.T) {
		l := loteDePrestamosReal(t, pool)
		sqlDePrestamos(t, pool, `UPDATE reserva SET estado='FINALIZADA' WHERE id=$1`, l.reservas[0])
		// El equipo finalizado se conserva como antecedente; el retiro válido
		// corresponde a la segunda PC, liberada pero aún dentro de su franja.
		sqlDePrestamos(t, pool, `UPDATE reserva SET estado='NO_RETIRADA' WHERE id=$1`, l.reservas[1])
		if _, err := l.svc.EntregarPorReserva(ctx, application.EntregaPorReservaParams{ReservaIDs: l.reservas[1:2], EquiposEsperados: l.esperados(l.reservas[1:2]), LiberarNoEntregadas: true}); err != nil {
			t.Fatal(err)
		}
		g, err := l.repo.BuscarReservaGrupoPorID(ctx, l.grupo)
		if err != nil || g.Estado != domain.GrupoFinalizada {
			t.Fatalf("grupo incoherente: %+v %v", g, err)
		}
	})
	// La escritura condicional también protege al caller que guarda una copia vieja.
	t.Run("copia-obsoleta-no-reescribe-devolucion", func(t *testing.T) {
		l := loteDePrestamosReal(t, pool)
		p := crearPrestamoDeTest(t, l.repo, entregaDeTest(l.equipos[0]), ahoraDeTest().Add(-time.Hour))
		vieja, err := l.repo.BuscarPrestamoPorID(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Devolver("", "primera", ahoraDeTest()); err != nil {
			t.Fatal(err)
		}
		if err := l.repo.GuardarPrestamo(ctx, p); err != nil {
			t.Fatal(err)
		}
		if err := vieja.Devolver("", "segunda", ahoraDeTest().Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if err := l.repo.GuardarPrestamo(ctx, vieja); !errors.Is(err, domain.ErrPrestamoYaDevuelto) {
			t.Fatalf("se permitió sobrescribir una devolución: %v", err)
		}
		guardado, err := l.repo.BuscarPrestamoPorID(ctx, p.ID)
		if err != nil || guardado.Observaciones != "primera" {
			t.Fatalf("la constancia cambió: %+v %v", guardado, err)
		}
	})
}

// El wrapper pausa después de INSERT real y dentro de la transacción.
type repoEntregaEnPausa struct {
	application.Repo
	primera, continuar chan struct{}
	una                *sync.Once
}

func (r repoEntregaEnPausa) EnTransaccion(ctx context.Context, fn func(application.Repo) error) error {
	return r.Repo.EnTransaccion(ctx, func(repo application.Repo) error { r.Repo = repo; return fn(r) })
}
func (r repoEntregaEnPausa) CrearPrestamo(ctx context.Context, p *domain.Prestamo) error {
	if err := r.Repo.CrearPrestamo(ctx, p); err != nil {
		return err
	}
	var err error
	r.una.Do(func() {
		close(r.primera)
		select {
		case <-r.continuar:
		case <-ctx.Done():
			err = ctx.Err()
		}
	})
	return err
}

func TestPrestamosConsistencia_EntregaConcurrentePreservaLaPrimeraTanda(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	l := loteDePrestamosReal(t, pool)
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	pausa := repoEntregaEnPausa{Repo: l.repo, primera: make(chan struct{}), continuar: make(chan struct{}), una: &sync.Once{}}
	primeraSvc := application.NewService(pausa, nil, NewValidadorEquipoPostgres(pool), nil, nil, NuevoID, l.svc.Ahora, eventbus.NewInMemoryEventBus())
	type respuesta struct {
		r   *application.ResultadoEntrega
		err error
	}
	a, b := make(chan respuesta, 1), make(chan respuesta, 1)
	go func() {
		r, err := primeraSvc.EntregarPorReserva(ctx, application.EntregaPorReservaParams{ReservaIDs: l.reservas[:1], EquiposEsperados: l.esperados(l.reservas[:1])})
		a <- respuesta{r, err}
	}()
	select {
	case <-pausa.primera:
	case <-ctx.Done():
		t.Fatal("la entrega no llegó a la pausa")
	}
	go func() {
		r, err := l.svc.EntregarPorReserva(ctx, application.EntregaPorReservaParams{ReservaIDs: l.reservas[1:2], EquiposEsperados: l.esperados(l.reservas[1:2]), LiberarNoEntregadas: true})
		b <- respuesta{r, err}
	}()
	for {
		var espera bool
		// La entrega puede esperar en la coordinación institucional o en las filas.
		// Se observa el bloqueo entre conexiones de esta base aislada, sin depender
		// del texto de una consulta que podría ejecutarse después de esa espera.
		if err := pool.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM pg_stat_activity
			WHERE datname=current_database() AND pid<>pg_backend_pid()
			AND wait_event_type='Lock' AND cardinality(pg_blocking_pids(pid))>0
		)`).Scan(&espera); err != nil {
			t.Fatal(err)
		}
		if espera {
			break
		}
		select {
		case r := <-b:
			t.Fatalf("la segunda tanda no esperó a la primera: %+v %v", r.r, r.err)
		case <-ctx.Done():
			t.Fatal("no se observó la espera")
		case <-time.After(10 * time.Millisecond):
		}
	}
	close(pausa.continuar)
	ra, rb := <-a, <-b
	if ra.err != nil || rb.err != nil {
		t.Fatalf("entregas simultáneas: %v / %v", ra.err, rb.err)
	}
	if len(rb.r.Liberadas) != 1 || rb.r.Liberadas[0] != l.reservas[2] {
		t.Fatalf("se liberó la primera tanda: %v", rb.r.Liberadas)
	}
	r, err := l.repo.BuscarReservaPorID(ctx, l.reservas[0])
	if err != nil || r.Estado != domain.ReservaConfirmada {
		t.Fatalf("primera reserva incoherente: %+v %v", r, err)
	}
}

func TestPrestamosConsistencia_DevolucionesSimultaneasNoSeSobrescriben(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	l := loteDePrestamosReal(t, pool)
	p := crearPrestamoDeTest(t, l.repo, entregaDeTest(l.equipos[0]), l.svc.Ahora().Add(-time.Hour))
	receptores := []string{crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA"), crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")}
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	type respuesta struct {
		r           *application.ResultadoDevolucion
		err         error
		observacion string
		receptor    string
	}
	inicio, salidas := make(chan struct{}), make(chan respuesta, 2)
	for i, texto := range []string{"primera constancia", "segunda constancia"} {
		receptor := receptores[i]
		go func() {
			<-inicio
			r, err := l.svc.RecibirEquipos(ctx, []string{p.ID}, receptor, texto)
			salidas <- respuesta{r, err, texto, receptor}
		}()
	}
	close(inicio)
	a, b := <-salidas, <-salidas
	if a.err != nil || b.err != nil {
		t.Fatalf("devoluciones simultáneas: %v / %v", a.err, b.err)
	}
	if len(a.r.Recibidos)+len(b.r.Recibidos) != 1 || len(a.r.NoRecibidos)+len(b.r.NoRecibidos) != 1 {
		t.Fatalf("dos constancias de devolución: %+v / %+v", a.r, b.r)
	}
	ganadora := a
	if len(a.r.Recibidos) == 0 {
		ganadora = b
	}
	guardado, err := l.repo.BuscarPrestamoPorID(ctx, p.ID)
	if err != nil || guardado.Observaciones != ganadora.observacion || guardado.RecibidoPor == nil || *guardado.RecibidoPor != ganadora.receptor || !guardado.DevueltoEn.Equal(*ganadora.r.Recibidos[0].DevueltoEn) {
		t.Fatalf("la segunda devolución sobrescribió la primera: %+v %v", guardado, err)
	}
}

func (l loteDePrestamos) esperados(ids []string) map[string]string {
	equipos := make(map[string]string)
	for _, id := range ids {
		for i, reserva := range l.reservas {
			if id == reserva {
				equipos[id] = l.equipos[i]
			}
		}
	}
	return equipos
}
