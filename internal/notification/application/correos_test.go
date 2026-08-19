package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// ── fakeEnviador ────────────────────────────────────────────────────────

type mailEnviado struct{ para, asunto, cuerpo string }

type fakeEnviador struct {
	enviados []mailEnviado
	err      error
}

func (f *fakeEnviador) Enviar(_ context.Context, para, asunto, cuerpo string) error {
	if f.err != nil {
		return f.err
	}
	f.enviados = append(f.enviados, mailEnviado{para, asunto, cuerpo})
	return nil
}

const urlDePrueba = "https://sgrc.escuela.edu.ar"

// mensajeroDePrueba arma el Mensajero ya suscrito al bus, en modo
// sincrónico para poder afirmar sin esperas.
func mensajeroDePrueba(admins ...string) (*eventbus.InMemoryEventBus, *fakeEnviador) {
	bus := eventbus.NewInMemoryEventBus()
	enviador := &fakeEnviador{}
	m := NewMensajero(enviador, &fakeListadorAdmins{adminEmails: admins}, &fakePreferencias{siempreSi: true}, urlDePrueba)
	RegisterEmailHandlersSincronos(bus, m)
	return bus, enviador
}

// ── Docente pendiente → Admins ──────────────────────────────────────────

func TestCorreo_DocentePendiente_LlegaATodosLosAdmins(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar", "admin2@escuela.edu.ar")

	bus.Publish(eventbus.Evento{
		Tipo: "docente.registro.pendiente",
		Payload: map[string]string{
			"usuarioId": "usr-1",
			"nombre":    "Ana",
			"apellido":  "Pérez",
			"email":     "ana@escuela.edu.ar",
		},
	})

	if len(enviador.enviados) != 2 {
		t.Fatalf("esperaba 2 mails (uno por Admin), hubo %d", len(enviador.enviados))
	}
	cuerpo := enviador.enviados[0].cuerpo
	// El Admin tiene que poder decidir sin entrar: quién se registró y con
	// qué dirección.
	if !strings.Contains(cuerpo, "Ana Pérez") {
		t.Errorf("el cuerpo no dice quién se registró:\n%s", cuerpo)
	}
	if !strings.Contains(cuerpo, "ana@escuela.edu.ar") {
		t.Errorf("el cuerpo no trae el email declarado:\n%s", cuerpo)
	}
	if !strings.Contains(cuerpo, urlDePrueba) {
		t.Errorf("falta el enlace al sistema:\n%s", cuerpo)
	}
}

func TestCorreo_DocentePendiente_SinAdminsNoMandaNada(t *testing.T) {
	bus, enviador := mensajeroDePrueba()

	bus.Publish(eventbus.Evento{
		Tipo:    "docente.registro.pendiente",
		Payload: map[string]string{"nombre": "Ana", "apellido": "Pérez", "email": "ana@escuela.edu.ar"},
	})

	if len(enviador.enviados) != 0 {
		t.Fatalf("sin Admins no hay a quién avisarle, se mandaron %d", len(enviador.enviados))
	}
}

func TestCorreo_DocentePendiente_UnEnvioQueFallaNoFrenaLosOtros(t *testing.T) {
	// El enviador falla con una dirección puntual y anda con las demás.
	enviador := &enviadorQueFallaCon{malo: "admin2@escuela.edu.ar"}
	bus := eventbus.NewInMemoryEventBus()
	m := NewMensajero(enviador, &fakeListadorAdmins{
		adminEmails: []string{"admin1@escuela.edu.ar", "admin2@escuela.edu.ar", "admin3@escuela.edu.ar"},
	}, &fakePreferencias{}, urlDePrueba)
	RegisterEmailHandlersSincronos(bus, m)

	bus.Publish(eventbus.Evento{
		Tipo:    "docente.registro.pendiente",
		Payload: map[string]string{"nombre": "Ana", "apellido": "Pérez", "email": "ana@escuela.edu.ar"},
	})

	// Que rebote la casilla de uno no es razón para que los otros dos se
	// queden sin enterarse.
	if len(enviador.ok) != 2 {
		t.Fatalf("esperaba que los otros 2 Admins recibieran el aviso, recibieron %d", len(enviador.ok))
	}
}

