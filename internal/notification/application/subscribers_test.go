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
				{ReservaID: "r1", Etiqueta: "PC 7", Fecha: fecha(2026, 9, 10)},
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
		// El prefijo se pone una sola vez, acá.
		esperado := "Tu reserva del 10/09/2026 (PC 7) fue cancelada: PC rota"
		if n.Mensaje != esperado {
			t.Errorf("mensaje incorrecto:\n  esperado %q\n  obtenido %q", esperado, n.Mensaje)
		}
	}
}

// El caso que motivó agrupar: un Admin bloquea tres PCs de la misma reserva
// para una evaluación y el docente recibía tres avisos idénticos.
func TestRegisterEventHandlers_ReservaCancelada_VariasEquipos_UnSoloAviso(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{
		Tipo: "reserva.cancelada",
		Payload: eventbus.CancelacionesDeUsuario{
			UsuarioID: "docente1",
			Motivo:    "bloqueo administrativo (Aprender 2026)",
			Reservas: []eventbus.ReservaCancelada{
				{ReservaID: "r1", Etiqueta: "PC 7", Fecha: fecha(2026, 9, 10)},
				{ReservaID: "r2", Etiqueta: "PC 3", Fecha: fecha(2026, 9, 10)},
				{ReservaID: "r3", Etiqueta: "PC 12", Fecha: fecha(2026, 9, 10)},
			},
		},
	})

	esperarNotificaciones(t, repo, 1)

	for _, n := range repo.notificaciones {
		// Las PCs salen ordenadas, no en el orden en que se cancelaron.
		esperado := "Se cancelaron 3 de tus reservas del 10/09/2026 (PC 3, PC 7, PC 12): bloqueo administrativo (Aprender 2026)"
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
				{ReservaID: "r1", Etiqueta: "PC 7", Fecha: fecha(2026, 9, 10)},
				{ReservaID: "r2", Etiqueta: "PC 7", Fecha: fecha(2026, 9, 17)},
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
		esperado := "Se cancelaron 2 de tus reservas del 10/09/2026 (2 equipos): PC rota"
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
			Motivo:    "bloqueo administrativo (Aprender 2026)",
			Reservas: []eventbus.ReservaCancelada{
				{ReservaID: "r1", Etiqueta: "PC 7", Fecha: fecha(2026, 9, 10)},
			},
		},
	})

	esperarNotificaciones(t, repo, 1)

	esperado := "Tu reserva del 10/09/2026 (PC 7) fue cancelada: bloqueo administrativo (Aprender 2026)"
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

	// Publicar con un payload de tipo incorrecto (no el esperado por el handler)
	// — no debería panickear, el eventbus ya recupera panics de handlers
	// individuales, pero acá confirmamos que el propio handler se cuida solo con
	// el type assertion seguro (ok, no panic directo).
	bus.Publish(eventbus.Evento{Tipo: "docente.registro.pendiente", Payload: "esto no es un map"})
	bus.Publish(eventbus.Evento{Tipo: "reserva.cancelada", Payload: 12345})

	time.Sleep(50 * time.Millisecond)
	if len(repo.notificaciones) != 0 {
		t.Errorf("un payload inválido no debería haber creado ninguna notificación: %d", len(repo.notificaciones))
	}
}

// esperarNotificaciones sondea el repo fake hasta que aparezca la cantidad
// esperada de notificaciones, o falla tras un timeout corto — necesario
// porque eventbus.Publish corre los handlers síncronamente en la goroutine
// del publisher, así que en la práctica esto ya está listo apenas Publish
// retorna, pero sondear es más robusto que asumirlo.
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

// El aviso "X está pendiente de aprobación" pide una acción concreta.
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

// RF-05.9: el único aviso del sistema que nace de un reloj y no de que
// alguien haya hecho algo.
func TestRegisterEventHandlers_LicenciaPorVencer_NotificaATodosLosAdmins(t *testing.T) {
	repo := nuevoFakeRepo()
	listador := &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2"}}
	svc := nuevoServicioDeTest(repo, listador)
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{
		Tipo: "licencia.por-vencer",
		Payload: eventbus.AvisoDeLicencias{
			PorVencer: []eventbus.LicenciaPorVencer{{
				Nombre: "AutoCAD 2027", Identificador: 3, CarroNombre: "Carro 1",
				FechaVencimiento: time.Date(2026, time.September, 3, 0, 0, 0, 0, time.UTC),
				DiasRestantes:    1,
			}},
		},
	})

	esperarNotificaciones(t, repo, 2)
	for _, n := range repo.notificaciones {
		// El tipo es lo que le da a la pantalla el botón hacia la lista de
		// licencias, sin tener que adivinar leyendo el mensaje.
		if n.Tipo != domain.TipoLicenciaPorVencer {
			t.Errorf("tipo = %q, esperaba %q", n.Tipo, domain.TipoLicenciaPorVencer)
		}
		if !strings.Contains(n.Mensaje, "AutoCAD 2027") {
			t.Errorf("el mensaje no dice de qué licencia habla: %q", n.Mensaje)
		}
	}
}

