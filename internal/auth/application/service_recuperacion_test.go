package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// El código que devuelve codigoFalso, para no repetirlo en cada test.
const codigoDePrueba = "123456"

var ahoraDeLosTests = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// servicioDeRecuperacion arma un Service con el bus real y devuelve además
// los eventos que se van publicando, que es donde viaja el código.
func servicioDeRecuperacion(t *testing.T, repo Repo, correoHabilitado bool) (*Service, *[]eventbus.Evento) {
	t.Helper()
	contadorID = 0
	bus := eventbus.NewInMemoryEventBus()

	var publicados []eventbus.Evento
	for _, tipo := range []string{"password.recuperacion.solicitada", "password.recuperacion.cuenta-google"} {
		bus.Subscribe(tipo, func(e eventbus.Evento) { publicados = append(publicados, e) })
	}

	svc := NewService(
		repo, bus, hashFalso, verifyFalso, firmarFalso, idSecuencial,
		temporalFalso, codigoFalso, relojFijo(ahoraDeLosTests),
		nuevoFakeGestorMaterias(), nuevoFakeCanceladorReservas(), nil,
		correoHabilitado,
	)
	return svc, &publicados
}

// docenteAprobado deja una cuenta lista para recuperar: aprobada y con
// contraseña propia.
func docenteAprobado(repo *fakeRepo, email string) *domain.Usuario {
	u := &domain.Usuario{
		ID:           "usr-1",
		Nombre:       "Ana",
		Apellido:     "Pérez",
		Email:        email,
		PasswordHash: "hash:la-vieja",
		Rol:          domain.RolDocente,
		Estado:       domain.EstadoAprobada,
	}
	repo.usuarios[u.ID] = u
	return u
}

// ══════════════════════════════════════════════════════════════════
// Solicitar el código
// ══════════════════════════════════════════════════════════════════