type enviadorQueFallaCon struct {
	malo string
	ok   []string
}

func (e *enviadorQueFallaCon) Enviar(_ context.Context, para, _, _ string) error {
	if para == e.malo {
		return errors.New("casilla inexistente")
	}
	e.ok = append(e.ok, para)
	return nil
}

// ── Cuenta aprobada → la persona ────────────────────────────────────────

func TestCorreo_CuentaAprobada_LeLlegaALaPersona(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin@escuela.edu.ar")

	bus.Publish(eventbus.Evento{
		Tipo: "cuenta.aprobada",
		Payload: eventbus.CuentaAprobada{
			UsuarioID: "usr-1", Email: "ana@escuela.edu.ar", Nombre: "Ana",
		},
	})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 mail, hubo %d", len(enviador.enviados))
	}
	mail := enviador.enviados[0]
	if mail.para != "ana@escuela.edu.ar" {
		t.Errorf("le llegó a %q", mail.para)
	}
	if !strings.Contains(mail.cuerpo, "Hola Ana:") {
		t.Errorf("esperaba el saludo con el nombre:\n%s", mail.cuerpo)
	}
	if !strings.Contains(mail.cuerpo, urlDePrueba) {
		t.Errorf("decirle que la aprobaron sin decirle a dónde entrar no sirve:\n%s", mail.cuerpo)
	}
	// La cuenta puede haberse creado con Google y no tener contraseña: el
	// texto no puede dar por hecho que existe una.
	if strings.Contains(mail.cuerpo, "tu contraseña") {
		t.Errorf("el texto no puede hablar de una contraseña que quizás no exista:\n%s", mail.cuerpo)
	}
}

// ── Código de recuperación ──────────────────────────────────────────────

func TestCorreo_CodigoDeRecuperacion_LlevaElCodigoYLaVigencia(t *testing.T) {
	bus, enviador := mensajeroDePrueba()

	bus.Publish(eventbus.Evento{
		Tipo: "password.recuperacion.solicitada",
		Payload: eventbus.DatosDeRecuperacion{
			Email: "ana@escuela.edu.ar", Nombre: "Ana", Codigo: "482913", MinutosDeVigencia: 15,
		},
	})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 mail, hubo %d", len(enviador.enviados))
	}
	cuerpo := enviador.enviados[0].cuerpo
	if !strings.Contains(cuerpo, "482913") {
		t.Errorf("falta el código:\n%s", cuerpo)
	}
	if !strings.Contains(cuerpo, "15 minutos") {
		t.Errorf("falta cuánto dura:\n%s", cuerpo)
	}
	// Quien recibe esto sin haberlo pedido tiene que saber que no tiene que
	// hacer nada — si no, el mail parece un problema.
	if !strings.Contains(cuerpo, "Si no lo pediste") {
		t.Errorf("falta qué hacer si no lo pidió:\n%s", cuerpo)
	}
	// El enlace al sistema NO va en este mail: es el único que contiene una
	// credencial, y un link en un mail con un código es exactamente la forma de
	// un phishing.
	if strings.Contains(cuerpo, urlDePrueba) {
		t.Errorf("el mail del código no lleva enlaces:\n%s", cuerpo)
	}
}

func TestCorreo_CodigoDeRecuperacion_SinNombreNoQuedaElSaludoColgado(t *testing.T) {
	bus, enviador := mensajeroDePrueba()

	bus.Publish(eventbus.Evento{
		Tipo: "password.recuperacion.solicitada",
		Payload: eventbus.DatosDeRecuperacion{
			Email: "ana@escuela.edu.ar", Codigo: "482913", MinutosDeVigencia: 15,
		},
	})

	cuerpo := enviador.enviados[0].cuerpo
	if strings.Contains(cuerpo, "Hola :") {
		t.Errorf("saludo mal armado con el nombre vacío:\n%s", cuerpo)
	}
}

