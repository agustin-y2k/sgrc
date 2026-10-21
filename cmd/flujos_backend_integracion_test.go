//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	auditoriaapp "github.com/agustin-y2k/sgrc/internal/auditoria/application"
	auditoriainfra "github.com/agustin-y2k/sgrc/internal/auditoria/infrastructure"
	auditoriahttp "github.com/agustin-y2k/sgrc/internal/auditoria/interfaces/http"
	inventoryapp "github.com/agustin-y2k/sgrc/internal/inventory/application"
	inventoryinfra "github.com/agustin-y2k/sgrc/internal/inventory/infrastructure"
	notificationdomain "github.com/agustin-y2k/sgrc/internal/notification/domain"
	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
	sugerenciasapp "github.com/agustin-y2k/sgrc/internal/sugerencias/application"
	sugerenciasinfra "github.com/agustin-y2k/sgrc/internal/sugerencias/infrastructure"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contactoNoDisponible struct{}

func (contactoNoDisponible) NombreYEmail(context.Context, string) (string, string, error) {
	return "", "", errors.New("contacto temporalmente no disponible")
}

func TestFlujosBackend_RespuestaSinContactoEntregaAvisosDelHilo(t *testing.T) {
	pool := baseVacia(t)
	e := prepararBajaReal(t, pool, false)
	ctx := context.Background()
	var admin string
	if err := pool.QueryRow(ctx, `SELECT id FROM usuario WHERE rol='ADMIN'`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	bus := busDurableReal(t, pool)
	svc := sugerenciasDurables(pool, bus)
	hilo, err := svc.Escribir(ctx, e.usuario, "AYUDA", "Consulta", "Texto inicial", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bus.Despachar(ctx, 50); err != nil {
		t.Fatal(err)
	}
	sinContacto := sugerenciasapp.NewService(sugerenciasinfra.NewPostgresRepo(pool), contactoNoDisponible{}, uuid.NewString, time.Now, bus)
	if _, err := sinContacto.Responder(ctx, hilo.ID, admin, true, "Respuesta del administrador"); err != nil {
		t.Fatal(err)
	}
	if _, err := bus.Despachar(ctx, 50); err != nil {
		t.Fatal(err)
	}
	comprobarCantidad(t, pool, 0, `SELECT count(*) FROM notificacion WHERE hilo_id=$1 AND tipo='SUGERENCIA' AND estado='NO_LEIDA'`, hilo.ID)
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE hilo_id=$1 AND usuario_id=$2 AND tipo='SUGERENCIA_RESPONDIDA' AND estado='NO_LEIDA'`, hilo.ID, e.usuario)
	if _, err := sinContacto.Responder(ctx, hilo.ID, e.usuario, false, "Otra consulta"); err != nil {
		t.Fatal(err)
	}
	if _, err := bus.Despachar(ctx, 50); err != nil {
		t.Fatal(err)
	}
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE hilo_id=$1 AND usuario_id=$2 AND tipo='SUGERENCIA' AND estado='NO_LEIDA'`, hilo.ID, admin)
}

func TestFlujosBackend_BorradoLicenciaYCierreDeAvisoSonAtomicos(t *testing.T) {
	pool := baseVacia(t)
	e := prepararInstitucion(t, pool)
	ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Admin','Prueba','admin@test.local','hash','ADMIN','APROBADA')`, uuid.NewString())
	ctx := context.Background()
	bus := busDurableReal(t, pool)
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	svc := inventoryapp.NewService(inventoryinfra.NewPostgresRepo(pool), nil, uuid.NewString, func() time.Time { return now }, nil, bus)
	vencida := now.AddDate(0, 0, -1)
	alta, err := svc.CrearLicencias(ctx, inventoryapp.NuevaLicenciaParams{
		EquipoIDs: []string{e.d.equipo}, Nombre: "Software de prueba", DiasDuracion: 30, DiasAviso: 7,
		Vencimiento: inventoryapp.VencimientoDeclarado{VenceEl: &vencida},
	})
	if err != nil || len(alta.Creadas) != 1 {
		t.Fatalf("alta: %+v %v", alta, err)
	}
	if _, err := inventoryapp.NewAvisadorDeLicencias(inventoryinfra.NewPostgresRepo(pool), bus, func() time.Time { return now }).Barrer(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := bus.Despachar(ctx, 50); err != nil {
		t.Fatal(err)
	}
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE tipo='LICENCIA_POR_VENCER' AND estado='NO_LEIDA'`)
	quitar := falloBajaSQL(t, pool, "evento_pendiente", "INSERT", "")
	err = svc.BorrarLicencia(ctx, alta.Creadas[0].ID)
	quitar()
	if err == nil {
		t.Fatal("se confirmó el borrado sin su evento")
	}
	if _, err := svc.ObtenerLicencia(ctx, alta.Creadas[0].ID); err != nil {
		t.Fatalf("el fallo del evento no revirtió el borrado: %v", err)
	}
	if err := svc.BorrarLicencia(ctx, alta.Creadas[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := bus.Despachar(ctx, 50); err != nil {
		t.Fatal(err)
	}
	comprobarCantidad(t, pool, 0, `SELECT count(*) FROM notificacion WHERE tipo='LICENCIA_POR_VENCER' AND estado='NO_LEIDA'`)
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE tipo='LICENCIA_POR_VENCER' AND estado='LEIDA'`)
}

type cierreContadoEnPausa struct {
	reservationapp.Repo
	pausa *pausaInstitucional
}

func TestFlujosBackend_PreferenciasAuditadasYRollback(t *testing.T) {
	pool := baseVacia(t)
	e := prepararInstitucion(t, pool)
	ctx := pgtransaccion.ConActor(context.Background(), e.d.docente, "192.0.2.42")
	alta, err := e.inventario.MarcarPreferencia(ctx, inventoryapp.NuevaPreferenciaParams{EquipoIDs: []string{e.d.equipo}, MateriaNombre: "Matemática", Prioridad: 2})
	if err != nil || len(alta.Creadas) != 1 {
		t.Fatalf("alta: %+v %v", alta, err)
	}
	if _, err := e.inventario.EditarPreferencia(ctx, alta.Creadas[0].ID, nil, nil, nil, 1); err != nil {
		t.Fatal(err)
	}
	if err := e.inventario.BorrarPreferencia(ctx, alta.Creadas[0].ID); err != nil {
		t.Fatal(err)
	}
	comprobarCantidad(t, pool, 3, `SELECT count(*) FROM audit_log WHERE entidad='equipo_preferencia' AND usuario_id=$1 AND host(ip_origen)='192.0.2.42'`, e.d.docente)
	quitar := falloBajaSQL(t, pool, "audit_log", "INSERT", "WHEN (NEW.entidad='equipo_preferencia')")
	_, err = e.inventario.MarcarPreferencia(ctx, inventoryapp.NuevaPreferenciaParams{EquipoIDs: []string{e.d.equipo}, MateriaNombre: "Biblioteca", Prioridad: 1})
	quitar()
	if err == nil {
		t.Fatal("se guardó una preferencia sin auditoría")
	}
	var falloSQL *pgconn.PgError
	if !errors.As(err, &falloSQL) || falloSQL.Message != "fallo de baja inyectado" {
		t.Fatalf("la prueba no alcanzó el fallo inyectado de auditoría: %v", err)
	}
	comprobarCantidad(t, pool, 0, `SELECT count(*) FROM equipo_preferencia`)
}

func TestFlujosBackend_AuditoriaFiltraDiasLocalesYRechazaRangoInvertido(t *testing.T) {
	pool := baseVacia(t)
	zona, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatal(err)
	}
	repo := auditoriainfra.NewPostgresRepo(pool, zona)
	h := auditoriahttp.NewHandler(auditoriaapp.NewService(repo))
	app := fiber.New()
	app.Get("/api/auditoria", h.Listar)
	fecha := time.Date(2026, 5, 4, 0, 0, 0, 0, zona)
	instantes := []time.Time{fecha.Add(-time.Microsecond), fecha, fecha.Add(12 * time.Hour), fecha.AddDate(0, 0, 1).Add(-time.Microsecond), fecha.AddDate(0, 0, 1)}
	esperadas := make(map[string]bool)
	for i, instante := range instantes {
		id := uuid.NewString()
		ejecutarCierreSQL(t, pool, `INSERT INTO audit_log(id,usuario_id,accion,entidad,creado_en) VALUES($1,$2,'PRUEBA_FLUJO','equipo',$3)`, id, uuid.NewString(), instante)
		if i >= 1 && i <= 3 {
			esperadas[id] = true
		}
	}
	for _, caso := range []struct {
		query         string
		estado, total int
	}{
		{"desde=2026-05-04&hasta=2026-05-04", 200, 3},
		{"desde=2026-05-05&hasta=2026-05-04", 400, 0},
	} {
		t.Run(caso.query, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("GET", "/api/auditoria?accion=PRUEBA_FLUJO&"+caso.query, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != caso.estado {
				t.Fatalf("estado=%d esperado=%d", resp.StatusCode, caso.estado)
			}
			if caso.estado == 200 {
				var respuesta struct {
					Data []struct{ ID string }
					Meta struct{ Total int }
				}
				if err := json.NewDecoder(resp.Body).Decode(&respuesta); err != nil {
					t.Fatal(err)
				}
				if respuesta.Meta.Total != caso.total || len(respuesta.Data) != caso.total {
					t.Fatalf("rango local incorrecto: %+v", respuesta)
				}
				for _, entrada := range respuesta.Data {
					if !esperadas[entrada.ID] {
						t.Fatalf("incluyó una entrada del día vecino: %s", entrada.ID)
					}
				}
			}
		})
	}
}