func TestSolicitarRecuperacion_GuardaElCodigoHasheadoYLoPublica(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc, eventos := servicioDeRecuperacion(t, repo, true)

	if err := svc.SolicitarRecuperacionDePassword(context.Background(), "ana@escuela.edu.ar"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	guardado := repo.codigos[u.ID]
	if guardado == nil {
		t.Fatal("no se guardó ningún código")
	}
	// Lo que se persiste es el hash, nunca el código: si la base se filtra,
	// un código en claro es una cuenta abierta hasta que expire.
	if guardado.CodigoHash == codigoDePrueba {
		t.Error("el código se guardó en claro")
	}
	if guardado.CodigoHash != "hash:"+codigoDePrueba {
		t.Errorf("esperaba el hash del código, obtuve %q", guardado.CodigoHash)
	}

	if len(*eventos) != 1 {
		t.Fatalf("esperaba 1 evento, hubo %d", len(*eventos))
	}
	datos, ok := (*eventos)[0].Payload.(eventbus.DatosDeRecuperacion)
	if !ok {
		t.Fatalf("payload inesperado: %T", (*eventos)[0].Payload)
	}
	if datos.Codigo != codigoDePrueba {
		t.Errorf("el mail tiene que llevar el código en claro, lleva %q", datos.Codigo)
	}
	if datos.Email != "ana@escuela.edu.ar" || datos.Nombre != "Ana" {
		t.Errorf("destinatario mal armado: %+v", datos)
	}
	if datos.MinutosDeVigencia != 15 {
		t.Errorf("esperaba 15 minutos de vigencia, obtuve %d", datos.MinutosDeVigencia)
	}
}

func TestSolicitarRecuperacion_EmailInexistenteNoDiceNada(t *testing.T) {
	repo := nuevoFakeRepo()
	docenteAprobado(repo, "ana@escuela.edu.ar")
	svc, eventos := servicioDeRecuperacion(t, repo, true)

	// Sin error y sin evento: la respuesta tiene que ser indistinguible de
	// la de un email que sí existe, o el formulario delata qué cuentas hay.
	if err := svc.SolicitarRecuperacionDePassword(context.Background(), "nadie@escuela.edu.ar"); err != nil {
		t.Fatalf("no puede devolver error: delataría que el email no existe (%v)", err)
	}
	if len(*eventos) != 0 {
		t.Errorf("no se tenía que mandar ningún mail, se publicaron %d eventos", len(*eventos))
	}
}

func TestSolicitarRecuperacion_CuentaNoAprobadaNoRecibeCodigo(t *testing.T) {
	for _, estado := range []domain.Estado{domain.EstadoPendiente, domain.EstadoRechazada, domain.EstadoBaja} {
		repo := nuevoFakeRepo()
		u := docenteAprobado(repo, "ana@escuela.edu.ar")
		u.Estado = estado
		svc, eventos := servicioDeRecuperacion(t, repo, true)

		if err := svc.SolicitarRecuperacionDePassword(context.Background(), "ana@escuela.edu.ar"); err != nil {
			t.Fatalf("%s: no debería devolver error: %v", estado, err)
		}
		if len(*eventos) != 0 {
			t.Errorf("%s: una cuenta que no está aprobada no tiene a dónde entrar, no corresponde mandarle un código", estado)
		}
		if len(repo.codigos) != 0 {
			t.Errorf("%s: no se tenía que guardar ningún código", estado)
		}
	}
}

func TestSolicitarRecuperacion_CuentaDeGoogleRecibeElOtroMail(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	u.PasswordHash = "" // creada con Google: no tiene contraseña propia
	u.GoogleSub = "sub-123"
	svc, eventos := servicioDeRecuperacion(t, repo, true)

	if err := svc.SolicitarRecuperacionDePassword(context.Background(), "ana@escuela.edu.ar"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if len(repo.codigos) != 0 {
		t.Error("una cuenta sin contraseña no tiene nada que recuperar: no se le genera código")
	}
	if len(*eventos) != 1 {
		t.Fatalf("esperaba el aviso de cuenta con Google, hubo %d eventos", len(*eventos))
	}
	if (*eventos)[0].Tipo != "password.recuperacion.cuenta-google" {
		t.Errorf("evento inesperado: %s", (*eventos)[0].Tipo)
	}
}

func TestSolicitarRecuperacion_NormalizaElEmail(t *testing.T) {
	repo := nuevoFakeRepo()
	docenteAprobado(repo, "ana@escuela.edu.ar")
	svc, eventos := servicioDeRecuperacion(t, repo, true)

	if err := svc.SolicitarRecuperacionDePassword(context.Background(), "  ANA@Escuela.Edu.Ar  "); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(*eventos) != 1 {
		t.Fatal("el email tiene que encontrarse igual con otra capitalización y espacios de más")
	}
}

func TestSolicitarRecuperacion_EmailMalEscritoEsError(t *testing.T) {
	svc, _ := servicioDeRecuperacion(t, nuevoFakeRepo(), true)

	// Acá sí conviene el error: no filtra nada (cualquiera ve que no es una
	// dirección) y sin él el typo más común termina en "revisá tu casilla".
	err := svc.SolicitarRecuperacionDePassword(context.Background(), "ana-arroba-escuela")
	if !errors.Is(err, domain.ErrEmailInvalido) {
		t.Fatalf("esperaba ErrEmailInvalido, obtuve %v", err)
	}
}

func TestSolicitarRecuperacion_SinCorreoConfiguradoAvisa(t *testing.T) {
	repo := nuevoFakeRepo()
	docenteAprobado(repo, "ana@escuela.edu.ar")
	svc, _ := servicioDeRecuperacion(t, repo, false)

	err := svc.SolicitarRecuperacionDePassword(context.Background(), "ana@escuela.edu.ar")
	if !errors.Is(err, ErrRecuperacionNoDisponible) {
		t.Fatalf("esperaba ErrRecuperacionNoDisponible, obtuve %v", err)
	}
	if svc.RecuperacionPorEmailDisponible() {
		t.Error("RecuperacionPorEmailDisponible tendría que ser false")
	}
}

// ══════════════════════════════════════════════════════════════════
// Restablecer con el código
// ══════════════════════════════════════════════════════════════════

// pedirCodigo deja una solicitud hecha y devuelve el servicio listo para
// el segundo paso.
func pedirCodigo(t *testing.T, repo *fakeRepo, email string) *Service {
	t.Helper()
	svc, _ := servicioDeRecuperacion(t, repo, true)
	if err := svc.SolicitarRecuperacionDePassword(context.Background(), email); err != nil {
		t.Fatalf("preparando el código: %v", err)
	}
	return svc
}

func TestRestablecer_CambiaLaPasswordYConsumeElCodigo(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")

	id, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", codigoDePrueba, "una-contraseña-nueva")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if id != u.ID {
		t.Errorf("esperaba el id %q para auditar, obtuve %q", u.ID, id)
	}

	if u.PasswordHash != "hash:una-contraseña-nueva" {
		t.Errorf("la contraseña no se cambió: %q", u.PasswordHash)
	}
	// La eligió la persona: no es una temporal que haya que reemplazar.
	if u.DebeCambiarPassword {
		t.Error("no tiene que quedar obligada a cambiarla de nuevo")
	}
	if repo.codigos[u.ID].UsadoEn == nil {
		t.Error("el código tiene que quedar consumido")
	}
}

func TestRestablecer_ElMismoCodigoNoSirveDosVeces(t *testing.T) {
	repo := nuevoFakeRepo()
	docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")

	if _, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", codigoDePrueba, "una-contraseña-nueva"); err != nil {
		t.Fatalf("el primer uso tiene que funcionar: %v", err)
	}

	_, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", codigoDePrueba, "otra-contraseña-mas")
	if !errors.Is(err, ErrCodigoRecuperacionInvalido) {
		t.Fatalf("esperaba ErrCodigoRecuperacionInvalido en el segundo uso, obtuve %v", err)
	}
}