// Un aviso vacío no puede dejar un "0 licencias necesitan atención" en la
// campana de cada Admin.
func TestRegisterEventHandlers_LicenciaPorVencer_AvisoVacioNoNotifica(t *testing.T) {
	repo := nuevoFakeRepo()
	listador := &fakeListadorAdmins{adminIDs: []string{"admin1"}}
	svc := nuevoServicioDeTest(repo, listador)
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{Tipo: "licencia.por-vencer", Payload: eventbus.AvisoDeLicencias{}})

	if len(repo.notificaciones) != 0 {
		t.Errorf("no debería crear notificaciones, creó %d", len(repo.notificaciones))
	}
}

// ── El barrido de reservas y entregas (RF-08.10 a RF-08.13) ────────────

// El recordatorio NO escribe en la campana (1.18.0): sale solo como correo, y
// apagado por defecto. Era el aviso de mayor volumen del sistema —uno por
// clase y por día— y el único que no traía ninguna noticia: le contaba al
// docente algo que él mismo cargó.
func TestRegisterEventHandlers_Recordatorio_NoEscribeEnLaCampana(t *testing.T) {
	repo := nuevoFakeRepo()
	listador := &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2"}}
	svc := nuevoServicioDeTest(repo, listador)
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{Tipo: "reserva.recordatorio", Payload: eventbus.RecordatorioDeReserva{
		UsuarioID: "docente1", Nombre: "Ada", MateriaNombre: "Matemáticas",
		HoraInicio: 8 * time.Hour, Equipos: []string{"PC 1", "PC 2"}, MinutosDeGracia: 40,
	}})

	if len(repo.notificaciones) != 0 {
		t.Errorf("el recordatorio no va a la campana, se crearon %d avisos: %+v",
			len(repo.notificaciones), repo.notificaciones)
	}
}

// El corte avisa SOLO a los Admin. Al docente de la próxima reserva no: el
// corte sale de noche, cuando ya no puede conseguir otra máquina, y su aviso
// le llega una hora antes de la clase por reserva.equipo-no-disponible.
func TestRegisterEventHandlers_Cierre_AvisaSoloALosAdmins(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{adminIDs: []string{"admin1"}})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.EquiposSinDevolverAlCierre{
		Equipos: []eventbus.EquipoSinDevolverAlCierre{{
			Etiqueta: "PC 3", Quien: "Marta",
			ProximoUsuarioID: "docente2",
			ProximaFecha:     time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC),
		}},
	}})

	esperarNotificaciones(t, repo, 1)
	for _, n := range repo.notificaciones {
		if n.UsuarioID != "admin1" {
			t.Errorf("el corte es de los Admin, le llegó a %q", n.UsuarioID)
		}
	}
}

// TestEquiposDeLasCanceladas_OrdenNatural: con sort.Strings, "PC 12" iba
// antes que "PC 3" porque compara carácter por carácter.
func TestListaDeEquipos_OrdenNatural(t *testing.T) {
	reservas := []eventbus.ReservaCancelada{
		{Etiqueta: "PC 12"}, {Etiqueta: "PC 3"}, {Etiqueta: "PC 7"},
	}

	if got := equiposDeLasCanceladas(reservas); got != "PC 3, PC 7, PC 12" {
		t.Errorf("equiposDeLasCanceladas = %q, esperaba orden natural", got)
	}
}

// Y con un equipo suelto en la mezcla, que no tiene número.
func TestListaDeEquipos_ConUnEquipoSuelto(t *testing.T) {
	reservas := []eventbus.ReservaCancelada{
		{Etiqueta: "Proyector Epson"}, {Etiqueta: "PC 3"},
	}

	if got := equiposDeLasCanceladas(reservas); got != "PC 3, Proyector Epson" {
		t.Errorf("equiposDeLasCanceladas = %q", got)
	}
}

// ── Un aviso a todos los Admin no puede ser trabajo para todos ──────────

