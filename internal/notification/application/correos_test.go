package application

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	m := NewMensajero(enviador, &fakeListadorAdmins{adminEmails: admins}, urlDePrueba)
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
	}, urlDePrueba)
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
	RegisterEmailHandlersSincronos(bus, NewMensajero(enviador, &fakeListadorAdmins{}, ""))

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
	RegisterEmailHandlersSincronos(bus, NewMensajero(enviador, &fakeListadorAdmins{}, urlDePrueba+"/"))

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
		&fakeEnviador{err: errors.New("gmail no responde")}, listador, urlDePrueba))

	bus.Publish(eventbus.Evento{
		Tipo:    "docente.registro.pendiente",
		Payload: map[string]string{"usuarioId": "usr-1", "nombre": "Ana", "apellido": "Pérez", "email": "ana@escuela.edu.ar"},
	})

	if len(repo.notificaciones) != 1 {
		t.Fatalf("el aviso interno tenía que escribirse igual, hay %d", len(repo.notificaciones))
	}
}
