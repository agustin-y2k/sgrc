package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// ── fakeVerificadorGoogle ──────────────────────────────────────────────
// Reemplaza a la verificación real de firmas contra las claves públicas de
// Google (eso se prueba aparte, en infrastructure/google_idtoken_test.go, con
// tokens firmados de verdad).

type fakeVerificadorGoogle struct {
	identidad *IdentidadGoogle
	err       error
	// recibido guarda el token crudo, para verificar que se pasa tal cual.
	recibido string
}

func (f *fakeVerificadorGoogle) Verificar(ctx context.Context, idToken string) (*IdentidadGoogle, error) {
	f.recibido = idToken
	if f.err != nil {
		return nil, f.err
	}
	return f.identidad, nil
}

func identidadDePrueba() *IdentidadGoogle {
	return &IdentidadGoogle{
		Sub:             "112233445566",
		Email:           "ada@escuela.edu.ar",
		EmailVerificado: true,
		Nombre:          "Ada",
		Apellido:        "Lovelace",
	}
}

func nuevoServicioConGoogle(repo Repo, verificador VerificadorGoogle) *Service {
	contadorID = 0
	return NewService(
		repo,
		eventbus.NewInMemoryEventBus(),
		hashFalso,
		verifyFalso,
		firmarFalso,
		idSecuencial,
		temporalFalso,
		codigoFalso,
		relojFijo(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)),
		nuevoFakeGestorMaterias(),
		nuevoFakeCanceladorReservas(),
		verificador,
		true, // con correo: es lo que habilita la recuperación por autoservicio
	)
}

// usuarioDeGoogle arma una cuenta ya vinculada, como la que dejaría un
// registro con Google: sin PasswordHash.
func usuarioDeGoogle(id, email, sub string, estado domain.Estado) *domain.Usuario {
	return &domain.Usuario{
		ID:        id,
		Nombre:    "Ada",
		Apellido:  "Lovelace",
		Email:     email,
		GoogleSub: sub,
		Rol:       domain.RolDocente,
		Estado:    estado,
	}
}

// ── Disponibilidad y validez del token ────────────────────────────────

// Sin GOOGLE_CLIENT_ID configurado el verificador es nil.
func TestLoginConGoogle_SinVerificador_NoDisponible(t *testing.T) {
	svc := nuevoServicioConGoogle(nuevoFakeRepo(), nil)

	_, err := svc.LoginConGoogle(context.Background(), "un-token", false)

	if !errors.Is(err, ErrLoginGoogleNoDisponible) {
		t.Fatalf("esperaba ErrLoginGoogleNoDisponible, hubo: %v", err)
	}
}

func TestRegistrarConGoogle_SinVerificador_NoDisponible(t *testing.T) {
	svc := nuevoServicioConGoogle(nuevoFakeRepo(), nil)

	_, err := svc.RegistrarConGoogle(context.Background(), "un-token", "", "", solicitudDeDocente())

	if !errors.Is(err, ErrLoginGoogleNoDisponible) {
		t.Fatalf("esperaba ErrLoginGoogleNoDisponible, hubo: %v", err)
	}
}

func TestLoginConGoogle_TokenVacio_NiSiquieraLlamaAGoogle(t *testing.T) {
	verificador := &fakeVerificadorGoogle{identidad: identidadDePrueba()}
	svc := nuevoServicioConGoogle(nuevoFakeRepo(), verificador)

	_, err := svc.LoginConGoogle(context.Background(), "   ", false)

	if !errors.Is(err, ErrTokenGoogleInvalido) {
		t.Fatalf("esperaba ErrTokenGoogleInvalido, hubo: %v", err)
	}
	if verificador.recibido != "" {
		t.Error("un token vacío no debería llegar hasta el verificador")
	}
}

// Un token que no valida es 401, y el error de negocio tiene que llegar
// intacto hasta el handler para que lo mapee a ese código.
func TestLoginConGoogle_TokenInvalido(t *testing.T) {
	verificador := &fakeVerificadorGoogle{err: ErrTokenGoogleInvalido}
	svc := nuevoServicioConGoogle(nuevoFakeRepo(), verificador)

	_, err := svc.LoginConGoogle(context.Background(), "token-falsificado", false)

	if !errors.Is(err, ErrTokenGoogleInvalido) {
		t.Fatalf("esperaba ErrTokenGoogleInvalido, hubo: %v", err)
	}
}