func TestCorreo_CuentaSoloConGoogle_ExplicaPorQueNoHayCodigo(t *testing.T) {
	bus, enviador := mensajeroDePrueba()

	bus.Publish(eventbus.Evento{
		Tipo:    "password.recuperacion.cuenta-google",
		Payload: eventbus.CuentaSoloConGoogle{Email: "ana@escuela.edu.ar", Nombre: "Ana"},
	})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 mail, hubo %d", len(enviador.enviados))
	}
	cuerpo := enviador.enviados[0].cuerpo
	if !strings.Contains(cuerpo, "Google") {
		t.Errorf("el mail existe justamente para explicar lo de Google:\n%s", cuerpo)
	}
}

// ── Robustez ────────────────────────────────────────────────────────────

func TestCorreo_PayloadInesperado_NoMandaNadaNiPanikea(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin@escuela.edu.ar")

	for _, tipo := range []string{
		"docente.registro.pendiente",
		"cuenta.aprobada",
		"password.recuperacion.solicitada",
		"password.recuperacion.cuenta-google",
	} {
		bus.Publish(eventbus.Evento{Tipo: tipo, Payload: "esto no es el payload esperado"})
	}

	if len(enviador.enviados) != 0 {
		t.Fatalf("no se tenía que mandar nada, se mandaron %d", len(enviador.enviados))
	}
}

func TestCorreo_SinURLConfiguradaElMailSaleIgual(t *testing.T) {
	bus := eventbus.NewInMemoryEventBus()
	enviador := &fakeEnviador{}
	RegisterEmailHandlersSincronos(bus, NewMensajero(enviador, &fakeListadorAdmins{}, &fakePreferencias{siempreSi: true}, ""))

	bus.Publish(eventbus.Evento{
		Tipo:    "cuenta.aprobada",
		Payload: eventbus.CuentaAprobada{Email: "ana@escuela.edu.ar", Nombre: "Ana"},
	})

	if len(enviador.enviados) != 1 {
		t.Fatalf("el mail tiene que salir igual, hubo %d", len(enviador.enviados))
	}
	if strings.Contains(enviador.enviados[0].cuerpo, "http") {
		t.Errorf("no había URL configurada, no puede aparecer una:\n%s", enviador.enviados[0].cuerpo)
	}
}

func TestCorreo_LaBarraFinalDeLaURLNoSeDuplica(t *testing.T) {
	bus := eventbus.NewInMemoryEventBus()
	enviador := &fakeEnviador{}
	RegisterEmailHandlersSincronos(bus, NewMensajero(enviador, &fakeListadorAdmins{}, &fakePreferencias{siempreSi: true}, urlDePrueba+"/"))

	bus.Publish(eventbus.Evento{
		Tipo:    "cuenta.aprobada",
		Payload: eventbus.CuentaAprobada{Email: "ana@escuela.edu.ar", Nombre: "Ana"},
	})

	if strings.Contains(enviador.enviados[0].cuerpo, urlDePrueba+"/") {
		t.Errorf("la barra final quedó pegada:\n%s", enviador.enviados[0].cuerpo)
	}
}

func TestCorreo_ElAvisoInternoYElMailSonIndependientes(t *testing.T) {
	// Los dos suscriptores escuchan docente.registro.pendiente.
	bus := eventbus.NewInMemoryEventBus()
	repo := nuevoFakeRepo()
	listador := &fakeListadorAdmins{adminIDs: []string{"admin-1"}, adminEmails: []string{"admin@escuela.edu.ar"}}
	svc := nuevoServicioDeTest(repo, listador)
	RegisterEventHandlersSincronos(bus, svc)
	RegisterEmailHandlersSincronos(bus, NewMensajero(
		&fakeEnviador{err: errors.New("gmail no responde")}, listador, &fakePreferencias{siempreSi: true}, urlDePrueba))

	bus.Publish(eventbus.Evento{
		Tipo:    "docente.registro.pendiente",
		Payload: map[string]string{"usuarioId": "usr-1", "nombre": "Ana", "apellido": "Pérez", "email": "ana@escuela.edu.ar"},
	})

	if len(repo.notificaciones) != 1 {
		t.Fatalf("el aviso interno tenía que escribirse igual, hay %d", len(repo.notificaciones))
	}
}

