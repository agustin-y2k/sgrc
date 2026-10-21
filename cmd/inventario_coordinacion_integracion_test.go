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

	availabilityapp "github.com/agustin-y2k/sgrc/internal/availability/application"
	availabilitydomain "github.com/agustin-y2k/sgrc/internal/availability/domain"
	availabilityinfra "github.com/agustin-y2k/sgrc/internal/availability/infrastructure"
	inventoryapp "github.com/agustin-y2k/sgrc/internal/inventory/application"
	inventorydomain "github.com/agustin-y2k/sgrc/internal/inventory/domain"
	inventoryinfra "github.com/agustin-y2k/sgrc/internal/inventory/infrastructure"
	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationdomain "github.com/agustin-y2k/sgrc/internal/reservation/domain"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pausaInstitucional struct {
	lista, continuar chan struct{}
	una              sync.Once
}

func nuevaPausaInstitucional(t *testing.T) *pausaInstitucional {
	p := &pausaInstitucional{lista: make(chan struct{}), continuar: make(chan struct{})}
	t.Cleanup(func() {
		select {
		case <-p.continuar:
		default:
			close(p.continuar)
		}
	})
	return p
}
func (p *pausaInstitucional) detener(ctx context.Context) error {
	var err error
	p.una.Do(func() {
		close(p.lista)
		select {
		case <-p.continuar:
		case <-ctx.Done():
			err = ctx.Err()
		}
	})
	return err
}
func (p *pausaInstitucional) esperar(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-p.lista:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
func esperarOperacionInstitucional(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	limite, cancelar := context.WithTimeout(ctx, 3*time.Second)
	defer cancelar()
	for {
		var esperando bool
		err := pool.QueryRow(limite, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%pg_advisory_xact_lock%')`).Scan(&esperando)
		if err != nil {
			t.Fatal(err)
		}
		if esperando {
			return
		}
		select {
		case <-limite.Done():
			t.Fatal("la operación no esperó la coordinación institucional")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

type inventarioEnPausa struct {
	inventoryapp.Repo
	pausa *pausaInstitucional
	vacio bool
}

type carroLeidoEnPausa struct {
	inventoryapp.Repo
	pausa *pausaInstitucional
}

func (r carroLeidoEnPausa) BuscarCarroPorID(ctx context.Context, id string) (*inventorydomain.Carro, error) {
	c, err := r.Repo.BuscarCarroPorID(ctx, id)
	if err == nil {
		err = r.pausa.detener(ctx)
	}
	return c, err
}

func (r inventarioEnPausa) BuscarEquipoPorID(ctx context.Context, id string) (*inventorydomain.Equipo, error) {
	e, err := r.Repo.BuscarEquipoPorID(ctx, id)
	if err == nil && !r.vacio {
		err = r.pausa.detener(ctx)
	}
	return e, err
}
func (r inventarioEnPausa) ListarEquiposPorCarro(ctx context.Context, id string) ([]*inventorydomain.Equipo, error) {
	e, err := r.Repo.ListarEquiposPorCarro(ctx, id)
	if err == nil && r.vacio {
		err = r.pausa.detener(ctx)
	}
	return e, err
}

type equipoValidadoEnPausa struct {
	reservationapp.ValidadorEquipo
	pausa *pausaInstitucional
}

func (v equipoValidadoEnPausa) EquiposNoReservables(ctx context.Context, ids []string) ([]string, error) {
	ids, err := v.ValidadorEquipo.EquiposNoReservables(ctx, ids)
	if err == nil {
		err = v.pausa.detener(ctx)
	}
	return ids, err
}
func (v equipoValidadoEnPausa) EquipoDisponibleParaReservar(ctx context.Context, id string) (bool, error) {
	valido, err := v.ValidadorEquipo.EquipoDisponibleParaReservar(ctx, id)
	if err == nil {
		err = v.pausa.detener(ctx)
	}
	return valido, err
}
func (v equipoValidadoEnPausa) CondicionParaEntregar(ctx context.Context, id string) (reservationapp.CondicionDeEquipo, error) {
	condicion, err := v.ValidadorEquipo.CondicionParaEntregar(ctx, id)
	if err == nil {
		err = v.pausa.detener(ctx)
	}
	return condicion, err
}

type jornadaEnPausa struct {
	availabilityapp.Repo
	pausa *pausaInstitucional
}

func (r jornadaEnPausa) ReemplazarJornada(ctx context.Context, bloques []*availabilitydomain.BloqueJornada) error {
	if err := r.Repo.ReemplazarJornada(ctx, bloques); err != nil {
		return err
	}
	return r.pausa.detener(ctx)
}

type escenarioInstitucional struct {
	d          datosCierre
	reservaID  string
	reservas   *reservationapp.Service
	inventario *inventoryapp.Service
	jornada    *availabilityapp.Service
	bus        eventbus.EventBus
}

func prepararInstitucion(t *testing.T, pool *pgxpool.Pool) escenarioInstitucional {
	t.Helper()
	d := prepararCierre(t, pool)
	ejecutarCierreSQL(t, pool, `TRUNCATE jornada_institucion`)
	ejecutarCierreSQL(t, pool, `UPDATE reserva SET estado='CONFIRMADA' WHERE reserva_grupo_id=$1`, d.grupo)
	var reservaID string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM reserva WHERE reserva_grupo_id=$1`, d.grupo).Scan(&reservaID); err != nil {
		t.Fatal(err)
	}
	bus := eventbus.NewInMemoryEventBus()
	ahora := func() time.Time { return time.Date(2026, 5, 4, 6, 0, 0, 0, time.UTC) }
	adaptador := &availabilityReservasAdapter{}
	jornada := availabilityapp.NewService(availabilityinfra.NewPostgresRepo(pool), nil, adaptador, uuid.NewString, ahora)
	reservas := reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), reservationinfra.NewValidadorMateriaPostgres(pool), reservationinfra.NewValidadorEquipoPostgres(pool), &reservationValidadorJornadaAdapter{jornada}, reservationinfra.NewObtenedorNombrePostgres(pool), uuid.NewString, ahora, bus)
	adaptador.reservationSvc = reservas
	inventario := inventoryapp.NewService(inventoryinfra.NewPostgresRepo(pool), &inventoryValidadorReservasAdapter{reservas}, uuid.NewString, ahora, nil, bus)
	return escenarioInstitucional{d, reservaID, reservas, inventario, jornada, bus}
}
func (e escenarioInstitucional) reservasCon(pool *pgxpool.Pool, validador reservationapp.ValidadorEquipo) *reservationapp.Service {
	return reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), reservationinfra.NewValidadorMateriaPostgres(pool), validador, &reservationValidadorJornadaAdapter{e.jornada}, reservationinfra.NewObtenedorNombrePostgres(pool), uuid.NewString, func() time.Time { return time.Date(2026, 5, 4, 6, 0, 0, 0, time.UTC) }, e.bus)
}
func (e escenarioInstitucional) inventarioCon(repo inventoryapp.Repo) *inventoryapp.Service {
	return inventoryapp.NewService(repo, &inventoryValidadorReservasAdapter{e.reservas}, uuid.NewString, func() time.Time { return time.Date(2026, 5, 4, 6, 0, 0, 0, time.UTC) }, nil, e.bus)
}
func (e escenarioInstitucional) reservar(ctx context.Context, svc *reservationapp.Service, modo, equipo string) error {
	fecha := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	switch modo {
	case "serie":
		_, err := svc.CrearReservaRecurrente(ctx, e.d.materia, e.d.docente, true, reservationdomain.Lunes, 8*time.Hour, 9*time.Hour, fecha, fecha.AddDate(0, 0, 7), []string{equipo}, nil)
		return err
	case "cambio":
		_, err := svc.CambiarEquipoDeReserva(ctx, e.reservaID, equipo, e.d.docente, true, true)
		return err
	default:
		_, _, _, err := svc.CrearReserva(ctx, e.d.materia, e.d.docente, true, fecha, 8*time.Hour, 9*time.Hour, []string{equipo}, nil)
		return err
	}
}
func (e escenarioInstitucional) entregar(ctx context.Context, svc *reservationapp.Service, suelta bool) (*reservationapp.ResultadoEntrega, error) {
	if suelta {
		return svc.EntregarSuelta(ctx, reservationapp.EntregaSueltaParams{EquipoIDs: []string{e.d.equipo}, Nombre: "Ada Docente"})
	}
	return svc.EntregarPorReserva(ctx, reservationapp.EntregaPorReservaParams{ReservaIDs: []string{e.reservaID}})
}

func TestInventarioCoordinado_ReservasYMantenimiento(t *testing.T) {
	pool := baseVacia(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelar()
	for _, modo := range []string{"simple", "serie", "cambio"} {
		for _, reservaPrimero := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s-reserva-primero-%v", modo, reservaPrimero), func(t *testing.T) {
				e := prepararInstitucion(t, pool)
				equipo := e.d.equipo
				if modo == "cambio" {
					equipo = uuid.NewString()
					ejecutarCierreSQL(t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,'Reemplazo','Computadora')`, equipo)
				}
				pausa := nuevaPausaInstitucional(t)
				finReserva, finEstado := make(chan error, 1), make(chan error, 1)
				if reservaPrimero {
					validador := equipoValidadoEnPausa{reservationinfra.NewValidadorEquipoPostgres(pool), pausa}
					go func() { finReserva <- e.reservar(ctx, e.reservasCon(pool, validador), modo, equipo) }()
					pausa.esperar(t, ctx)
					go func() {
						_, err := e.inventario.CambiarEstadoEquipo(ctx, equipo, inventorydomain.EstadoEnMantenimiento, nil)
						finEstado <- err
					}()
				} else {
					repo := inventarioEnPausa{inventoryinfra.NewPostgresRepo(pool), pausa, false}
					go func() {
						_, err := e.inventarioCon(repo).CambiarEstadoEquipo(ctx, equipo, inventorydomain.EstadoEnMantenimiento, nil)
						finEstado <- err
					}()
					pausa.esperar(t, ctx)
					go func() { finReserva <- e.reservar(ctx, e.reservas, modo, equipo) }()
				}
				esperarOperacionInstitucional(t, ctx, pool)
				close(pausa.continuar)
				if err := <-finEstado; err != nil {
					t.Fatal(err)
				}
				err := <-finReserva
				if reservaPrimero && err != nil {
					t.Fatal(err)
				}
				if !reservaPrimero && !errors.Is(err, reservationapp.ErrEquipoNoDisponible) {
					t.Fatalf("reserva aceptó mantenimiento: %v", err)
				}
				var vivas int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM reserva WHERE equipo_id=$1 AND estado='CONFIRMADA'`, equipo).Scan(&vivas); err != nil || vivas != 0 {
					t.Fatalf("reservas en mantenimiento: %d %v", vivas, err)
				}
			})
		}
	}
}

func TestInventarioCoordinado_BajaYCustodia(t *testing.T) {
	pool := baseVacia(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelar()
	for _, suelta := range []bool{false, true} {
		for _, entregaPrimero := range []bool{false, true} {
			t.Run(fmt.Sprintf("suelta-%v-entrega-primero-%v", suelta, entregaPrimero), func(t *testing.T) {
				e := prepararInstitucion(t, pool)
				pausa := nuevaPausaInstitucional(t)
				finEntrega, finBaja := make(chan error, 1), make(chan error, 1)
				entregar := func(svc *reservationapp.Service) {
					res, err := e.entregar(ctx, svc, suelta)
					if err == nil && ((entregaPrimero && len(res.Entregadas) != 1) || (!entregaPrimero && (len(res.Entregadas) != 0 || len(res.NoEntregadas) != 1))) {
						err = fmt.Errorf("entrega incoherente: %+v", res)
					}
					finEntrega <- err
				}
				if entregaPrimero {
					go entregar(e.reservasCon(pool, equipoValidadoEnPausa{reservationinfra.NewValidadorEquipoPostgres(pool), pausa}))
					pausa.esperar(t, ctx)
					go func() { _, err := e.inventario.DarDeBajaEquipo(ctx, e.d.equipo); finBaja <- err }()
				} else {
					go func() {
						_, err := e.inventarioCon(inventarioEnPausa{inventoryinfra.NewPostgresRepo(pool), pausa, false}).DarDeBajaEquipo(ctx, e.d.equipo)
						finBaja <- err
					}()
					pausa.esperar(t, ctx)
					go entregar(e.reservas)
				}
				esperarOperacionInstitucional(t, ctx, pool)
				close(pausa.continuar)
				if err := <-finEntrega; err != nil {
					t.Fatal(err)
				}
				err := <-finBaja
				if entregaPrimero && !errors.Is(err, inventoryapp.ErrEquipoPrestado) {
					t.Fatalf("baja permitió custodia: %v", err)
				}
				if !entregaPrimero && err != nil {
					t.Fatal(err)
				}
				var baja bool
				var afuera int
				if err := pool.QueryRow(ctx, `SELECT dado_de_baja,(SELECT count(*) FROM prestamo WHERE equipo_id=$1 AND devuelto_en IS NULL) FROM equipo WHERE id=$1`, e.d.equipo).Scan(&baja, &afuera); err != nil {
					t.Fatal(err)
				}
				if baja == entregaPrimero || (entregaPrimero && afuera != 1) || (!entregaPrimero && afuera != 0) {
					t.Fatalf("inventario/custodia incompatibles: baja=%v afuera=%d", baja, afuera)
				}
			})
		}
	}
}

func TestInventarioCoordinado_CarroYEdicion(t *testing.T) {
	pool := baseVacia(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelar()
	t.Run("alta-validada-antes-de-baja-de-carro", func(t *testing.T) {
		e := prepararInstitucion(t, pool)
		carro := uuid.NewString()
		ejecutarCierreSQL(t, pool, `INSERT INTO carro(id,nombre) VALUES($1,'Destino')`, carro)
		pausa := nuevaPausaInstitucional(t)
		finAlta, finBaja := make(chan error, 1), make(chan error, 1)
		go func() {
			_, err := e.inventarioCon(carroLeidoEnPausa{inventoryinfra.NewPostgresRepo(pool), pausa}).CrearEquipoDeCarro(ctx, carro, 1, "SERIE2", false, "", "", "", "")
			finAlta <- err
		}()
		pausa.esperar(t, ctx)
		go func() { finBaja <- e.inventario.DarDeBajaCarro(ctx, carro) }()
		esperarOperacionInstitucional(t, ctx, pool)
		close(pausa.continuar)
		if err := <-finAlta; err != nil {
			t.Fatal(err)
		}
		if err := <-finBaja; !errors.Is(err, inventoryapp.ErrCarroConEquipos) {
			t.Fatalf("retiró carro con alta concurrente: %v", err)
		}
	})
	for _, accion := range []string{"alta", "movimiento", "reactivacion"} {
		t.Run("carro-vacio-antes-de-"+accion, func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			carro := uuid.NewString()
			ejecutarCierreSQL(t, pool, `INSERT INTO carro(id,nombre) VALUES($1,'Destino')`, carro)
			if accion != "alta" {
				origen := uuid.NewString()
				ejecutarCierreSQL(t, pool, `INSERT INTO carro(id,nombre) VALUES($1,'Origen')`, origen)
				ejecutarCierreSQL(t, pool, `UPDATE equipo SET carro_id=$2,identificador=1,numero_serie='SERIE1',nombre=NULL WHERE id=$1`, e.d.equipo, origen)
			}
			if accion == "reactivacion" {
				ejecutarCierreSQL(t, pool, `UPDATE equipo SET carro_id=$2,dado_de_baja=true,fecha_baja=now() WHERE id=$1`, e.d.equipo, carro)
			}
			pausa := nuevaPausaInstitucional(t)
			finBaja, finIngreso := make(chan error, 1), make(chan error, 1)
			go func() {
				finBaja <- e.inventarioCon(inventarioEnPausa{inventoryinfra.NewPostgresRepo(pool), pausa, true}).DarDeBajaCarro(ctx, carro)
			}()
			pausa.esperar(t, ctx)
			go func() {
				var err error
				switch accion {
				case "alta":
					_, err = e.inventario.CrearEquipoDeCarro(ctx, carro, 1, "SERIE2", false, "", "", "", "")
				case "movimiento":
					_, err = e.inventario.EditarEquipo(ctx, e.d.equipo, inventoryapp.EditarEquipoParams{CarroID: &carro})
				case "reactivacion":
					err = e.inventario.ReactivarEquipo(ctx, e.d.equipo)
				}
				finIngreso <- err
			}()
			esperarOperacionInstitucional(t, ctx, pool)
			close(pausa.continuar)
			if err := <-finBaja; err != nil {
				t.Fatal(err)
			}
			if err := <-finIngreso; !errors.Is(err, inventoryapp.ErrCarroDadoDeBaja) {
				t.Fatalf("ingresó al carro retirado: %v", err)
			}
			var vivos int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM equipo WHERE carro_id=$1 AND NOT dado_de_baja`, carro).Scan(&vivos); err != nil || vivos != 0 {
				t.Fatalf("carro retirado con equipos activos: %d %v", vivos, err)
			}
		})
	}
	for _, baja := range []bool{false, true} {
		for _, editarPrimero := range []bool{false, true} {
			t.Run(fmt.Sprintf("edicion-baja-%v-editar-primero-%v", baja, editarPrimero), func(t *testing.T) {
				e := prepararInstitucion(t, pool)
				pausa := nuevaPausaInstitucional(t)
				finEdit, finEstado := make(chan error, 1), make(chan error, 1)
				editar := func(svc *inventoryapp.Service) {
					valor := true
					_, err := svc.EditarEquipo(ctx, e.d.equipo, inventoryapp.EditarEquipoParams{Freezado: &valor})
					finEdit <- err
				}
				cambiar := func(svc *inventoryapp.Service) {
					var err error
					if baja {
						_, err = svc.DarDeBajaEquipo(ctx, e.d.equipo)
					} else {
						_, err = svc.CambiarEstadoEquipo(ctx, e.d.equipo, inventorydomain.EstadoFueraDeServicio, nil)
					}
					finEstado <- err
				}
				conPausa := e.inventarioCon(inventarioEnPausa{inventoryinfra.NewPostgresRepo(pool), pausa, false})
				if editarPrimero {
					go editar(conPausa)
				} else {
					go cambiar(conPausa)
				}
				pausa.esperar(t, ctx)
				if editarPrimero {
					go cambiar(e.inventario)
				} else {
					go editar(e.inventario)
				}
				esperarOperacionInstitucional(t, ctx, pool)
				close(pausa.continuar)
				if err := <-finEdit; err != nil {
					t.Fatal(err)
				}
				if err := <-finEstado; err != nil {
					t.Fatal(err)
				}
				pc, err := inventoryinfra.NewPostgresRepo(pool).BuscarEquipoPorID(ctx, e.d.equipo)
				if err != nil || !pc.Freezado || pc.DadoDeBaja != baja || (!baja && pc.Estado != inventorydomain.EstadoFueraDeServicio) {
					t.Fatalf("la edición revirtió circulación: %+v %v", pc, err)
				}
			})
		}
	}
}