// Una falla de red buscando las claves de Google NO es un token inválido:
// decirle 401 a quien mandó un token perfectamente bueno haría que el
// problema real (nuestro) sea indepurable desde afuera.
func TestLoginConGoogle_FallaDeInfraestructura_NoSeDisfrazaDeTokenInvalido(t *testing.T) {
	fallaDeRed := errors.New("connection refused")
	svc := nuevoServicioConGoogle(nuevoFakeRepo(), &fakeVerificadorGoogle{err: fallaDeRed})

	_, err := svc.LoginConGoogle(context.Background(), "un-token", false)

	if errors.Is(err, ErrTokenGoogleInvalido) {
		t.Fatal("una falla de red no debe reportarse como token inválido")
	}
	if !errors.Is(err, fallaDeRed) {
		t.Fatalf("esperaba que se conservara la causa original, hubo: %v", err)
	}
}

// Sin email_verified, cualquiera que pueda poner una dirección ajena en su
// perfil de Google entraría a la cuenta de otra persona.
func TestLoginConGoogle_EmailNoVerificado_Rechaza(t *testing.T) {
	identidad := identidadDePrueba()
	identidad.EmailVerificado = false

	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = usuarioDeGoogle("u1", "ada@escuela.edu.ar", "112233445566", domain.EstadoAprobada)
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidad})

	_, err := svc.LoginConGoogle(context.Background(), "un-token", false)

	if !errors.Is(err, ErrEmailNoVerificadoPorGoogle) {
		t.Fatalf("esperaba ErrEmailNoVerificadoPorGoogle, hubo: %v", err)
	}
}

func TestLoginConGoogle_DominioNoPermitido_LlegaIntacto(t *testing.T) {
	svc := nuevoServicioConGoogle(nuevoFakeRepo(), &fakeVerificadorGoogle{err: ErrDominioNoPermitido})

	_, err := svc.LoginConGoogle(context.Background(), "un-token", false)

	if !errors.Is(err, ErrDominioNoPermitido) {
		t.Fatalf("esperaba ErrDominioNoPermitido, hubo: %v", err)
	}
}

// ── Login ─────────────────────────────────────────────────────────────

func TestLoginConGoogle_CuentaVinculadaYAprobada_DevuelveToken(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = usuarioDeGoogle("u1", "ada@escuela.edu.ar", "112233445566", domain.EstadoAprobada)
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	res, err := svc.LoginConGoogle(context.Background(), "un-token", false)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if res.Token != "token-de-u1" {
		t.Errorf("token inesperado: %s", res.Token)
	}
}

// El sub es lo estable, el email no. Quien ya entró alguna vez sigue
// entrando a SU cuenta aunque Google le haya cambiado la dirección.
func TestLoginConGoogle_ElSubGanaSobreElEmail(t *testing.T) {
	repo := nuevoFakeRepo()
	// La cuenta se registró con la dirección vieja y quedó vinculada al sub.
	repo.usuarios["u1"] = usuarioDeGoogle("u1", "ada.vieja@escuela.edu.ar", "112233445566", domain.EstadoAprobada)
	// Otra persona ocupa hoy la dirección nueva.
	repo.usuarios["u2"] = usuarioDeGoogle("u2", "ada@escuela.edu.ar", "999999", domain.EstadoAprobada)

	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	res, err := svc.LoginConGoogle(context.Background(), "un-token", false)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if res.Token != "token-de-u1" {
		t.Errorf("entró a la cuenta equivocada: %s (el sub tiene que ganar sobre el email)", res.Token)
	}
}

