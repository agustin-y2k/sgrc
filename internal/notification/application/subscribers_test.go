package application

import (
	"strings"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

func TestRegisterEventHandlers_DocenteRegistroPendiente_NotificaATodosLosAdmins(t *testing.T) {
	repo := nuevoFakeRepo()
	listador := &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2"}}
	svc := nuevoServicioDeTest(repo, listador)
	bus := eventbus.NewInMemoryEventBus()
	// Sincrónico a propósito: en producción la entrega es asincrónica para
	// no alargar el request, pero acá se necesita determinismo.
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{
		Tipo: "docente.registro.pendiente",
		Payload: map[string]string{
			"usuarioId": "d1", "nombre": "Ada", "apellido": "Lovelace", "email": "ada@x.com",
		},
	})

	esperarNotificaciones(t, repo, 2)

	for _, n := range repo.notificaciones {
		if n.UsuarioID != "admin1" && n.UsuarioID != "admin2" {
			t.Errorf("notificación con usuarioId inesperado: %s", n.UsuarioID)
		}
	}
}

func TestRegisterEventHandlers_DocenteBajaMateriaHuerfana_NotificaATodosLosAdmins(t *testing.T) {
	repo := nuevoFakeRepo()
	listador := &fakeListadorAdmins{adminIDs: []string{"admin1"}}
	svc := nuevoServicioDeTest(repo, listador)
	bus := eventbus.NewInMemoryEventBus()
	// Sincrónico a propósito: en producción la entrega es asincrónica para
	// no alargar el request, pero acá se necesita determinismo.
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{
		Tipo: "docente.baja.materia-huerfana",
		Payload: map[string]any{
			"usuarioId": "d1", "materiaId": "m1", "reservasCanceladas": 3,
		},
	})

	esperarNotificaciones(t, repo, 1)
}

// El otro camino a una materia huérfana (RF-02.8): no se dio de baja a
// nadie, se le quitó la asignación al último docente.
func TestRegisterEventHandlers_DocenteDesasignadoMateriaHuerfana_NotificaATodosLosAdmins(t *testing.T) {
	repo := nuevoFakeRepo()
	listador := &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2"}}
	svc := nuevoServicioDeTest(repo, listador)
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{
		Tipo: "docente.desasignado.materia-huerfana",
		Payload: map[string]any{
			"usuarioId": "d1", "materiaId": "m1", "reservasCanceladas": 2,
		},
	})

	esperarNotificaciones(t, repo, 2)
	for _, n := range repo.notificaciones {
		if !strings.Contains(n.Mensaje, "se quitó al último docente") {
			t.Errorf("el mensaje no distingue este caso del de la baja: %q", n.Mensaje)
		}
	}
}

func TestRegisterEventHandlers_DocenteBajaNotificarAdmin_NotificaATodosLosAdmins(t *testing.T) {
	repo := nuevoFakeRepo()
	listador := &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2"}}
	svc := nuevoServicioDeTest(repo, listador)
	bus := eventbus.NewInMemoryEventBus()
	// Sincrónico a propósito: en producción la entrega es asincrónica para
	// no alargar el request, pero acá se necesita determinismo.
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{
		Tipo:    "docente.baja.notificar_admin",
		Payload: map[string]any{"usuarioId": "d1", "materiaId": "m1"},
	})

	esperarNotificaciones(t, repo, 2)
}

func TestRegisterEventHandlers_ReservaCancelada_NotificaAlDocenteAfectado(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})
	bus := eventbus.NewInMemoryEventBus()
	// Sincrónico a propósito: en producción la entrega es asincrónica para
	// no alargar el request, pero acá se necesita determinismo.
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{
		Tipo: "reserva.cancelada",
		Payload: eventbus.CancelacionesDeUsuario{
			UsuarioID: "docente1",
			Motivo:    "PC rota",
			Reservas: []eventbus.ReservaCancelada{
				{ReservaID: "r1", PCIdentificador: 7, Fecha: fecha(2026, 9, 10)},
			},
		},
	})

	esperarNotificaciones(t, repo, 1)

	for _, n := range repo.notificaciones {
		if n.UsuarioID != "docente1" {
			t.Errorf("esperaba notificar a docente1, notificó a %s", n.UsuarioID)
		}
		// Con una sola reserva sí hay "la" reserva a la que apuntar.
		if n.ReservaID == nil || *n.ReservaID != "r1" {
			t.Errorf("ReservaID incorrecto: %v", n.ReservaID)
		}
		if n.Tipo != domain.TipoReservaCancelada {
			t.Errorf("tipo incorrecto: %q", n.Tipo)
		}
		// El prefijo se pone una sola vez, acá. Quien publica manda la
		// razón pelada: si además armara la frase, el docente leía
		// "Tu reserva fue cancelada: Tu reserva fue cancelada: …".
		esperado := "Tu reserva del 10/09/2026 (PC 7) fue cancelada: PC rota"
		if n.Mensaje != esperado {
			t.Errorf("mensaje incorrecto:\n  esperado %q\n  obtenido %q", esperado, n.Mensaje)
		}
	}
}