// ── La suscripción manda: sin casilla tildada no sale correo (RF-05.13) ──

// eventosQueVanATodosLosAdmins es uno de cada categoría, con el payload
// mínimo que hace que el correo salga.
func eventosQueVanATodosLosAdmins() []struct {
	nombre    string
	categoria domain.CategoriaEmail
	evento    eventbus.Evento
} {
	return []struct {
		nombre    string
		categoria domain.CategoriaEmail
		evento    eventbus.Evento
	}{
		{
			"cuenta pendiente", domain.CatCuentaPendiente,
			eventbus.Evento{Tipo: "docente.registro.pendiente", Payload: map[string]string{
				"usuarioId": "usr-1", "nombre": "Ana", "apellido": "Pérez", "email": "ana@escuela.edu.ar",
			}},
		},
		{
			"licencias por vencer", domain.CatLicenciaPorVencer,
			eventbus.Evento{Tipo: "licencia.por-vencer", Payload: eventbus.AvisoDeLicencias{
				PorVencer: []eventbus.LicenciaPorVencer{{Nombre: "Windows", Etiqueta: "PC 3", DiasRestantes: 1}},
			}},
		},
		{
			"buzón", domain.CatSugerencia,
			eventbus.Evento{Tipo: "sugerencia.nueva", Payload: eventbus.SugerenciaNueva{
				Quien: "Ana", Tipo: "PROBLEMA", Texto: "la PC 3 no prende",
			}},
		},
		{
			"pedido de materia", domain.CatPedidoDeMateria,
			eventbus.Evento{Tipo: "materia.pedido.nuevo", Payload: eventbus.PedidoDeMateriaNuevo{
				UsuarioID: "usr-1", Nombre: "Ana", MateriaNombre: "Física",
			}},
		},
		{
			"devolución demorada", domain.CatDevolucionDemorada,
			eventbus.Evento{Tipo: "prestamo.demorado", Payload: eventbus.PrestamosDemorados{
				Prestamos: []eventbus.PrestamoDemorado{{Etiqueta: "PC 7", Quien: "Ana", MinutosDeDemora: 40}},
			}},
		},
		{
			"cierre de jornada", domain.CatCierreSinDevolver,
			eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.EquiposSinDevolverAlCierre{
				Equipos: []eventbus.EquipoSinDevolverAlCierre{{Etiqueta: "PC 7", Quien: "Ana"}},
			}},
		},
	}
}

// El estado en que arranca la escuela: hay Admins aprobados y ninguno tildó
// nada. No sale un solo correo, y el aviso interno de cada uno de estos
// eventos sigue llegando igual (ver subscribers_test.go).
func TestCorreo_SinNadieSuscripto_NoSaleNingunCorreoALosAdmins(t *testing.T) {
	for _, caso := range eventosQueVanATodosLosAdmins() {
		t.Run(caso.nombre, func(t *testing.T) {
			bus := eventbus.NewInMemoryEventBus()
			enviador := &fakeEnviador{}
			m := NewMensajero(enviador, &fakeListadorAdmins{
				adminEmails: []string{"admin1@escuela.edu.ar", "admin2@escuela.edu.ar"},
				suscriptos:  map[domain.CategoriaEmail][]string{}, // nadie tildó nada
			}, &fakePreferencias{}, urlDePrueba)
			RegisterEmailHandlersSincronos(bus, m)

			bus.Publish(caso.evento)

			if len(enviador.enviados) != 0 {
				t.Fatalf("nadie se suscribió y salieron %d correos: %+v", len(enviador.enviados), enviador.enviados)
			}
		})
	}
}