func TestInventarioCoordinado_JornadaYRollback(t *testing.T) {
	pool := baseVacia(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelar()
	tramos := []availabilityapp.TramoDeJornada{{DiaSemana: availabilitydomain.Lunes, HoraInicio: 11 * time.Hour, HoraFin: 12 * time.Hour}}
	for _, reservaPrimero := range []bool{false, true} {
		t.Run(fmt.Sprintf("jornada-reserva-primero-%v", reservaPrimero), func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			pausa := nuevaPausaInstitucional(t)
			finReserva, finJornada := make(chan error, 1), make(chan error, 1)
			if reservaPrimero {
				go func() {
					finReserva <- e.reservar(ctx, e.reservasCon(pool, equipoValidadoEnPausa{reservationinfra.NewValidadorEquipoPostgres(pool), pausa}), "simple", e.d.equipo)
				}()
				pausa.esperar(t, ctx)
				go func() { _, err := e.jornada.ReemplazarJornada(ctx, tramos, true); finJornada <- err }()
			} else {
				svc := availabilityapp.NewService(jornadaEnPausa{availabilityinfra.NewPostgresRepo(pool), pausa}, nil, &availabilityReservasAdapter{e.reservas}, uuid.NewString, func() time.Time { return time.Date(2026, 5, 4, 6, 0, 0, 0, time.UTC) })
				go func() { _, err := svc.ReemplazarJornada(ctx, tramos, true); finJornada <- err }()
				pausa.esperar(t, ctx)
				go func() { finReserva <- e.reservar(ctx, e.reservas, "simple", e.d.equipo) }()
			}
			esperarOperacionInstitucional(t, ctx, pool)
			close(pausa.continuar)
			if err := <-finJornada; err != nil {
				t.Fatal(err)
			}
			err := <-finReserva
			if reservaPrimero && err != nil {
				t.Fatal(err)
			}
			if !reservaPrimero && !errors.Is(err, reservationapp.ErrFueraDeJornada) {
				t.Fatalf("aceptó jornada obsoleta: %v", err)
			}
			var vivas int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM reserva WHERE equipo_id=$1 AND estado='CONFIRMADA'`, e.d.equipo).Scan(&vivas); err != nil || vivas != 0 {
				t.Fatalf("reserva fuera de jornada: %d %v", vivas, err)
			}
		})
	}
	for _, accion := range []string{"mantenimiento", "baja", "jornada"} {
		t.Run("rollback-y-avisos-"+accion, func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			var avisos atomic.Int32
			e.bus.Subscribe("reserva.cancelada", func(eventbus.Evento) { avisos.Add(1) })
			// El fallo sucede DESPUÉS de cancelar el hijo: también se debe revertir
			// el equipo o la jornada ya guardados y descartar los avisos.
			quitar := falloBajaSQL(t, pool, "reserva_grupo", "UPDATE", "")
			var err error
			var resultado *availabilityapp.ResultadoDeJornada
			switch accion {
			case "mantenimiento":
				_, err = e.inventario.CambiarEstadoEquipo(ctx, e.d.equipo, inventorydomain.EstadoEnMantenimiento, nil)
			case "baja":
				_, err = e.inventario.DarDeBajaEquipo(ctx, e.d.equipo)
			case "jornada":
				resultado, err = e.jornada.ReemplazarJornada(ctx, tramos, true)
			}
			quitar()
			if err == nil || resultado != nil {
				t.Fatalf("ocultó fallo o devolvió cambio confirmado: %+v %v", resultado, err)
			}
			pc, err := inventoryinfra.NewPostgresRepo(pool).BuscarEquipoPorID(ctx, e.d.equipo)
			if err != nil || pc.DadoDeBaja || pc.Estado != inventorydomain.EstadoDisponible {
				t.Fatalf("inventario no revertido: %+v %v", pc, err)
			}
			var estado string
			var jornada int
			if err := pool.QueryRow(ctx, `SELECT estado,(SELECT count(*) FROM jornada_institucion) FROM reserva WHERE id=$1`, e.reservaID).Scan(&estado, &jornada); err != nil || estado != "CONFIRMADA" || jornada != 0 {
				t.Fatalf("cascada no revertida: %s %d %v", estado, jornada, err)
			}
			if avisos.Load() != 0 {
				t.Fatal("publicó cancelaciones revertidas")
			}
			// Repetir la misma intención debe completar la operación desde el estado previo.
			switch accion {
			case "mantenimiento":
				_, err = e.inventario.CambiarEstadoEquipo(ctx, e.d.equipo, inventorydomain.EstadoEnMantenimiento, nil)
			case "baja":
				_, err = e.inventario.DarDeBajaEquipo(ctx, e.d.equipo)
			case "jornada":
				_, err = e.jornada.ReemplazarJornada(ctx, tramos, true)
			}
			if err != nil || avisos.Load() != 1 {
				t.Fatalf("reintento o avisos: %v %d", err, avisos.Load())
			}
		})
	}
}

type mostradorInstitucional struct{}

func (mostradorInstitucional) MostradorEn(context.Context, time.Time) (reservationapp.MostradorAtendido, error) {
	return reservationapp.MostradorAtendido{Atendido: true, Declarado: true}, nil
}
func (mostradorInstitucional) MostradorEseDia(context.Context, time.Time) (reservationapp.MostradorAtendido, error) {
	return reservationapp.MostradorAtendido{Atendido: true, Declarado: true}, nil
}

func TestInventarioCoordinado_BarridoYEntregaEspontanea(t *testing.T) {
	pool := baseVacia(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelar()
	e := prepararInstitucion(t, pool)
	pausa := nuevaPausaInstitucional(t)
	finEntrega, finBarrido := make(chan error, 1), make(chan error, 1)
	go func() {
		res, err := e.entregar(ctx, e.reservasCon(pool, equipoValidadoEnPausa{reservationinfra.NewValidadorEquipoPostgres(pool), pausa}), true)
		if err == nil && len(res.Entregadas) != 1 {
			err = fmt.Errorf("entrega rechazada: %+v", res)
		}
		finEntrega <- err
	}()
	pausa.esperar(t, ctx)
	ahora := func() time.Time { return time.Date(2026, 5, 4, 8, 50, 0, 0, time.UTC) }
	v := reservationapp.NewVigilante(reservationinfra.NewPostgresRepo(pool), e.bus, &reservationValidadorJornadaAdapter{e.jornada}, mostradorInstitucional{}, ahora, reservationapp.ConfigDeVigilanciaPorDefecto())
	go func() {
		res, err := v.Barrer(ctx)
		if err == nil && res.Liberadas != 0 {
			err = fmt.Errorf("liberó %d reservas con entrega en curso", res.Liberadas)
		}
		finBarrido <- err
	}()
	esperarOperacionInstitucional(t, ctx, pool)
	close(pausa.continuar)
	if err := <-finEntrega; err != nil {
		t.Fatal(err)
	}
	if err := <-finBarrido; err != nil {
		t.Fatal(err)
	}
	var estado string
	if err := pool.QueryRow(ctx, `SELECT estado FROM reserva WHERE id=$1`, e.reservaID).Scan(&estado); err != nil || estado != "CONFIRMADA" {
		t.Fatalf("liberó reserva de equipo afuera: %s %v", estado, err)
	}
}

func TestInventarioCoordinado_UnaConexion(t *testing.T) {
	pool := baseVacia(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	cfg := pool.Config()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	unico, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer unico.Close()
	e := prepararInstitucion(t, unico)
	if err := e.reservar(ctx, e.reservas, "simple", e.d.equipo); err != nil {
		t.Fatal(err)
	}
	if _, err := e.jornada.ReemplazarJornada(ctx, []availabilityapp.TramoDeJornada{{DiaSemana: availabilitydomain.Lunes, HoraInicio: 7 * time.Hour, HoraFin: 10 * time.Hour}}, true); err != nil {
		t.Fatal(err)
	}
	res, err := e.entregar(ctx, e.reservas, true)
	if err != nil || len(res.Entregadas) != 1 {
		t.Fatalf("entrega con pool de una conexión: %+v %v", res, err)
	}
	if _, err := e.reservas.RecibirEquipos(ctx, []string{res.Entregadas[0].ID}, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.inventario.CambiarEstadoEquipo(ctx, e.d.equipo, inventorydomain.EstadoEnMantenimiento, nil); err != nil {
		t.Fatal(err)
	}
}