// El ida y vuelta del buzón dejaba un aviso por mensaje en la campana de CADA
// Admin, y ninguno decía nada que el primero no dijera ya.
func TestRegisterEventHandlers_ElSeguimientoNoRepiteElAviso(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2"}})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{Tipo: "sugerencia.nueva", Payload: eventbus.SugerenciaNueva{
		SugerenciaID: "s1", UsuarioID: "docente1", Quien: "Ada", Tipo: "PROBLEMA",
		Asunto: "la PC 3", Texto: "no prende",
	}})
	esperarNotificaciones(t, repo, 2)

	// Tres mensajes más en el mismo hilo, sin que ningún Admin los haya leído.
	for i := 0; i < 3; i++ {
		bus.Publish(eventbus.Evento{Tipo: "sugerencia.seguimiento", Payload: eventbus.SugerenciaSeguimiento{
			SugerenciaID: "s1", UsuarioID: "docente1", Quien: "Ada", Tipo: "PROBLEMA",
			Asunto: "la PC 3", Texto: "sigue sin prender",
		}})
	}

	if len(repo.notificaciones) != 2 {
		t.Errorf("con uno sin leer por Admin alcanza, hay %d avisos", len(repo.notificaciones))
	}
}

// Lo resuelve uno: deja de estar pendiente para los demás. Sin esto, el que
// contesta tacha el suyo y los otros se quedan con un aviso sin leer para
// siempre sobre algo que ya no existe.
func TestRegisterEventHandlers_ResponderElBuzonCierraElAvisoDeTodos(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2"}})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{Tipo: "sugerencia.nueva", Payload: eventbus.SugerenciaNueva{
		SugerenciaID: "s1", UsuarioID: "docente1", Quien: "Ada", Tipo: "PROBLEMA",
		Asunto: "la PC 3", Texto: "no prende",
	}})
	esperarNotificaciones(t, repo, 2)

	bus.Publish(eventbus.Evento{Tipo: "sugerencia.respondida", Payload: eventbus.SugerenciaRespondida{
		SugerenciaID: "s1", UsuarioID: "docente1", Nombre: "Ada", Tipo: "PROBLEMA",
		Asunto: "la PC 3", TextoOriginal: "no prende", Respuesta: "ya la vemos",
	}})
	esperarNotificaciones(t, repo, 3)

	for _, n := range repo.notificaciones {
		if n.Tipo == domain.TipoSugerencia && n.Estado != domain.Leida {
			t.Errorf("el aviso de %s tendría que haberse cerrado solo", n.UsuarioID)
		}
	}
}

func TestRegisterEventHandlers_ResolverElPedidoDeMateriaCierraElAvisoDeTodos(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2"}})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{Tipo: "materia.pedido.nuevo", Payload: eventbus.PedidoDeMateriaNuevo{
		PedidoID: "p1", UsuarioID: "docente1", Nombre: "Ada", MateriaNombre: "Física",
	}})
	esperarNotificaciones(t, repo, 2)

	bus.Publish(eventbus.Evento{Tipo: "materia.pedido.resuelto", Payload: eventbus.PedidoDeMateriaResuelto{
		PedidoID: "p1", UsuarioID: "docente1", Nombre: "Ada", MateriaNombre: "Física", Aprobado: true,
	}})
	esperarNotificaciones(t, repo, 3)

	for _, n := range repo.notificaciones {
		if n.Tipo == domain.TipoPedidoDeMateria && n.Estado != domain.Leida {
			t.Errorf("el pedido de %s tendría que haberse cerrado solo", n.UsuarioID)
		}
	}
}

// A quien ya dicta la materia no se le avisa nada: no se le pide ninguna
// acción y no puede hacer nada con la noticia.
func TestRegisterEventHandlers_PedidoDeMateria_NoAvisaAlTitular(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{adminIDs: []string{"admin1"}})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{Tipo: "materia.pedido.nuevo", Payload: eventbus.PedidoDeMateriaNuevo{
		PedidoID: "p1", UsuarioID: "docente1", Nombre: "Ada", MateriaNombre: "Física",
		DocentesActuales: []eventbus.DocenteDeMateria{
			{UsuarioID: "docente-titular", Email: "titular@escuela.edu.ar"},
		},
	}})
	esperarNotificaciones(t, repo, 1)

	for _, n := range repo.notificaciones {
		if n.UsuarioID == "docente-titular" {
			t.Error("al titular no se le avisa: no se le pide nada")
		}
	}
}

// ── El cierre de los avisos que hablan de un conjunto ────────────────────
//
// Licencias y equipos afuera no hablan de una persona sino de varias cosas, y
// el conjunto se rearma cada vez: las licencias no vencen todas el mismo día.
// Por eso el cierre no es "se renovó una" sino "ya no queda ninguna".

