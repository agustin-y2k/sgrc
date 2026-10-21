//go:build integration

package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	academicinfra "github.com/agustin-y2k/sgrc/internal/academic/infrastructure"
	inventoryapp "github.com/agustin-y2k/sgrc/internal/inventory/application"
	inventorydomain "github.com/agustin-y2k/sgrc/internal/inventory/domain"
	inventoryinfra "github.com/agustin-y2k/sgrc/internal/inventory/infrastructure"
	notificationapp "github.com/agustin-y2k/sgrc/internal/notification/application"
	notificationinfra "github.com/agustin-y2k/sgrc/internal/notification/infrastructure"
	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
	"github.com/agustin-y2k/sgrc/internal/shared/secretos"
	sugerenciasapp "github.com/agustin-y2k/sgrc/internal/sugerencias/application"
	sugerenciasinfra "github.com/agustin-y2k/sgrc/internal/sugerencias/infrastructure"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func busDurableReal(t *testing.T, pool *pgxpool.Pool) *eventbus.DurableBus {
	t.Helper()
	cifrador, err := secretos.Nuevo("clave-estable-para-eventos-de-prueba-segura")
	if err != nil {
		t.Fatal(err)
	}
	bus := eventbus.NewDurableBus(pool, cifrador)
	avisos := notificationapp.NewService(notificationinfra.NewPostgresRepo(pool), notificationinfra.NewListadorAdminsPostgres(pool), nil, uuid.NewString, time.Now)
	notificationapp.RegisterEventHandlers(bus, avisos)
	return bus
}
func publicarDurable(t *testing.T, bus *eventbus.DurableBus, e eventbus.Evento) {
	t.Helper()
	if err := bus.EnOperacion(context.Background(), func(ctx context.Context) error { eventbus.Emitir(ctx, bus, e); return nil }); err != nil {
		t.Fatal(err)
	}
}
func comprobarCantidad(t *testing.T, pool *pgxpool.Pool, esperada int, sql string, args ...any) {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil || n != esperada {
		t.Fatalf("cantidad=%d, esperada=%d, error=%v; %s", n, esperada, err, sql)
	}
}
func sugerenciasDurables(pool *pgxpool.Pool, bus *eventbus.DurableBus) *sugerenciasapp.Service {
	return sugerenciasapp.NewService(sugerenciasinfra.NewPostgresRepo(pool), sugerenciasinfra.NewUsuarioPostgres(pool), uuid.NewString, time.Now, bus)
}
func TestEventosDurables_AtomicidadReinicioReintentoYOrden(t *testing.T) {
	pool := baseVacia(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	for _, caso := range []string{"origen-revertido", "outbox-no-disponible", "cancelacion-revertida", "baja-durable", "remocion-durable", "auth-durable", "destinatario-eliminado", "referencias-eliminadas", "consumidor-reintenta", "reinicio-y-dos-workers", "tareas-independientes", "hilos-independientes", "reclamo-concurrente", "reclamo-revertido", "cifrado", "pool-unico", "barrido-unico", "marca-revertida"} {
		t.Run(caso, func(t *testing.T) {
			e := prepararBajaReal(t, pool, false)
			ejecutarCierreSQL(t, pool, `TRUNCATE evento_pendiente`)
			var admin string
			if err := pool.QueryRow(ctx, `SELECT id FROM usuario WHERE rol='ADMIN'`).Scan(&admin); err != nil {
				t.Fatal(err)
			}
			bus := busDurableReal(t, pool)
			svc := sugerenciasDurables(pool, bus)
			if caso == "origen-revertido" {
				err := bus.EnOperacion(ctx, func(ctx context.Context) error {
					if _, err := svc.Escribir(ctx, e.usuario, "SUGERENCIA", "Idea", "Texto", "", ""); err != nil {
						return err
					}
					return errors.New("fallo posterior a la publicación")
				})
				if err == nil {
					t.Fatal("no se rechazó el origen")
				}
				comprobarCantidad(t, pool, 0, `SELECT count(*) FROM sugerencia`)
				comprobarCantidad(t, pool, 0, `SELECT count(*) FROM evento_pendiente`)
				return
			}
			if caso == "outbox-no-disponible" {
				quitar := falloBajaSQL(t, pool, "evento_pendiente", "INSERT", "")
				_, err := svc.Escribir(ctx, e.usuario, "SUGERENCIA", "Idea", "Texto", "", "")
				quitar()
				if err == nil {
					t.Fatal("confirmó el hilo sin su evento")
				}
				comprobarCantidad(t, pool, 0, `SELECT count(*) FROM sugerencia`)
				comprobarCantidad(t, pool, 0, `SELECT count(*) FROM sugerencia_mensaje`)
				comprobarCantidad(t, pool, 0, `SELECT count(*) FROM evento_pendiente`)
				return
			}

			if caso == "cancelacion-revertida" {
				reservas := reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), nil, reservationinfra.NewValidadorEquipoPostgres(pool), nil, reservationinfra.NewObtenedorNombrePostgres(pool), uuid.NewString, func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }, bus)
				var grupo string
				if err := pool.QueryRow(ctx, `SELECT reserva_grupo_id FROM reserva WHERE id=$1`, e.reservas[0]).Scan(&grupo); err != nil {
					t.Fatal(err)
				}
				for _, alcance := range []string{"reserva", "ocurrencia", "serie"} {
					quitar := falloBajaSQL(t, pool, "evento_pendiente", "INSERT", "")
					var err error
					if alcance == "reserva" {
						err = reservas.CancelarReserva(ctx, e.reservas[0], admin, true, "Motivo")
					} else {
						_, err = reservas.CancelarOcurrenciaRecurrente(ctx, grupo, admin, true, "Motivo", alcance == "ocurrencia")
					}
					quitar()
					if err == nil {
						t.Fatalf("confirmó %s sin evento", alcance)
					}
					comprobarCantidad(t, pool, 1, `SELECT count(*) FROM reserva WHERE id=$1 AND estado='CONFIRMADA'`, e.reservas[0])
					comprobarCantidad(t, pool, 0, `SELECT count(*) FROM evento_pendiente`)
				}
				return
			}
			if caso == "destinatario-eliminado" || caso == "referencias-eliminadas" {
				publicarDurable(t, bus, eventbus.Evento{Tipo: "reserva.pedido-de-liberacion", Payload: eventbus.PedidoDeLiberacion{UsuarioID: e.usuario, ReservaID: e.reservas[0], SolicitanteID: admin, SolicitanteNombre: "Otro", Etiqueta: "Notebook"}})
				if caso == "destinatario-eliminado" {
					ejecutarCierreSQL(t, pool, `DELETE FROM usuario WHERE id=$1`, e.usuario)
				} else {
					ejecutarCierreSQL(t, pool, `DELETE FROM reserva WHERE id=$1`, e.reservas[0])
					ejecutarCierreSQL(t, pool, `DELETE FROM usuario WHERE id=$1`, admin)
				}
				if n, err := bus.Despachar(ctx, 10); err != nil || n != 1 {
					t.Fatalf("evento bloqueado por dato eliminado: %d %v", n, err)
				}
				esperados := 1
				if caso == "destinatario-eliminado" {
					esperados = 0
				}
				comprobarCantidad(t, pool, esperados, `SELECT count(*) FROM notificacion`)
				return
			}

			if caso == "baja-durable" || caso == "remocion-durable" {
				e.bus = bus
				esperados := 4
				if caso == "baja-durable" {
					if err := servicioBajaReal(pool, e, e.repo).DarDeBaja(ctx, e.usuario); err != nil {
						t.Fatal(err)
					}
				} else {
					esperados = 2
					academic := servicioAsignacionesReal(pool, e, academicinfra.NewValidadorUsuarioPostgres(pool))
					if _, err := academic.RemoverDocenteMateria(ctx, vinculoReal(t, pool, e.usuario, e.materias[0])); err != nil {
						t.Fatal(err)
					}
				}
				comprobarCantidad(t, pool, esperados, `SELECT count(*) FROM evento_pendiente`)
				if n, err := bus.Despachar(ctx, 10); err != nil || n != esperados {
					t.Fatalf("eventos de cascada: %d %v", n, err)
				}
				comprobarCantidad(t, pool, esperados, `SELECT count(*) FROM notificacion`)
				return
			}
			if caso == "auth-durable" {
				auth := servicioRecuperacionConBus(pool, bus)
				if err := auth.SolicitarRecuperacionDePassword(ctx, "ada@test.local"); err != nil {
					t.Fatal(err)
				}
				comprobarCantidad(t, pool, 1, `SELECT count(*) FROM codigo_recuperacion WHERE usado_en IS NULL`)
				pendiente := uuid.NewString()
				ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Nuevo','Docente','nuevo@test.local','hash','DOCENTE','PENDIENTE')`, pendiente)
				publicarDurable(t, bus, eventbus.Evento{Tipo: "docente.registro.pendiente", Payload: map[string]string{"usuarioId": pendiente, "nombre": "Nuevo", "apellido": "Docente"}})
				if err := auth.Aprobar(ctx, pendiente, admin); err != nil {
					t.Fatal(err)
				}
				if n, err := bus.Despachar(ctx, 10); err != nil || n != 4 {
					t.Fatalf("eventos de auth: %d %v", n, err)
				}
				comprobarCantidad(t, pool, 0, `SELECT count(*) FROM notificacion WHERE tipo='DOCENTE_PENDIENTE' AND estado='NO_LEIDA'`)
				return
			}
			if caso == "cifrado" {
				publicarDurable(t, bus, eventbus.Evento{Tipo: "password.recuperacion.solicitada", Payload: eventbus.DatosDeRecuperacion{Email: "privado@test.local", Codigo: "123456-secret", Nombre: "Privado"}})
				var almacenado string
				if err := pool.QueryRow(ctx, `SELECT payload FROM evento_pendiente`).Scan(&almacenado); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(almacenado, "123456-secret") || strings.Contains(almacenado, "privado@test.local") {
					t.Fatal("datos privados guardados en claro")
				}
				recibido := false
				bus.Subscribe("password.recuperacion.solicitada", func(e eventbus.Evento) {
					p, ok := e.Payload.(eventbus.DatosDeRecuperacion)
					recibido = ok && p.Codigo == "123456-secret"
				})
				if n, err := bus.Despachar(ctx, 10); err != nil || n != 1 || !recibido {
					t.Fatalf("payload no recuperable: %d %v %t", n, err, recibido)
				}
				return
			}
			if caso == "reclamo-concurrente" || caso == "reclamo-revertido" {
				reservas := reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), nil, reservationinfra.NewValidadorEquipoPostgres(pool), nil, reservationinfra.NewObtenedorNombrePostgres(pool), uuid.NewString, func() time.Time { return time.Date(2026, 5, 4, 6, 0, 0, 0, time.UTC) }, bus)
				if caso == "reclamo-revertido" {
					quitar := falloBajaSQL(t, pool, "evento_pendiente", "INSERT", "")
					err := reservas.PedirLiberacionDeReserva(ctx, e.reservas[0], admin, "Necesito el equipo")
					quitar()
					if err == nil {
						t.Fatal("confirmó un reclamo sin aviso durable")
					}
					comprobarCantidad(t, pool, 0, `SELECT count(*) FROM pedido_liberacion`)
					if err := reservas.PedirLiberacionDeReserva(ctx, e.reservas[0], admin, "Necesito el equipo"); err != nil {
						t.Fatal(err)
					}
				} else {
					inicio := make(chan struct{})
					resultados := make(chan error, 8)
					var wg sync.WaitGroup
					for range 8 {
						wg.Add(1)
						go func() {
							defer wg.Done()
							<-inicio
							resultados <- reservas.PedirLiberacionDeReserva(ctx, e.reservas[0], admin, "Necesito el equipo")
						}()
					}
					close(inicio)
					wg.Wait()
					close(resultados)
					exitos := 0
					for err := range resultados {
						if err == nil {
							exitos++
						} else if !errors.Is(err, reservationapp.ErrPedidoRepetido) {
							t.Fatal(err)
						}
					}
					if exitos != 1 {
						t.Fatalf("pedidos aceptados=%d", exitos)
					}
				}
				comprobarCantidad(t, pool, 1, `SELECT count(*) FROM pedido_liberacion`)
				comprobarCantidad(t, pool, 1, `SELECT count(*) FROM evento_pendiente`)
				if n, err := bus.Despachar(ctx, 10); err != nil || n != 1 {
					t.Fatalf("entrega=%d %v", n, err)
				}
				comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE tipo='PEDIDO_DE_LIBERACION'`)
				return
			}
			if caso == "barrido-unico" || caso == "marca-revertida" {
				var equipo string
				if err := pool.QueryRow(ctx, `SELECT equipo_id FROM reserva WHERE id=$1`, e.reservas[0]).Scan(&equipo); err != nil {
					t.Fatal(err)
				}
				ejecutarCierreSQL(t, pool, `INSERT INTO licencia_software(id,equipo_id,nombre,dias_duracion,dias_aviso,fecha_vencimiento) VALUES($1,$2,'Software',30,5,'2026-05-06')`, uuid.NewString(), equipo)
				barrer := func() error {
					_, err := inventoryapp.NewAvisadorDeLicencias(inventoryinfra.NewPostgresRepo(pool), bus, func() time.Time { return time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC) }).Barrer(ctx)
					return err
				}
				if caso == "marca-revertida" {
					quitar := falloBajaSQL(t, pool, "licencia_software", "UPDATE", "")
					err := barrer()
					quitar()
					if err == nil {
						t.Fatal("confirmó el evento sin su marca")
					}
					comprobarCantidad(t, pool, 0, `SELECT count(*) FROM evento_pendiente`)
					comprobarCantidad(t, pool, 0, `SELECT count(*) FROM licencia_software WHERE avisado_previo_para IS NOT NULL`)
				}
				resultados := make(chan error, 2)
				for range 2 {
					go func() { resultados <- barrer() }()
				}
				for range 2 {
					if err := <-resultados; err != nil {
						t.Fatal(err)
					}
				}
				comprobarCantidad(t, pool, 1, `SELECT count(*) FROM evento_pendiente WHERE tipo='licencia.por-vencer'`)
				if n, err := bus.Despachar(ctx, 10); err != nil || n != 1 {
					t.Fatalf("barrido %d %v", n, err)
				}
				comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE tipo='LICENCIA_POR_VENCER'`)
				return
			}
			if caso == "tareas-independientes" {
				pedidos := []string{uuid.NewString(), uuid.NewString()}
				for _, id := range pedidos {
					ejecutarCierreSQL(t, pool, `INSERT INTO pedido_de_materia(id,usuario_id,materia_solicitada,motivo) VALUES($1,$2,$3,'Motivo')`, id, e.usuario, id)
					publicarDurable(t, bus, eventbus.Evento{Tipo: "materia.pedido.nuevo", Payload: eventbus.PedidoDeMateriaNuevo{PedidoID: id, UsuarioID: e.usuario, MateriaNombre: id}})
				}
				ejecutarCierreSQL(t, pool, `UPDATE pedido_de_materia SET estado='RECHAZADO',resuelto_por=$2,resuelto_en=now() WHERE id=$1`, pedidos[0], admin)
				publicarDurable(t, bus, eventbus.Evento{Tipo: "materia.pedido.resuelto", Payload: eventbus.PedidoDeMateriaResuelto{PedidoID: pedidos[0], UsuarioID: e.usuario}})
				if n, err := bus.Despachar(ctx, 10); err != nil || n != 3 {
					t.Fatalf("pedidos %d %v", n, err)
				}
				comprobarCantidad(t, pool, 0, `SELECT count(*) FROM notificacion WHERE pedido_id=$1 AND tipo='PEDIDO_DE_MATERIA' AND estado='NO_LEIDA'`, pedidos[0])
				comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE pedido_id=$1 AND tipo='PEDIDO_DE_MATERIA' AND estado='NO_LEIDA'`, pedidos[1])
				return
			}
			if caso == "pool-unico" {
				config := pool.Config().Copy()
				config.MaxConns = 1
				unico, err := pgxpool.NewWithConfig(ctx, config)
				if err != nil {
					t.Fatal(err)
				}
				defer unico.Close()
				bus = busDurableReal(t, unico)
				svc = sugerenciasDurables(unico, bus)
			}
			sug, err := svc.Escribir(ctx, e.usuario, "SUGERENCIA", "Idea", "Texto", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if caso == "hilos-independientes" {
				otro, err := svc.Escribir(ctx, e.usuario, "AYUDA", "Otra pregunta", "Otro texto", "", "")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := svc.Responder(ctx, sug.ID, admin, true, "Respuesta"); err != nil {
					t.Fatal(err)
				}
				if n, err := bus.Despachar(ctx, 10); err != nil || n != 3 {
					t.Fatalf("hilos %d %v", n, err)
				}
				comprobarCantidad(t, pool, 0, `SELECT count(*) FROM notificacion WHERE hilo_id=$1 AND tipo='SUGERENCIA' AND estado='NO_LEIDA'`, sug.ID)
				comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE hilo_id=$1 AND tipo='SUGERENCIA' AND estado='NO_LEIDA'`, otro.ID)
				if _, err := svc.MarcarResuelta(ctx, otro.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := bus.Despachar(ctx, 10); err != nil {
					t.Fatal(err)
				}
				comprobarCantidad(t, pool, 0, `SELECT count(*) FROM notificacion WHERE tipo='SUGERENCIA' AND estado='NO_LEIDA'`)
				return
			}
			if caso == "consumidor-reintenta" {
				segundoAdmin := uuid.NewString()
				ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Segundo','Admin',$2,'hash','ADMIN','APROBADA')`, segundoAdmin, segundoAdmin+"@test.local")
				// Fallar al segundo destinatario comprueba rollback de un aviso ya creado.
				fallar := segundoAdmin
				if admin > segundoAdmin {
					fallar = admin
				}
				quitar := falloBajaSQL(t, pool, "notificacion", "INSERT", "WHEN (NEW.usuario_id='"+fallar+"')")
				n, err := bus.Despachar(ctx, 10)
				quitar()
				if err == nil || n != 0 {
					t.Fatalf("ocultó fallo de entrega: %d %v", n, err)
				}
				comprobarCantidad(t, pool, 0, `SELECT count(*) FROM notificacion`)
				comprobarCantidad(t, pool, 1, `SELECT count(*) FROM evento_pendiente WHERE procesado_en IS NULL AND intentos=1`)
				ejecutarCierreSQL(t, pool, `UPDATE evento_pendiente SET reintentar_en=now()`)
				bus = busDurableReal(t, pool)
				if n, err := bus.Despachar(ctx, 10); err != nil || n != 1 {
					t.Fatalf("reintento %d %v", n, err)
				}
				comprobarCantidad(t, pool, 2, `SELECT count(*) FROM notificacion WHERE hilo_id=$1`, sug.ID)
				if n, err := bus.Despachar(ctx, 10); err != nil || n != 0 {
					t.Fatalf("duplicó entrega %d %v", n, err)
				}
				return
			}
			if caso == "reinicio-y-dos-workers" {
				a, b := busDurableReal(t, pool), busDurableReal(t, pool)
				fin := make(chan error, 2)
				for _, bus := range []*eventbus.DurableBus{a, b} {
					go func(bus *eventbus.DurableBus) { _, err := bus.Despachar(ctx, 10); fin <- err }(bus)
				}
				for range 2 {
					if err := <-fin; err != nil {
						t.Fatal(err)
					}
				}
			} else if n, err := bus.Despachar(ctx, 10); err != nil || n != 1 {
				t.Fatalf("entrega %d %v", n, err)
			}
			comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE hilo_id=$1`, sug.ID)
			comprobarCantidad(t, pool, 1, `SELECT count(*) FROM evento_pendiente WHERE procesado_en IS NOT NULL`)
		})
	}
}

func TestAuditoriaTransaccional_ActorCamposYRollback(t *testing.T) {
	pool := baseVacia(t)
	e := prepararInstitucion(t, pool)
	ctx := pgtransaccion.ConActor(context.Background(), e.d.docente, "203.0.113.7")
	var equipo string
	if err := pool.QueryRow(ctx, `SELECT id FROM equipo LIMIT 1`).Scan(&equipo); err != nil {
		t.Fatal(err)
	}
	cifrador, err := secretos.Nuevo("clave-estable-para-cuentas-de-prueba-segura")
	if err != nil {
		t.Fatal(err)
	}
	inventario := inventoryapp.NewService(inventoryinfra.NewPostgresRepo(pool), nil, uuid.NewString, time.Now, cifrador, nil)
	cuenta, err := inventario.CrearCuentaDeEquipo(ctx, equipo, inventorydomain.DatosDeCuenta{Usuario: "Alumno", Clase: "Local", Privilegio: inventorydomain.PrivilegioComun, Visibilidad: inventorydomain.VisibilidadPublica, TienePassword: true}, "password-muy-privada")
	if err != nil {
		t.Fatal(err)
	}
	var actor, ip, detalle string
	if err := pool.QueryRow(ctx, `SELECT usuario_id,host(ip_origen),detalle::text FROM audit_log WHERE entidad='equipo_cuenta' AND entidad_id=$1 AND accion='CAMBIO_TRANSACCIONAL'`, cuenta.ID).Scan(&actor, &ip, &detalle); err != nil {
		t.Fatal(err)
	}
	if actor != e.d.docente || ip != "203.0.113.7" || !strings.Contains(detalle, "INSERT") || strings.Contains(detalle, "password-muy-privada") || strings.Contains(detalle, cuenta.PasswordCifrada) {
		t.Fatalf("atribución o filtrado: %s %s %s", actor, ip, detalle)
	}
	quitar := falloBajaSQL(t, pool, "audit_log", "INSERT", "")
	nombre := "Nombre nuevo"
	_, err = inventario.EditarCuentaDeEquipo(ctx, cuenta.ID, inventoryapp.EditarCuentaParams{Usuario: &nombre})
	quitar()
	if err == nil {
		t.Fatal("confirmó el cambio sin auditoría")
	}
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM equipo_cuenta WHERE id=$1 AND usuario='Alumno'`, cuenta.ID)
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM audit_log WHERE entidad='equipo_cuenta' AND entidad_id=$1`, cuenta.ID)
}