func (r cierreContadoEnPausa) ContarAvisadosSinDevolver(ctx context.Context) (int, error) {
	n, err := r.Repo.ContarAvisadosSinDevolver(ctx)
	if err == nil {
		err = r.pausa.detener(ctx)
	}
	return n, err
}

func servicioDevoluciones(pool *pgxpool.Pool, repo reservationapp.Repo, bus eventbus.EventBus) *reservationapp.Service {
	return reservationapp.NewService(repo, nil, reservationinfra.NewValidadorEquipoPostgres(pool), nil, nil, uuid.NewString, time.Now, bus)
}

// Dos equipos distintos pueden regresar a la vez. El último debe cerrar el
// aviso de cierre; dos snapshots anteriores al commit no deben dejarlo abierto.
func TestFlujosBackend_DevolucionesDistintasCierranElAviso(t *testing.T) {
	pool := baseVacia(t)
	e := prepararBajaReal(t, pool, false)
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	bus := busDurableReal(t, pool)
	repo := reservationinfra.NewPostgresRepo(pool)
	svc := servicioDevoluciones(pool, repo, bus)
	entrega, err := svc.EntregarPorReserva(ctx, reservationapp.EntregaPorReservaParams{ReservaIDs: e.reservas})
	if err != nil || len(entrega.Entregadas) != 2 {
		t.Fatalf("entrega inicial: %+v %v", entrega, err)
	}
	ejecutarCierreSQL(t, pool, `UPDATE prestamo SET avisado_cierre_para='2026-05-04'`)
	ejecutarCierreSQL(t, pool, `INSERT INTO notificacion(usuario_id,mensaje,tipo) SELECT id,'Quedaron equipos afuera',$1 FROM usuario WHERE rol='ADMIN'`, string(notificationdomain.TipoEquipoSinDevolver))
	pausa := nuevaPausaInstitucional(t)
	primero := servicioDevoluciones(pool, cierreContadoEnPausa{repo, pausa}, bus)
	finPrimero, finSegundo := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := primero.RecibirEquipos(ctx, []string{entrega.Entregadas[0].ID}, "", "Primera devolución")
		finPrimero <- err
	}()
	pausa.esperar(t, ctx)
	go func() {
		_, err := svc.RecibirEquipos(ctx, []string{entrega.Entregadas[1].ID}, "", "Segunda devolución")
		finSegundo <- err
	}()
	esperarOperacionInstitucional(t, ctx, pool)
	close(pausa.continuar)
	if err := <-finPrimero; err != nil {
		t.Fatal(err)
	}
	if err := <-finSegundo; err != nil {
		t.Fatal(err)
	}
	if _, err := bus.Despachar(ctx, 50); err != nil {
		t.Fatal(err)
	}
	comprobarCantidad(t, pool, 0, `SELECT count(*) FROM prestamo WHERE devuelto_en IS NULL`)
	comprobarCantidad(t, pool, 0, `SELECT count(*) FROM notificacion WHERE tipo=$1 AND estado='NO_LEIDA'`, string(notificationdomain.TipoEquipoSinDevolver))
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE tipo=$1 AND estado='LEIDA'`, string(notificationdomain.TipoEquipoSinDevolver))
}

func TestFlujosBackend_PreferenciasEsperanLaReservaEnCurso(t *testing.T) {
	pool := baseVacia(t)
	for _, operacion := range []string{"alta", "edicion", "borrado"} {
		t.Run(operacion, func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancelar()
			alta, err := e.inventario.MarcarPreferencia(ctx, inventoryapp.NuevaPreferenciaParams{EquipoIDs: []string{e.d.equipo}, MateriaNombre: "Matemática", Prioridad: 2})
			if err != nil || len(alta.Creadas) != 1 {
				t.Fatalf("marca inicial: %+v %v", alta, err)
			}
			pausa := nuevaPausaInstitucional(t)
			reservas := e.reservasCon(pool, equipoValidadoEnPausa{reservationinfra.NewValidadorEquipoPostgres(pool), pausa})
			finReserva, finPreferencia := make(chan error, 1), make(chan error, 1)
			go func() { finReserva <- e.reservar(ctx, reservas, "simple", e.d.equipo) }()
			pausa.esperar(t, ctx)
			go func() {
				var err error
				switch operacion {
				case "alta":
					_, err = e.inventario.MarcarPreferencia(ctx, inventoryapp.NuevaPreferenciaParams{EquipoIDs: []string{e.d.equipo}, MateriaNombre: "Biblioteca", Prioridad: 1})
				case "edicion":
					_, err = e.inventario.EditarPreferencia(ctx, alta.Creadas[0].ID, nil, nil, nil, 1)
				case "borrado":
					err = e.inventario.BorrarPreferencia(ctx, alta.Creadas[0].ID)
				}
				finPreferencia <- err
			}()
			esperarOperacionInstitucional(t, ctx, pool)
			close(pausa.continuar)
			if err := <-finReserva; err != nil {
				t.Fatal(err)
			}
			if err := <-finPreferencia; err != nil {
				t.Fatal(err)
			}
			comprobarCantidad(t, pool, 1, `SELECT count(*) FROM reserva WHERE equipo_id=$1 AND fecha='2026-05-11' AND estado='CONFIRMADA'`, e.d.equipo)
		})
	}
}