func TestLoginConGoogle_SinCuenta_PideRegistro(t *testing.T) {
	svc := nuevoServicioConGoogle(nuevoFakeRepo(), &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	_, err := svc.LoginConGoogle(context.Background(), "un-token", false)

	if !errors.Is(err, ErrCuentaGoogleNoRegistrada) {
		t.Fatalf("esperaba ErrCuentaGoogleNoRegistrada, hubo: %v", err)
	}
}

// El caso del docente que ya tenía cuenta con contraseña y ahora entra con
// Google: se le agrega el vínculo y CONSERVA la contraseña — las dos formas
// de ingreso conviven.
func TestLoginConGoogle_VinculaCuentaExistentePorEmail_YConservaLaPassword(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID:           "u1",
		Nombre:       "Ada",
		Apellido:     "Lovelace",
		Email:        "ada@escuela.edu.ar",
		PasswordHash: "hash:password123",
		Rol:          domain.RolDocente,
		Estado:       domain.EstadoAprobada,
	}
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	if _, err := svc.LoginConGoogle(context.Background(), "un-token", false); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	u := repo.usuarios["u1"]
	if u.GoogleSub != "112233445566" {
		t.Errorf("la cuenta no quedó vinculada: google_sub = %q", u.GoogleSub)
	}
	if u.PasswordHash != "hash:password123" {
		t.Errorf("vincular Google no debe borrar la contraseña: %q", u.PasswordHash)
	}
}

// La vinculación por email es segura solo porque antes se exigió
// email_verified.
func TestLoginConGoogle_EmailNoVerificado_NoVinculaNada(t *testing.T) {
	identidad := identidadDePrueba()
	identidad.EmailVerificado = false

	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Nombre: "Ada", Apellido: "Lovelace",
		Email: "ada@escuela.edu.ar", PasswordHash: "hash:password123",
		Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidad})

	if _, err := svc.LoginConGoogle(context.Background(), "un-token", false); err == nil {
		t.Fatal("esperaba que fallara")
	}

	if repo.usuarios["u1"].GoogleSub != "" {
		t.Error("no se debe vincular una cuenta con un email que Google no confirmó")
	}
}

// Una cuenta PENDIENTE se vincula igual y recién después se rechaza el
// ingreso: así, el día que el Admin la apruebe, entra sin que la persona
// tenga que repetir nada.
func TestLoginConGoogle_CuentaPendiente_VinculaPeroNoDejaEntrar(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Nombre: "Ada", Apellido: "Lovelace",
		Email: "ada@escuela.edu.ar", PasswordHash: "hash:password123",
		Rol: domain.RolDocente, Estado: domain.EstadoPendiente,
	}
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	_, err := svc.LoginConGoogle(context.Background(), "un-token", false)

	if !errors.Is(err, ErrCuentaNoHabilitada) {
		t.Fatalf("esperaba ErrCuentaNoHabilitada, hubo: %v", err)
	}
	if repo.usuarios["u1"].GoogleSub != "112233445566" {
		t.Error("la cuenta pendiente debería quedar vinculada igual")
	}
}

// RF-02.9: la baja es terminal. No se reactiva entrando con Google.
func TestLoginConGoogle_CuentaEnBaja_NoSeVincula(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Nombre: "Ada", Apellido: "Lovelace",
		Email: "ada@escuela.edu.ar", PasswordHash: "hash:password123",
		Rol: domain.RolDocente, Estado: domain.EstadoBaja,
	}
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	_, err := svc.LoginConGoogle(context.Background(), "un-token", false)

	// El error del INGRESO, no el del registro: acá la persona está
	// intentando entrar, no crear una cuenta.
	if !errors.Is(err, ErrIngresoCuentaEnBaja) {
		t.Fatalf("esperaba ErrIngresoCuentaEnBaja, hubo: %v", err)
	}
	if errors.Is(err, ErrCuentaEnBaja) {
		t.Error("ese es el mensaje del registro: le dice que pida eliminar la cuenta para poder registrarse de nuevo, que no es lo que preguntó")
	}
	if repo.usuarios["u1"].GoogleSub != "" {
		t.Error("una cuenta en BAJA no debe quedar vinculada a nada")
	}
}

func TestLoginConGoogle_NormalizaElEmailAntesDeBuscar(t *testing.T) {
	identidad := identidadDePrueba()
	identidad.Email = "  Ada.Lovelace@Escuela.Edu.Ar  "

	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Nombre: "Ada", Apellido: "Lovelace",
		Email: "ada.lovelace@escuela.edu.ar", PasswordHash: "hash:password123",
		Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidad})

	if _, err := svc.LoginConGoogle(context.Background(), "un-token", false); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
}