// Cada categoría manda sobre la suya y sobre ninguna otra: tildar el buzón no
// puede traer de arriba los avisos de licencias.
func TestCorreo_CadaCategoriaLlegaSoloAQuienLaTildo(t *testing.T) {
	for _, caso := range eventosQueVanATodosLosAdmins() {
		t.Run(caso.nombre, func(t *testing.T) {
			bus := eventbus.NewInMemoryEventBus()
			enviador := &fakeEnviador{}
			m := NewMensajero(enviador, &fakeListadorAdmins{
				adminEmails: []string{"suscripto@escuela.edu.ar", "elotro@escuela.edu.ar"},
				suscriptos: map[domain.CategoriaEmail][]string{
					caso.categoria: {"suscripto@escuela.edu.ar"},
				},
			}, &fakePreferencias{}, urlDePrueba)
			RegisterEmailHandlersSincronos(bus, m)

			bus.Publish(caso.evento)

			if len(enviador.enviados) != 1 {
				t.Fatalf("esperaba 1 correo, hubo %d: %+v", len(enviador.enviados), enviador.enviados)
			}
			if enviador.enviados[0].para != "suscripto@escuela.edu.ar" {
				t.Errorf("le llegó a quien no lo pidió: %s", enviador.enviados[0].para)
			}
		})
	}
}

// El correo personal no pasa por el panel: el docente al que se le demoró una
// devolución recibe el suyo aunque ningún Admin haya tildado la categoría.
func TestCorreo_DemoraDelDocente_NoDependeDeLaSuscripcionDeLosAdmins(t *testing.T) {
	bus := eventbus.NewInMemoryEventBus()
	enviador := &fakeEnviador{}
	m := NewMensajero(enviador, &fakeListadorAdmins{
		adminEmails: []string{"admin1@escuela.edu.ar"},
		suscriptos:  map[domain.CategoriaEmail][]string{},
	}, &fakePreferencias{porEmail: map[string]map[domain.CategoriaEmail]bool{
		"ana@escuela.edu.ar": {domain.CatDevolucionPendiente: true},
	}}, urlDePrueba)
	RegisterEmailHandlersSincronos(bus, m)

	bus.Publish(eventbus.Evento{Tipo: "prestamo.demorado", Payload: eventbus.PrestamosDemorados{
		Prestamos: []eventbus.PrestamoDemorado{{
			Etiqueta: "PC 7", Quien: "Ana", Email: "ana@escuela.edu.ar", MinutosDeDemora: 40,
		}},
	}})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba el correo de Ana y nada más, hubo %d: %+v", len(enviador.enviados), enviador.enviados)
	}
	if enviador.enviados[0].para != "ana@escuela.edu.ar" {
		t.Errorf("esperaba el recordatorio a Ana, salió para %s", enviador.enviados[0].para)
	}
}

// ── Los correos personales también se eligen (RF-05.13) ─────────────────

// mensajeroConPreferencias arma el Mensajero con lo que cada dirección eligió;
// lo que no está ahí se rige por el valor por defecto de la categoría.
func mensajeroConPreferencias(elegido map[string]map[domain.CategoriaEmail]bool) (*eventbus.InMemoryEventBus, *fakeEnviador) {
	bus := eventbus.NewInMemoryEventBus()
	enviador := &fakeEnviador{}
	m := NewMensajero(enviador, &fakeListadorAdmins{}, &fakePreferencias{porEmail: elegido}, urlDePrueba)
	RegisterEmailHandlersSincronos(bus, m)
	return bus, enviador
}

// Lo del reloj arranca apagado: quien no dijo nada no recibe el recordatorio
// de su propia clase, que es algo que ya sabe.
func TestCorreo_Recordatorio_ApagadoPorDefecto(t *testing.T) {
	bus, enviador := mensajeroConPreferencias(nil)

	bus.Publish(eventbus.Evento{Tipo: "reserva.recordatorio", Payload: eventbus.RecordatorioDeReserva{
		UsuarioID: "usr-1", Email: "ana@escuela.edu.ar", Nombre: "Ana",
		MateriaNombre: "Física", Equipos: []string{"PC 3"},
	}})

	if len(enviador.enviados) != 0 {
		t.Fatalf("nadie lo pidió y salió igual: %+v", enviador.enviados)
	}
}