// El caso que motivó agrupar: un Admin bloquea tres PCs de la misma reserva
// para una evaluación y el docente recibía tres avisos idénticos.
func TestRegisterEventHandlers_ReservaCancelada_VariasPCs_UnSoloAviso(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{
		Tipo: "reserva.cancelada",
		Payload: eventbus.CancelacionesDeUsuario{
			UsuarioID: "docente1",
			Motivo:    "bloqueo por evaluación estatal (Aprender 2026)",
			Reservas: []eventbus.ReservaCancelada{
				{ReservaID: "r1", PCIdentificador: 7, Fecha: fecha(2026, 9, 10)},
				{ReservaID: "r2", PCIdentificador: 3, Fecha: fecha(2026, 9, 10)},
				{ReservaID: "r3", PCIdentificador: 12, Fecha: fecha(2026, 9, 10)},
			},
		},
	})

	esperarNotificaciones(t, repo, 1)

	for _, n := range repo.notificaciones {
		// Las PCs salen ordenadas, no en el orden en que se cancelaron.
		esperado := "Se cancelaron 3 de tus reservas del 10/09/2026 (PC 3, PC 7, PC 12): bloqueo por evaluación estatal (Aprender 2026)"
		if n.Mensaje != esperado {
			t.Errorf("mensaje incorrecto:\n  esperado %q\n  obtenido %q", esperado, n.Mensaje)
		}
		// Con varias reservas no hay una sola a la que apuntar.
		if n.ReservaID != nil {
			t.Errorf("un aviso agrupado no debería colgar de una reserva puntual: %v", *n.ReservaID)
		}
	}
}

// Una recurrencia cancelada abarca varias fechas: ahí la fecha no se puede
// nombrar una sola vez.
func TestRegisterEventHandlers_ReservaCancelada_VariasFechas(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{
		Tipo: "reserva.cancelada",
		Payload: eventbus.CancelacionesDeUsuario{
			UsuarioID: "docente1",
			Motivo:    "la PC 7 pasó a FUERA_DE_SERVICIO",
			Reservas: []eventbus.ReservaCancelada{
				{ReservaID: "r1", PCIdentificador: 7, Fecha: fecha(2026, 9, 10)},
				{ReservaID: "r2", PCIdentificador: 7, Fecha: fecha(2026, 9, 17)},
			},
		},
	})

	esperarNotificaciones(t, repo, 1)

	for _, n := range repo.notificaciones {
		esperado := "Se cancelaron 2 de tus reservas (PC 7): la PC 7 pasó a FUERA_DE_SERVICIO"
		if n.Mensaje != esperado {
			t.Errorf("mensaje incorrecto:\n  esperado %q\n  obtenido %q", esperado, n.Mensaje)
		}
	}
}

// Si reservation no pudo resolver los identificadores, el aviso sale igual
// sin nombrar las PCs.
func TestRegisterEventHandlers_ReservaCancelada_SinIdentificadores(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{
		Tipo: "reserva.cancelada",
		Payload: eventbus.CancelacionesDeUsuario{
			UsuarioID: "docente1",
			Motivo:    "PC rota",
			Reservas: []eventbus.ReservaCancelada{
				{ReservaID: "r1", Fecha: fecha(2026, 9, 10)},
				{ReservaID: "r2", Fecha: fecha(2026, 9, 10)},
			},
		},
	})

	esperarNotificaciones(t, repo, 1)

	for _, n := range repo.notificaciones {
		esperado := "Se cancelaron 2 de tus reservas del 10/09/2026 (2 PCs): PC rota"
		if n.Mensaje != esperado {
			t.Errorf("mensaje incorrecto:\n  esperado %q\n  obtenido %q", esperado, n.Mensaje)
		}
	}
}

// El texto es lo único que ve el docente, así que se fija acá: sin este
// caso, un publisher que mande la frase entera pasa desapercibido.
func TestRegisterEventHandlers_ReservaCancelada_NoRepiteElPrefijo(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{
		Tipo: "reserva.cancelada",
		Payload: eventbus.CancelacionesDeUsuario{
			UsuarioID: "docente1",
			Motivo:    "bloqueo por evaluación estatal (Aprender 2026)",
			Reservas: []eventbus.ReservaCancelada{
				{ReservaID: "r1", PCIdentificador: 7, Fecha: fecha(2026, 9, 10)},
			},
		},
	})

	esperarNotificaciones(t, repo, 1)

	esperado := "Tu reserva del 10/09/2026 (PC 7) fue cancelada: bloqueo por evaluación estatal (Aprender 2026)"
	for _, n := range repo.notificaciones {
		if n.Mensaje != esperado {
			t.Errorf("mensaje incorrecto:\n  esperado %q\n  obtenido %q", esperado, n.Mensaje)
		}
	}
}