// ── Registro ──────────────────────────────────────────────────────────

func TestRegistrarConGoogle_CreaCuentaPendienteSinPassword(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	u, err := svc.RegistrarConGoogle(context.Background(), "un-token", "", "",
		SolicitudDeAsignacion{Cargo: domain.CargoSolicitadoDocente, Rol: domain.RolSolicitadoTitular,
			Curso: "5°A", Materia: "Programación"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if u.Estado != domain.EstadoPendiente {
		t.Errorf("una cuenta creada con Google también tiene que quedar PENDIENTE: %s", u.Estado)
	}
	if u.Rol != domain.RolDocente {
		t.Errorf("rol inesperado: %s", u.Rol)
	}
	if u.PasswordHash != "" {
		t.Errorf("no debería tener contraseña: %q", u.PasswordHash)
	}
	if u.GoogleSub != "112233445566" {
		t.Errorf("google_sub inesperado: %q", u.GoogleSub)
	}
	if u.Email != "ada@escuela.edu.ar" {
		t.Errorf("email inesperado: %q", u.Email)
	}
	if u.CursoSolicitado != "5°A" || u.MateriaSolicitada != "Programación" {
		t.Errorf("no se guardó lo que declaró que va a dictar: %q / %q", u.CursoSolicitado, u.MateriaSolicitada)
	}
	if repo.usuarios[u.ID] == nil {
		t.Error("la cuenta no se persistió")
	}
}

// El Admin tiene que enterarse igual que con el registro con contraseña
// (RF-05.6): para él es la misma cuenta pendiente.
func TestRegistrarConGoogle_PublicaEventoParaAdmins(t *testing.T) {
	svc := nuevoServicioConGoogle(nuevoFakeRepo(), &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	recibido := make(chan eventbus.Evento, 1)
	svc.bus.Subscribe("docente.registro.pendiente", func(e eventbus.Evento) { recibido <- e })

	if _, err := svc.RegistrarConGoogle(context.Background(), "un-token", "", "", solicitudDeDocente()); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	select {
	case e := <-recibido:
		if e.Payload.(map[string]string)["email"] != "ada@escuela.edu.ar" {
			t.Errorf("payload incorrecto: %v", e.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("nunca se publicó el evento de registro pendiente")
	}
}

func TestRegistrarConGoogle_NombreDelRequestGanaSobreElDelToken(t *testing.T) {
	svc := nuevoServicioConGoogle(nuevoFakeRepo(), &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	u, err := svc.RegistrarConGoogle(context.Background(), "un-token", "Augusta", "Byron", solicitudDeDocente())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if u.Nombre != "Augusta" || u.Apellido != "Byron" {
		t.Errorf("tenía que ganar lo que escribió la persona: %s %s", u.Nombre, u.Apellido)
	}
}

// given_name/family_name no son claims obligatorios: si el token no los
// trae y la persona tampoco los escribió, no hay con qué crear la cuenta.
func TestRegistrarConGoogle_SinNombreEnNingunLado_Error(t *testing.T) {
	identidad := identidadDePrueba()
	identidad.Nombre = ""
	identidad.Apellido = ""
	svc := nuevoServicioConGoogle(nuevoFakeRepo(), &fakeVerificadorGoogle{identidad: identidad})

	_, err := svc.RegistrarConGoogle(context.Background(), "un-token", "", "", solicitudDeDocente())

	if !errors.Is(err, ErrDatosObligatorios) {
		t.Fatalf("esperaba ErrDatosObligatorios, hubo: %v", err)
	}
}

func TestRegistrarConGoogle_YaVinculada_NoCreaOtra(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = usuarioDeGoogle("u1", "ada@escuela.edu.ar", "112233445566", domain.EstadoAprobada)
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	_, err := svc.RegistrarConGoogle(context.Background(), "un-token", "", "", solicitudDeDocente())

	if !errors.Is(err, ErrEmailYaRegistrado) {
		t.Fatalf("esperaba ErrEmailYaRegistrado, hubo: %v", err)
	}
	if len(repo.usuarios) != 1 {
		t.Errorf("se creó una cuenta duplicada: %d usuarios", len(repo.usuarios))
	}
}

func TestRegistrarConGoogle_EmailYaUsadoPorCuentaLocal_NoCreaOtra(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Nombre: "Ada", Apellido: "Lovelace",
		Email: "ada@escuela.edu.ar", PasswordHash: "hash:password123",
		Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	_, err := svc.RegistrarConGoogle(context.Background(), "un-token", "", "", solicitudDeDocente())

	if !errors.Is(err, ErrEmailYaRegistrado) {
		t.Fatalf("esperaba ErrEmailYaRegistrado, hubo: %v", err)
	}
	if len(repo.usuarios) != 1 {
		t.Errorf("se creó una cuenta duplicada: %d usuarios", len(repo.usuarios))
	}
}

// RF-01.3: el mensaje de una cuenta en BAJA es distinto del genérico, para
// que quien vuelve entienda que es su propia cuenta vieja.
func TestRegistrarConGoogle_EmailDeCuentaEnBaja(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Nombre: "Ada", Apellido: "Lovelace",
		Email: "ada@escuela.edu.ar", PasswordHash: "hash:password123",
		Rol: domain.RolDocente, Estado: domain.EstadoBaja,
	}
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	_, err := svc.RegistrarConGoogle(context.Background(), "un-token", "", "", solicitudDeDocente())

	if !errors.Is(err, ErrCuentaEnBaja) {
		t.Fatalf("esperaba ErrCuentaEnBaja, hubo: %v", err)
	}
}

// ── Convivencia con el login y el cambio de contraseña locales ─────────

// Una cuenta de Google no tiene contraseña.
func TestLogin_CuentaDeGoogle_NoDistingueDeUnEmailInexistente(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = usuarioDeGoogle("u1", "ada@escuela.edu.ar", "112233445566", domain.EstadoAprobada)

	verificaciones := 0
	svc := servicioConVerify(repo, func(password, hash string) (bool, error) {
		verificaciones++
		return false, nil
	})

	_, err := svc.Login(context.Background(), "ada@escuela.edu.ar", "loquesea", false)

	if !errors.Is(err, ErrCredencialesInvalidas) {
		t.Fatalf("esperaba ErrCredencialesInvalidas, hubo: %v", err)
	}
	// El mismo trabajo que haría contra un email inexistente: si acá se volviera
	// de inmediato, el tiempo de respuesta delataría que la cuenta existe y que
	// entra con Google.
	if verificaciones != 1 {
		t.Errorf("esperaba una verificación de descarte para igualar el tiempo, hubo %d", verificaciones)
	}
}

func TestCambiarPassword_CuentaDeGoogle_NoTieneQueCambiar(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = usuarioDeGoogle("u1", "ada@escuela.edu.ar", "112233445566", domain.EstadoAprobada)
	svc := nuevoServicioDeTest(repo)

	_, err := svc.CambiarPassword(context.Background(), "u1", "loquesea", "password-nueva", false)

	if !errors.Is(err, ErrCuentaSinPassword) {
		t.Fatalf("esperaba ErrCuentaSinPassword, hubo: %v", err)
	}
}

// El reset asistido por Admin (RF-01.6) sí funciona sobre una cuenta de
// Google: es la forma de devolverle el acceso a alguien que perdió su cuenta
// de Google.
func TestResetearPassword_CuentaDeGoogle_LeDaUnaContrasenia(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = usuarioDeGoogle("u1", "ada@escuela.edu.ar", "112233445566", domain.EstadoAprobada)
	svc := nuevoServicioDeTest(repo)

	temporal, err := svc.ResetearPassword(context.Background(), "u1")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	u := repo.usuarios["u1"]
	if !u.PuedeIngresarConPassword() {
		t.Error("después del reset la cuenta tiene que poder entrar con contraseña")
	}
	if !u.PuedeIngresarConGoogle() {
		t.Error("el reset no debe romper el vínculo con Google")
	}
	if temporal == "" {
		t.Error("el Admin necesita la temporal para dictársela")
	}
}