func TestRegisterEventHandlers_LicenciasRenovadas_CierraElAvisoCuandoNoQuedaNinguna(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2"}})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{Tipo: "licencia.por-vencer", Payload: eventbus.AvisoDeLicencias{
		PorVencer: []eventbus.LicenciaPorVencer{
			{Nombre: "AutoCAD", Etiqueta: "PC 3", DiasRestantes: 1},
			{Nombre: "AutoCAD", Etiqueta: "PC 7", DiasRestantes: 1},
		},
	}})
	esperarNotificaciones(t, repo, 2)

	// Se renovó una de las dos: todavía queda trabajo, el aviso sigue en pie.
	bus.Publish(eventbus.Evento{Tipo: "licencia.pendientes",
		Payload: eventbus.PendientesDeLicencia{Pendientes: 1}})
	for _, n := range repo.notificaciones {
		if n.Tipo == domain.TipoLicenciaPorVencer && n.Estado != domain.NoLeida {
			t.Fatal("todavía queda una licencia por renovar: el aviso no se puede cerrar")
		}
	}

	// Se renovó la última: ya no hay a qué apuntar.
	bus.Publish(eventbus.Evento{Tipo: "licencia.pendientes",
		Payload: eventbus.PendientesDeLicencia{Pendientes: 0}})
	for _, n := range repo.notificaciones {
		if n.Tipo == domain.TipoLicenciaPorVencer && n.Estado != domain.Leida {
			t.Errorf("no queda ninguna pendiente: el aviso de %s tendría que haberse cerrado", n.UsuarioID)
		}
	}
}

func TestRegisterEventHandlers_EquiposRecibidos_CierraElAvisoDeCierre(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2"}})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.EquiposSinDevolverAlCierre{
		Equipos: []eventbus.EquipoSinDevolverAlCierre{
			{Etiqueta: "PC 3", Quien: "Marta"},
			{Etiqueta: "PC 7", Quien: "Marta"},
		},
	}})
	esperarNotificaciones(t, repo, 2)

	// Recibieron una de las dos: la otra sigue afuera.
	bus.Publish(eventbus.Evento{Tipo: "prestamo.cierre.pendientes",
		Payload: eventbus.PendientesDelCierre{Pendientes: 1}})
	for _, n := range repo.notificaciones {
		if n.Tipo == domain.TipoEquipoSinDevolver && n.Estado != domain.NoLeida {
			t.Fatal("todavía hay un equipo afuera: el aviso no se puede cerrar")
		}
	}

	// Volvió la última.
	bus.Publish(eventbus.Evento{Tipo: "prestamo.cierre.pendientes",
		Payload: eventbus.PendientesDelCierre{Pendientes: 0}})
	for _, n := range repo.notificaciones {
		if n.Tipo == domain.TipoEquipoSinDevolver && n.Estado != domain.Leida {
			t.Errorf("volvió todo: el aviso de %s tendría que haberse cerrado", n.UsuarioID)
		}
	}
}

// El cierre es POR TIPO, así que no puede llevarse puesto lo demás: un aviso
// de cuenta pendiente sin leer tiene que seguir sin leer.
func TestRegisterEventHandlers_ElCierrePorTipoNoTocaLosOtrosAvisos(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{adminIDs: []string{"admin1"}})
	bus := eventbus.NewInMemoryEventBus()
	RegisterEventHandlersSincronos(bus, svc)

	bus.Publish(eventbus.Evento{Tipo: "licencia.por-vencer", Payload: eventbus.AvisoDeLicencias{
		PorVencer: []eventbus.LicenciaPorVencer{{Nombre: "AutoCAD", Etiqueta: "PC 3", DiasRestantes: 1}},
	}})
	bus.Publish(eventbus.Evento{Tipo: "docente.registro.pendiente", Payload: map[string]string{
		"usuarioId": "docente1", "nombre": "Ada", "apellido": "Lovelace", "email": "ada@escuela.edu.ar",
	}})
	esperarNotificaciones(t, repo, 2)

	bus.Publish(eventbus.Evento{Tipo: "licencia.pendientes",
		Payload: eventbus.PendientesDeLicencia{Pendientes: 0}})

	for _, n := range repo.notificaciones {
		if n.Tipo == domain.TipoDocentePendiente && n.Estado != domain.NoLeida {
			t.Error("la cuenta sigue esperando aprobación: ese aviso no se toca")
		}
	}
}