func TestRegisterEventHandlers_PayloadInesperado_NoPanikea(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})
	bus := eventbus.NewInMemoryEventBus()
	// Sincrónico a propósito: en producción la entrega es asincrónica para
	// no alargar el request, pero acá se necesita determinismo.
	RegisterEventHandlersSincronos(bus, svc)

	// Publicar con un payload de tipo incorrecto (no el esperado por el
	// handler) — no debería panickear, el eventbus ya recupera panics de
	// handlers individuales, pero acá confirmamos que el propio handler
	// se cuida solo con el type assertion seguro (ok, no panic directo).
	bus.Publish(eventbus.Evento{Tipo: "docente.registro.pendiente", Payload: "esto no es un map"})
	bus.Publish(eventbus.Evento{Tipo: "reserva.cancelada", Payload: 12345})

	time.Sleep(50 * time.Millisecond)
	if len(repo.notificaciones) != 0 {
		t.Errorf("un payload inválido no debería haber creado ninguna notificación: %d", len(repo.notificaciones))
	}
}

// esperarNotificaciones sondea el repo fake hasta que aparezca la
// cantidad esperada de notificaciones, o falla tras un timeout corto —
// necesario porque eventbus.Publish corre los handlers síncronamente en
// la goroutine del publisher, así que en la práctica esto ya está listo
// apenas Publish retorna, pero sondear es más robusto que asumirlo.
func esperarNotificaciones(t *testing.T, repo *fakeRepo, esperadas int) {
	t.Helper()
	limite := time.Now().Add(time.Second)
	for time.Now().Before(limite) {
		if len(repo.notificaciones) == esperadas {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("esperaba %d notificaciones, hay %d", esperadas, len(repo.notificaciones))
}

func fecha(anio int, mes time.Month, dia int) time.Time {
	return time.Date(anio, mes, dia, 0, 0, 0, 0, time.UTC)
}

// El aviso "X está pendiente de aprobación" pide una acción concreta. Una
// vez que alguien la hizo, seguir viéndolo sin leer manda al Admin a una
// lista donde esa persona ya no está.
func TestRegisterEventHandlers_CuentaResuelta_CierraElAvisoDePendiente(t *testing.T) {
	repo := nuevoFakeRepo()
	listador := &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2"}}
	svc := nuevoServicioDeTest(repo, listador)
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	// Se registra: los dos Admin reciben el aviso.
	bus.Publish(eventbus.Evento{
		Tipo: "docente.registro.pendiente",
		Payload: map[string]string{
			"usuarioId": "docente-nuevo", "nombre": "Ada", "apellido": "Lovelace",
		},
	})
	esperarNotificaciones(t, repo, 2)

	// Un Admin lo aprueba: el aviso se cierra para LOS DOS.
	bus.Publish(eventbus.Evento{
		Tipo:    "cuenta.pendiente.resuelta",
		Payload: map[string]string{"usuarioId": "docente-nuevo"},
	})

	for _, n := range repo.notificaciones {
		if n.Estado != domain.Leida {
			t.Errorf("el aviso de %s quedó sin leer después de resolver la cuenta", n.UsuarioID)
		}
	}
}

// Solo se cierra el aviso de esa persona: si se cerraran todos los
// pendientes, aprobar a uno haría desaparecer los avisos de los demás.
func TestRegisterEventHandlers_CuentaResuelta_NoTocaLosAvisosDeOtros(t *testing.T) {
	repo := nuevoFakeRepo()
	listador := &fakeListadorAdmins{adminIDs: []string{"admin1"}}
	svc := nuevoServicioDeTest(repo, listador)
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	for _, id := range []string{"docente-a", "docente-b"} {
		bus.Publish(eventbus.Evento{
			Tipo: "docente.registro.pendiente",
			Payload: map[string]string{
				"usuarioId": id, "nombre": "Ada", "apellido": id,
			},
		})
	}
	esperarNotificaciones(t, repo, 2)

	bus.Publish(eventbus.Evento{
		Tipo:    "cuenta.pendiente.resuelta",
		Payload: map[string]string{"usuarioId": "docente-a"},
	})

	leidas, sinLeer := 0, 0
	for _, n := range repo.notificaciones {
		if n.Estado == domain.Leida {
			leidas++
		} else {
			sinLeer++
		}
	}
	if leidas != 1 || sinLeer != 1 {
		t.Errorf("esperaba cerrar solo uno: %d leídas, %d sin leer", leidas, sinLeer)
	}
}