func TestCorreo_Recordatorio_LlegaSiLoPidio(t *testing.T) {
	bus, enviador := mensajeroConPreferencias(map[string]map[domain.CategoriaEmail]bool{
		"ana@escuela.edu.ar": {domain.CatRecordatorioDeReserva: true},
	})

	bus.Publish(eventbus.Evento{Tipo: "reserva.recordatorio", Payload: eventbus.RecordatorioDeReserva{
		UsuarioID: "usr-1", Email: "ana@escuela.edu.ar", Nombre: "Ana",
		MateriaNombre: "Física", Equipos: []string{"PC 3"},
	}})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 correo, hubo %d", len(enviador.enviados))
	}
}

// El aviso de que una computadora suya puede no estar arranca ENCENDIDO: es
// lo único que le da tiempo a conseguir otra antes de la clase.
func TestCorreo_EquipoNoDisponible_EncendidoPorDefecto(t *testing.T) {
	bus, enviador := mensajeroConPreferencias(nil)

	bus.Publish(eventbus.Evento{Tipo: "reserva.equipo-no-disponible", Payload: eventbus.EquipoNoDisponibleParaReserva{
		UsuarioID: "usr-1", Email: "ana@escuela.edu.ar", Nombre: "Ana", Equipos: []string{"PC 3"},
	}})

	if len(enviador.enviados) != 1 {
		t.Fatalf("tendría que haber llegado sin que nadie lo pida, hubo %d", len(enviador.enviados))
	}
}

// Y se puede apagar: es una preferencia, no una regla.
func TestCorreo_EquipoNoDisponible_SePuedeApagar(t *testing.T) {
	bus, enviador := mensajeroConPreferencias(map[string]map[domain.CategoriaEmail]bool{
		"ana@escuela.edu.ar": {domain.CatEquipoNoDisponible: false},
	})

	bus.Publish(eventbus.Evento{Tipo: "reserva.equipo-no-disponible", Payload: eventbus.EquipoNoDisponibleParaReserva{
		UsuarioID: "usr-1", Email: "ana@escuela.edu.ar", Nombre: "Ana", Equipos: []string{"PC 3"},
	}})

	if len(enviador.enviados) != 0 {
		t.Fatalf("lo apagó y llegó igual: %+v", enviador.enviados)
	}
}

// Los tres de la cuenta salen SIEMPRE, aunque esa persona haya apagado todo
// lo que se puede apagar. El del código es el caso que importa: quien lo
// necesita no puede entrar a leer la campana.
func TestCorreo_LosDeLaCuenta_NoLosApagaNadie(t *testing.T) {
	todoApagado := map[domain.CategoriaEmail]bool{}
	for _, c := range domain.CategoriasDeEmail() {
		todoApagado[c] = false
	}
	elegido := map[string]map[domain.CategoriaEmail]bool{"ana@escuela.edu.ar": todoApagado}

	casos := []struct {
		nombre string
		evento eventbus.Evento
	}{
		{"código de recuperación", eventbus.Evento{
			Tipo: "password.recuperacion.solicitada",
			Payload: eventbus.DatosDeRecuperacion{
				Email: "ana@escuela.edu.ar", Nombre: "Ana", Codigo: "123456", MinutosDeVigencia: 15,
			},
		}},
		{"cuenta aprobada", eventbus.Evento{
			Tipo:    "cuenta.aprobada",
			Payload: eventbus.CuentaAprobada{Email: "ana@escuela.edu.ar", Nombre: "Ana"},
		}},
		{"cuenta con Google", eventbus.Evento{
			Tipo:    "password.recuperacion.cuenta-google",
			Payload: eventbus.CuentaSoloConGoogle{Email: "ana@escuela.edu.ar", Nombre: "Ana"},
		}},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			bus, enviador := mensajeroConPreferencias(elegido)

			bus.Publish(caso.evento)

			if len(enviador.enviados) != 1 {
				t.Fatalf("tenía que salir igual, hubo %d", len(enviador.enviados))
			}
		})
	}
}

// ── La cancelación (RF-05.1/05.2/05.3) ──────────────────────────────────