func TestRestablecer_CodigoEquivocadoSumaIntentoYNoCambiaNada(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")

	_, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", "999999", "una-contraseña-nueva")
	if !errors.Is(err, ErrCodigoRecuperacionInvalido) {
		t.Fatalf("esperaba ErrCodigoRecuperacionInvalido, obtuve %v", err)
	}
	if u.PasswordHash != "hash:la-vieja" {
		t.Error("la contraseña no se tenía que tocar")
	}
	if repo.codigos[u.ID].Intentos != 1 {
		t.Errorf("esperaba 1 intento registrado, hay %d", repo.codigos[u.ID].Intentos)
	}
}

func TestRestablecer_SeQuemaAlAgotarLosIntentos(t *testing.T) {
	repo := nuevoFakeRepo()
	docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")

	var ultimo error
	for i := 0; i < domain.MaxIntentosCodigoRecuperacion; i++ {
		_, ultimo = svc.RestablecerPasswordConCodigo(context.Background(),
			"ana@escuela.edu.ar", "999999", "una-contraseña-nueva")
	}
	if !errors.Is(ultimo, ErrCodigoRecuperacionSinIntentos) {
		t.Fatalf("el último intento tendría que quemar el código, obtuve %v", ultimo)
	}

	// Y a partir de ahí ni siquiera el código correcto sirve: hay que pedir
	// uno nuevo. Es lo que hace que seis dígitos alcancen.
	_, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", codigoDePrueba, "una-contraseña-nueva")
	if err == nil {
		t.Fatal("un código quemado no puede aceptar el código correcto después")
	}
}

func TestRestablecer_SiNoSePuedeRegistrarElIntentoFallidoDevuelveErrorDeVerdad(t *testing.T) {
	repo := nuevoFakeRepo()
	docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")
	repo.errGuardarCodigo = errors.New("postgres no responde")

	_, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", "999999", "una-contraseña-nueva")

	// NO puede devolver "código inválido": ese es el error normal, y quien
	// esté probando códigos seguiría probando sin que nada cuente sus
	// intentos. Si el contador no se puede persistir, el tope no existe.
	if errors.Is(err, ErrCodigoRecuperacionInvalido) {
		t.Fatal("con el contador de intentos roto no puede responder como un fallo normal: la fuerza bruta quedaría sin tope")
	}
	if err == nil {
		t.Fatal("esperaba un error")
	}
}

func TestRestablecer_CodigoVencidoSeDistingue(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")

	// Se envejece el código a mano: el reloj del servicio está fijo.
	repo.codigos[u.ID].ExpiraEn = ahoraDeLosTests.Add(-time.Minute)

	_, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", codigoDePrueba, "una-contraseña-nueva")
	// Vencido SÍ se distingue de inválido: le pasa a la persona legítima,
	// que necesita saber que tiene que pedir otro y no que tipeó mal.
	if !errors.Is(err, ErrCodigoRecuperacionVencido) {
		t.Fatalf("esperaba ErrCodigoRecuperacionVencido, obtuve %v", err)
	}
}

func TestRestablecer_SinHaberPedidoCodigo(t *testing.T) {
	repo := nuevoFakeRepo()
	docenteAprobado(repo, "ana@escuela.edu.ar")
	svc, _ := servicioDeRecuperacion(t, repo, true)

	_, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", codigoDePrueba, "una-contraseña-nueva")
	if !errors.Is(err, ErrCodigoRecuperacionInvalido) {
		t.Fatalf("esperaba ErrCodigoRecuperacionInvalido, obtuve %v", err)
	}
}

func TestRestablecer_EmailInexistenteDaElMismoErrorQueUnCodigoMalo(t *testing.T) {
	repo := nuevoFakeRepo()
	docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")

	_, errInexistente := svc.RestablecerPasswordConCodigo(context.Background(),
		"nadie@escuela.edu.ar", codigoDePrueba, "una-contraseña-nueva")
	_, errCodigoMalo := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", "999999", "una-contraseña-nueva")

	// Los dos mensajes tienen que ser el MISMO string: si difirieran,
	// este endpoint diría qué direcciones están registradas.
	if errInexistente == nil || errCodigoMalo == nil {
		t.Fatal("los dos casos tienen que fallar")
	}
	if errInexistente.Error() != errCodigoMalo.Error() {
		t.Fatalf("los mensajes delatan si la cuenta existe:\n  inexistente: %v\n  código malo: %v",
			errInexistente, errCodigoMalo)
	}
}

func TestRestablecer_PasswordCortaSeRechazaAntesDeQuemarElCodigo(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")

	_, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", codigoDePrueba, "corta")
	if !errors.Is(err, ErrPasswordCorta) {
		t.Fatalf("esperaba ErrPasswordCorta, obtuve %v", err)
	}
	if repo.codigos[u.ID].Intentos != 0 {
		t.Error("una contraseña corta no puede consumir un intento del código")
	}

	// Y el código sigue sirviendo con una contraseña válida.
	if _, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", codigoDePrueba, "ahora-sí-es-larga"); err != nil {
		t.Fatalf("el código tendría que seguir sirviendo: %v", err)
	}
}

func TestRestablecer_CuentaDeGoogleNoSePuedeRestablecer(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")
	// La cuenta pasa a no tener contraseña propia después de haber pedido
	// el código: el segundo paso lo tiene que volver a chequear.
	u.PasswordHash = ""
	u.GoogleSub = "sub-123"

	_, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", codigoDePrueba, "una-contraseña-nueva")
	if !errors.Is(err, ErrCodigoRecuperacionInvalido) {
		t.Fatalf("esperaba ErrCodigoRecuperacionInvalido, obtuve %v", err)
	}
}

func TestRestablecer_SinCorreoConfiguradoAvisa(t *testing.T) {
	repo := nuevoFakeRepo()
	docenteAprobado(repo, "ana@escuela.edu.ar")
	svc, _ := servicioDeRecuperacion(t, repo, false)

	_, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", codigoDePrueba, "una-contraseña-nueva")
	if !errors.Is(err, ErrRecuperacionNoDisponible) {
		t.Fatalf("esperaba ErrRecuperacionNoDisponible, obtuve %v", err)
	}
}

func TestSolicitarRecuperacion_PedirDeNuevoInvalidaElAnterior(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")

	primero := repo.codigos[u.ID]
	if err := svc.SolicitarRecuperacionDePassword(context.Background(), "ana@escuela.edu.ar"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Tiene que quedar UNO solo vigente. Si se acumularan, el tope de cinco
	// intentos por código se multiplicaría por la cantidad de códigos que
	// alguien se tome el trabajo de pedir.
	if repo.codigos[u.ID].ID == primero.ID {
		t.Fatal("pedir un código nuevo tiene que reemplazar al anterior")
	}
}

// ══════════════════════════════════════════════════════════════════
// El evento de cuenta aprobada
// ══════════════════════════════════════════════════════════════════

func TestAprobar_PublicaCuentaAprobadaConNombreYEmail(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	u.Estado = domain.EstadoPendiente

	contadorID = 0
	bus := eventbus.NewInMemoryEventBus()
	var aprobadas []eventbus.CuentaAprobada
	bus.Subscribe("cuenta.aprobada", func(e eventbus.Evento) {
		if p, ok := e.Payload.(eventbus.CuentaAprobada); ok {
			aprobadas = append(aprobadas, p)
		}
	})
	svc := NewService(repo, bus, hashFalso, verifyFalso, firmarFalso, idSecuencial,
		temporalFalso, codigoFalso, relojFijo(ahoraDeLosTests),
		nuevoFakeGestorMaterias(), nuevoFakeCanceladorReservas(), nil, true)

	if err := svc.Aprobar(context.Background(), u.ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if len(aprobadas) != 1 {
		t.Fatalf("esperaba 1 evento cuenta.aprobada, hubo %d", len(aprobadas))
	}
	// El mail necesita las dos cosas: sin el email no hay a dónde mandarlo,
	// sin el nombre el saludo queda impersonal.
	if aprobadas[0].Email != "ana@escuela.edu.ar" || aprobadas[0].Nombre != "Ana" {
		t.Errorf("payload incompleto: %+v", aprobadas[0])
	}
}

func TestRechazar_NoPublicaCuentaAprobada(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	u.Estado = domain.EstadoPendiente

	contadorID = 0
	bus := eventbus.NewInMemoryEventBus()
	publicados := 0
	bus.Subscribe("cuenta.aprobada", func(eventbus.Evento) { publicados++ })
	svc := NewService(repo, bus, hashFalso, verifyFalso, firmarFalso, idSecuencial,
		temporalFalso, codigoFalso, relojFijo(ahoraDeLosTests),
		nuevoFakeGestorMaterias(), nuevoFakeCanceladorReservas(), nil, true)

	if err := svc.Rechazar(context.Background(), u.ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Este es el motivo por el que cuenta.aprobada existe como evento
	// aparte: cuenta.pendiente.resuelta se publica igual al rechazar, así
	// que colgarle el mail habría felicitado a quien acaban de rechazar.
	if publicados != 0 {
		t.Fatalf("rechazar no puede publicar cuenta.aprobada (se publicó %d veces)", publicados)
	}
}