// Arranca encendida: es la noticia de algo que decidió otro, y cuanto antes
// llegue más chances hay de conseguir otra máquina para esa clase.
func TestCorreo_Cancelacion_EncendidaPorDefecto(t *testing.T) {
	bus, enviador := mensajeroConPreferencias(nil)

	bus.Publish(eventbus.Evento{Tipo: "reserva.cancelada", Payload: eventbus.CancelacionesDeUsuario{
		UsuarioID: "usr-1", Nombre: "Ana", Email: "ana@escuela.edu.ar",
		Motivo: "la computadora pasó a mantenimiento",
		Reservas: []eventbus.ReservaCancelada{
			{ReservaID: "r-1", Etiqueta: "PC 3", Fecha: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)},
		},
	}})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 correo, hubo %d", len(enviador.enviados))
	}
	asunto, cuerpo := enviador.enviados[0].asunto, enviador.enviados[0].cuerpo
	if !strings.Contains(asunto, "PC 3") || !strings.Contains(asunto, "05/08") {
		t.Errorf("el asunto no dice qué se canceló ni cuándo: %q", asunto)
	}
	if !strings.Contains(cuerpo, "la computadora pasó a mantenimiento") {
		t.Errorf("falta el motivo, que es lo único que escribió una persona:\n%s", cuerpo)
	}
	// Lo que no se puede omitir: se cancelaron máquinas, no la clase.
	if !strings.Contains(cuerpo, "siguen reservadas") {
		t.Errorf("no aclara que el resto de la reserva sigue en pie:\n%s", cuerpo)
	}
}

// Un bloqueo sobre varias máquinas del mismo día es UN correo que las nombra,
// no uno por máquina.
func TestCorreo_Cancelacion_VariasDelMismoDia_UnSoloCorreo(t *testing.T) {
	bus, enviador := mensajeroConPreferencias(nil)
	elDia := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)

	bus.Publish(eventbus.Evento{Tipo: "reserva.cancelada", Payload: eventbus.CancelacionesDeUsuario{
		UsuarioID: "usr-1", Nombre: "Ana", Email: "ana@escuela.edu.ar",
		Motivo: "acto escolar",
		Reservas: []eventbus.ReservaCancelada{
			{ReservaID: "r-1", Etiqueta: "PC 3", Fecha: elDia},
			{ReservaID: "r-2", Etiqueta: "PC 12", Fecha: elDia},
		},
	}})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 correo con las dos, hubo %d", len(enviador.enviados))
	}
	cuerpo := enviador.enviados[0].cuerpo
	// En orden natural: "PC 3" antes que "PC 12", no al revés.
	if !strings.Contains(cuerpo, "PC 3, PC 12") {
		t.Errorf("no enumera los equipos en orden natural:\n%s", cuerpo)
	}
}

func TestCorreo_Cancelacion_SePuedeApagar(t *testing.T) {
	bus, enviador := mensajeroConPreferencias(map[string]map[domain.CategoriaEmail]bool{
		"ana@escuela.edu.ar": {domain.CatReservaCancelada: false},
	})

	bus.Publish(eventbus.Evento{Tipo: "reserva.cancelada", Payload: eventbus.CancelacionesDeUsuario{
		UsuarioID: "usr-1", Nombre: "Ana", Email: "ana@escuela.edu.ar", Motivo: "acto escolar",
		Reservas: []eventbus.ReservaCancelada{{ReservaID: "r-1", Etiqueta: "PC 3"}},
	}})

	if len(enviador.enviados) != 0 {
		t.Fatalf("lo apagó y llegó igual: %+v", enviador.enviados)
	}
}

// Sin dirección no hay correo, pero el aviso interno de la campana sale igual
// (eso lo prueba subscribers_test.go): una cuenta borrada no puede frenar la
// cancelación de nadie.
func TestCorreo_Cancelacion_SinEmail_NoRompe(t *testing.T) {
	bus, enviador := mensajeroConPreferencias(nil)

	bus.Publish(eventbus.Evento{Tipo: "reserva.cancelada", Payload: eventbus.CancelacionesDeUsuario{
		UsuarioID: "usr-1", Motivo: "acto escolar",
		Reservas: []eventbus.ReservaCancelada{{ReservaID: "r-1", Etiqueta: "PC 3"}},
	}})

	if len(enviador.enviados) != 0 {
		t.Fatalf("no había a quién escribirle: %+v", enviador.enviados)
	}
}
