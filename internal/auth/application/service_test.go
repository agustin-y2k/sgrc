package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// ── fakeRepo: repositorio en memoria para tests, sin Postgres ──────────

type fakeRepo struct {
	fotos    map[string]*domain.FotoDePerfil
	usuarios map[string]*domain.Usuario // por ID

	errBuscarPorEmail     error
	errBuscarPorID        error
	errBuscarPorGoogleSub error
	errCrear              error
	errGuardar            error
	errContarAdmins       error
	errEliminar           error
	errListar             error
	adminsAprobadosCount  int

	// Códigos de recuperación, indexados por usuario. Solo se guarda el
	// último sin usar: es exactamente el invariante que sostiene
	// CrearCodigoRecuperacion en Postgres (pedir uno nuevo invalida el
	// anterior), y tenerlo acá evita que un test pase contra el fake
	// apoyándose en algo que la implementación real no permite.
	codigos          map[string]*domain.CodigoRecuperacion
	errGuardarCodigo error
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		usuarios: make(map[string]*domain.Usuario),
		codigos:  make(map[string]*domain.CodigoRecuperacion),
	}
}

func (r *fakeRepo) CrearCodigoRecuperacion(ctx context.Context, c *domain.CodigoRecuperacion) error {
	r.codigos[c.UsuarioID] = c
	return nil
}

func (r *fakeRepo) BuscarCodigoVigenteDe(ctx context.Context, usuarioID string) (*domain.CodigoRecuperacion, error) {
	c, ok := r.codigos[usuarioID]
	if !ok || c.UsadoEn != nil {
		return nil, ErrCodigoNoEncontrado
	}
	return c, nil
}

func (r *fakeRepo) GuardarCodigoRecuperacion(ctx context.Context, c *domain.CodigoRecuperacion) error {
	if r.errGuardarCodigo != nil {
		return r.errGuardarCodigo
	}
	r.codigos[c.UsuarioID] = c
	return nil
}

func (r *fakeRepo) BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error) {
	if r.errBuscarPorEmail != nil {
		return nil, r.errBuscarPorEmail
	}
	for _, u := range r.usuarios {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, ErrUsuarioNoEncontrado
}

func (r *fakeRepo) BuscarPorID(ctx context.Context, id string) (*domain.Usuario, error) {
	if r.errBuscarPorID != nil {
		return nil, r.errBuscarPorID
	}
	u, ok := r.usuarios[id]
	if !ok {
		return nil, ErrUsuarioNoEncontrado
	}
	return u, nil
}

func (r *fakeRepo) BuscarPorGoogleSub(ctx context.Context, sub string) (*domain.Usuario, error) {
	if r.errBuscarPorGoogleSub != nil {
		return nil, r.errBuscarPorGoogleSub
	}
	// Igual que el repo real: un sub vacío no empata con nadie, ni siquiera
	// con las cuentas que no tienen ninguno.
	if sub == "" {
		return nil, ErrUsuarioNoEncontrado
	}
	for _, u := range r.usuarios {
		if u.GoogleSub == sub {
			return u, nil
		}
	}
	return nil, ErrUsuarioNoEncontrado
}

// EnTransaccion imita el todo-o-nada de Postgres: copia el estado antes de
// correr fn y lo restaura si falla.
func (r *fakeRepo) EnTransaccion(ctx context.Context, fn func(Repo) error) error {
	antes := make(map[string]*domain.Usuario, len(r.usuarios))
	for k, v := range r.usuarios {
		copia := *v
		antes[k] = &copia
	}
	if err := fn(r); err != nil {
		r.usuarios = antes
		return err
	}
	return nil
}

func (r *fakeRepo) Crear(ctx context.Context, u *domain.Usuario) error {
	if r.errCrear != nil {
		return r.errCrear
	}
	r.usuarios[u.ID] = u
	return nil
}

func (r *fakeRepo) Guardar(ctx context.Context, u *domain.Usuario) error {
	if r.errGuardar != nil {
		return r.errGuardar
	}
	r.usuarios[u.ID] = u
	return nil
}

func (r *fakeRepo) ContarAdminsAprobados(ctx context.Context) (int, error) {
	if r.errContarAdmins != nil {
		return 0, r.errContarAdmins
	}
	if r.adminsAprobadosCount > 0 {
		return r.adminsAprobadosCount, nil
	}
	n := 0
	for _, u := range r.usuarios {
		if u.EsAdmin() && u.Estado == domain.EstadoAprobada {
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) Eliminar(ctx context.Context, id string) error {
	if r.errEliminar != nil {
		return r.errEliminar
	}
	delete(r.usuarios, id)
	return nil
}

func (r *fakeRepo) Listar(ctx context.Context, filtroEstado *domain.Estado, filtroRol *domain.Rol, pagina paginacion.Pagina) ([]*domain.Usuario, int, error) {
	if r.errListar != nil {
		return nil, 0, r.errListar
	}
	var resultado []*domain.Usuario
	for _, id := range r.idsOrdenados() {
		u := r.usuarios[id]
		if filtroEstado != nil && u.Estado != *filtroEstado {
			continue
		}
		if filtroRol != nil && u.Rol != *filtroRol {
			continue
		}
		resultado = append(resultado, u)
	}

	total := len(resultado)
	desde := pagina.Offset()
	if desde >= total {
		return nil, total, nil
	}
	hasta := desde + pagina.Limit()
	if hasta > total {
		hasta = total
	}
	return resultado[desde:hasta], total, nil
}

// idsOrdenados da el orden estable que en el repo real pone el ORDER BY:
// sobre el map pelado, LIMIT/OFFSET devolvería una página distinta en cada
// corrida.
func (r *fakeRepo) idsOrdenados() []string {
	ids := make([]string, 0, len(r.usuarios))
	for id := range r.usuarios {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ── helpers de fábrica para las funciones inyectadas ───────────────────

func hashFalso(password string) (string, error) { return "hash:" + password, nil }
func verifyFalso(password, hash string) (bool, error) {
	return hash == "hash:"+password, nil
}
func firmarFalso(u *domain.Usuario) (string, error) { return "token-de-" + u.ID, nil }
func temporalFalso() (string, error)                { return "temporal123", nil }
func codigoFalso() (string, error)                  { return "123456", nil }

func relojFijo(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return "id-" + string(rune('0'+contadorID))
}

// ── fakeGestorMaterias / fakeCanceladorReservas ────────────────────────

type fakeGestorMaterias struct {
	materias            map[string][]string // usuarioID -> materiaIDs
	quedaOtroPorMateria map[string]bool     // materiaID -> bool
	errMaterias         error
	errQuedaOtro        error
	errRemover          error
	removidoDe          []string
}

func nuevoFakeGestorMaterias() *fakeGestorMaterias {
	return &fakeGestorMaterias{materias: map[string][]string{}, quedaOtroPorMateria: map[string]bool{}}
}

func (f *fakeGestorMaterias) MateriasDeDocente(ctx context.Context, usuarioID string) ([]string, error) {
	if f.errMaterias != nil {
		return nil, f.errMaterias
	}
	return f.materias[usuarioID], nil
}

func (f *fakeGestorMaterias) QuedaOtroDocenteActivo(ctx context.Context, materiaID, usuarioIDExcluido string) (bool, error) {
	if f.errQuedaOtro != nil {
		return false, f.errQuedaOtro
	}
	return f.quedaOtroPorMateria[materiaID], nil
}

func (f *fakeGestorMaterias) RemoverAsignacionesDeDocente(ctx context.Context, usuarioID string) error {
	if f.errRemover != nil {
		return f.errRemover
	}
	f.removidoDe = append(f.removidoDe, usuarioID)
	return nil
}

type fakeCanceladorReservas struct {
	canceladasPorMateria map[string]int
	err                  error
	// errPorMateria falla solo para algunas materias, para poder probar que
	// una falla puntual no se lleve puestas a las demás.
	errPorMateria      map[string]error
	llamadoParaMateria []string
}

func nuevoFakeCanceladorReservas() *fakeCanceladorReservas {
	return &fakeCanceladorReservas{canceladasPorMateria: map[string]int{}}
}

func (f *fakeCanceladorReservas) CancelarReservasFuturasDeMateria(ctx context.Context, materiaID, motivo string) (int, error) {
	f.llamadoParaMateria = append(f.llamadoParaMateria, materiaID)
	if f.err != nil {
		return 0, f.err
	}
	if err, hayError := f.errPorMateria[materiaID]; hayError {
		return 0, err
	}
	return f.canceladasPorMateria[materiaID], nil
}

func nuevoServicioDeTest(repo Repo) *Service {
	return nuevoServicioConCascada(repo, nuevoFakeGestorMaterias(), nuevoFakeCanceladorReservas())
}

func nuevoServicioConCascada(repo Repo, gestorMaterias GestorMateriasDocente, cancelador CanceladorReservasDeMateria) *Service {
	contadorID = 0
	return NewService(
		repo,
		eventbus.NewInMemoryEventBus(), // el bus real — ya está probado, no hace falta fakearlo
		hashFalso,
		verifyFalso,
		firmarFalso,
		idSecuencial,
		temporalFalso,
		codigoFalso,
		relojFijo(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)),
		gestorMaterias,
		cancelador,
		nil,  // sin ingreso con Google: los tests que lo usan arman el suyo
		true, // con correo: es lo que habilita la recuperación por autoservicio
	)
}

// servicioConFirmador permite espiar con qué usuario —y por lo tanto con
// qué VersionSesion— se firma el token. Es lo que hace falta para verificar
// que InvalidarSesiones corre ANTES de firmar (ver service_sesiones_test.go).
func servicioConFirmador(repo Repo, firmar TokenSigner) *Service {
	contadorID = 0
	return NewService(
		repo,
		eventbus.NewInMemoryEventBus(),
		hashFalso,
		verifyFalso,
		firmar,
		idSecuencial,
		temporalFalso,
		codigoFalso,
		relojFijo(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)),
		nuevoFakeGestorMaterias(),
		nuevoFakeCanceladorReservas(),
		nil,
		true,
	)
}

// servicioConVerify permite contar cuántas veces se verifica una
// contraseña, que es lo que distingue los dos caminos del login.
func servicioConVerify(repo Repo, verify VerifyFunc) *Service {
	contadorID = 0
	return NewService(
		repo,
		eventbus.NewInMemoryEventBus(),
		hashFalso,
		verify,
		firmarFalso,
		idSecuencial,
		temporalFalso,
		codigoFalso,
		relojFijo(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)),
		nuevoFakeGestorMaterias(),
		nuevoFakeCanceladorReservas(),
		nil,  // sin ingreso con Google: los tests que lo usan arman el suyo
		true, // con correo: es lo que habilita la recuperación por autoservicio
	)
}

// ── Registrar ───────────────────────────────────────────────────────────

func TestRegistrar_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	u, err := svc.Registrar(context.Background(), "Ada", "Lovelace", "ada@escuela.edu.ar", "password123", SolicitudDeAsignacion{})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.Estado != domain.EstadoPendiente {
		t.Errorf("estado inicial incorrecto: %s", u.Estado)
	}
	if u.Rol != domain.RolDocente {
		t.Errorf("rol inicial incorrecto: %s", u.Rol)
	}
	if u.PasswordHash != "hash:password123" {
		t.Errorf("password no se hasheó como se esperaba: %s", u.PasswordHash)
	}
}

func TestRegistrar_PublicaEventoParaAdmins(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	recibido := make(chan eventbus.Evento, 1)
	svc.bus.Subscribe("docente.registro.pendiente", func(e eventbus.Evento) {
		recibido <- e
	})

	_, err := svc.Registrar(context.Background(), "Ada", "Lovelace", "ada@escuela.edu.ar", "password123", SolicitudDeAsignacion{})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	select {
	case e := <-recibido:
		payload := e.Payload.(map[string]string)
		if payload["email"] != "ada@escuela.edu.ar" {
			t.Errorf("payload incorrecto: %v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("nunca se publicó el evento de registro pendiente")
	}
}

func TestRegistrar_PasswordCorta_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, err := svc.Registrar(context.Background(), "Ada", "Lovelace", "ada@escuela.edu.ar", "1234567", SolicitudDeAsignacion{})

	if !errors.Is(err, ErrPasswordCorta) {
		t.Fatalf("esperaba ErrPasswordCorta, obtuve %v", err)
	}
}

func TestRegistrar_EmailYaExiste_Activo_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["existente"] = &domain.Usuario{ID: "existente", Email: "ada@escuela.edu.ar", Estado: domain.EstadoAprobada}
	svc := nuevoServicioDeTest(repo)

	_, err := svc.Registrar(context.Background(), "Ada", "Lovelace", "ada@escuela.edu.ar", "password123", SolicitudDeAsignacion{})

	if !errors.Is(err, ErrEmailYaRegistrado) {
		t.Fatalf("esperaba ErrEmailYaRegistrado, obtuve %v", err)
	}
}

func TestRegistrar_EmailYaExiste_EnBaja_MensajeEspecifico(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["viejo"] = &domain.Usuario{ID: "viejo", Email: "ada@escuela.edu.ar", Estado: domain.EstadoBaja}
	svc := nuevoServicioDeTest(repo)

	_, err := svc.Registrar(context.Background(), "Ada", "Lovelace", "ada@escuela.edu.ar", "password123", SolicitudDeAsignacion{})

	if !errors.Is(err, ErrCuentaEnBaja) {
		t.Fatalf("esperaba ErrCuentaEnBaja, obtuve %v", err)
	}
}

func TestRegistrar_ErrorDelRepoAlCrear_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.errCrear = errors.New("la base está caída")
	svc := nuevoServicioDeTest(repo)

	_, err := svc.Registrar(context.Background(), "Ada", "Lovelace", "ada@escuela.edu.ar", "password123", SolicitudDeAsignacion{})

	if err == nil {
		t.Fatal("esperaba que el error del repo se propague")
	}
}

func TestRegistrar_CamposObligatoriosVacios_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())
	casos := []struct{ nombre, apellido, email string }{
		{"", "Lovelace", "ada@x.com"},
		{"Ada", "", "ada@x.com"},
		{"Ada", "Lovelace", ""},
	}
	for _, c := range casos {
		_, err := svc.Registrar(context.Background(), c.nombre, c.apellido, c.email, "password123", SolicitudDeAsignacion{})
		if err == nil {
			t.Errorf("caso %+v: esperaba error por campo obligatorio vacío", c)
		}
	}
}

// ── Login ───────────────────────────────────────────────────────────────

func TestLogin_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Email: "ada@x.com", PasswordHash: "hash:password123", Estado: domain.EstadoAprobada}
	svc := nuevoServicioDeTest(repo)

	res, err := svc.Login(context.Background(), "ada@x.com", "password123")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if res.Token != "token-de-u1" {
		t.Errorf("token incorrecto: %s", res.Token)
	}
}

func TestLogin_UsuarioNoExiste_CredencialesInvalidas(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, err := svc.Login(context.Background(), "nadie@x.com", "cualquiera")

	if !errors.Is(err, ErrCredencialesInvalidas) {
		t.Fatalf("esperaba ErrCredencialesInvalidas, obtuve %v", err)
	}
}

// El mensaje de error ya era el mismo en los dos casos; el TIEMPO no. Con
// email inexistente se volvía sin hashear nada, así que medir la respuesta
// alcanzaba para enumerar quién tiene cuenta. Lo que se verifica es que los
// dos caminos pasen por verify la misma cantidad de veces — medir
// milisegundos en un test sería una fuente de fallos intermitentes.
func TestLogin_EmailInexistente_GastaElMismoTiempoQueUnoReal(t *testing.T) {
	contarVerificaciones := func() (VerifyFunc, *int) {
		n := 0
		return func(password, hash string) (bool, error) {
			n++
			return hash == "hash:"+password, nil
		}, &n
	}

	repoConUsuario := nuevoFakeRepo()
	repoConUsuario.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@x.com", PasswordHash: "hash:password123", Estado: domain.EstadoAprobada,
	}

	verifyReal, vecesReal := contarVerificaciones()
	svcReal := servicioConVerify(repoConUsuario, verifyReal)
	if _, err := svcReal.Login(context.Background(), "ada@x.com", "incorrecta"); !errors.Is(err, ErrCredencialesInvalidas) {
		t.Fatalf("esperaba ErrCredencialesInvalidas, obtuve %v", err)
	}

	verifyFantasma, vecesFantasma := contarVerificaciones()
	svcFantasma := servicioConVerify(nuevoFakeRepo(), verifyFantasma)
	if _, err := svcFantasma.Login(context.Background(), "nadie@x.com", "incorrecta"); !errors.Is(err, ErrCredencialesInvalidas) {
		t.Fatalf("esperaba ErrCredencialesInvalidas, obtuve %v", err)
	}

	if *vecesReal != 1 {
		t.Fatalf("el login contra un email real verificó %d veces, esperaba 1", *vecesReal)
	}
	if *vecesFantasma != *vecesReal {
		t.Errorf("email inexistente verificó %d veces y uno real %d: la diferencia de tiempo es el oráculo",
			*vecesFantasma, *vecesReal)
	}
}

// El hash de descarte se calcula una sola vez: si se recalculara en cada
// intento fallido, un endpoint sin autenticar pasaría a costar 64 MB de
// argon2 por request, que es peor que el problema que resuelve.
func TestLogin_ElHashDeDescarteSeCalculaUnaSolaVez(t *testing.T) {
	hasheos := 0
	svc := NewService(
		nuevoFakeRepo(),
		eventbus.NewInMemoryEventBus(),
		func(password string) (string, error) { hasheos++; return "hash:" + password, nil },
		verifyFalso,
		firmarFalso,
		idSecuencial,
		temporalFalso,
		codigoFalso,
		relojFijo(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)),
		nuevoFakeGestorMaterias(),
		nuevoFakeCanceladorReservas(),
		nil,  // sin ingreso con Google: los tests que lo usan arman el suyo
		true, // con correo: es lo que habilita la recuperación por autoservicio
	)

	for i := 0; i < 5; i++ {
		if _, err := svc.Login(context.Background(), "nadie@x.com", "cualquiera"); err == nil {
			t.Fatal("esperaba que fallara")
		}
	}

	if hasheos != 1 {
		t.Errorf("se hasheó %d veces en 5 intentos, esperaba 1", hasheos)
	}
}

func TestLogin_PasswordIncorrecta_CredencialesInvalidas(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Email: "ada@x.com", PasswordHash: "hash:password123", Estado: domain.EstadoAprobada}
	svc := nuevoServicioDeTest(repo)

	_, err := svc.Login(context.Background(), "ada@x.com", "incorrecta")

	if !errors.Is(err, ErrCredencialesInvalidas) {
		t.Fatalf("esperaba ErrCredencialesInvalidas, obtuve %v", err)
	}
}

func TestLogin_CuentaNoAprobada_Rechazado(t *testing.T) {
	casos := []domain.Estado{domain.EstadoPendiente, domain.EstadoRechazada, domain.EstadoBaja}
	for _, estado := range casos {
		repo := nuevoFakeRepo()
		repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Email: "ada@x.com", PasswordHash: "hash:password123", Estado: estado}
		svc := nuevoServicioDeTest(repo)

		_, err := svc.Login(context.Background(), "ada@x.com", "password123")

		if !errors.Is(err, ErrCuentaNoHabilitada) {
			t.Errorf("estado %s: esperaba ErrCuentaNoHabilitada, obtuve %v", estado, err)
		}
	}
}

func TestLogin_DebeCambiarPassword_SePropagaEnElResultado(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@x.com", PasswordHash: "hash:password123",
		Estado: domain.EstadoAprobada, DebeCambiarPassword: true,
	}
	svc := nuevoServicioDeTest(repo)

	res, err := svc.Login(context.Background(), "ada@x.com", "password123")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !res.DebeCambiarPassword {
		t.Error("esperaba DebeCambiarPassword=true en el resultado")
	}
}

// ── Aprobar / Rechazar ────────────────────────────────────────────────

func TestAprobar_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Estado: domain.EstadoPendiente}
	svc := nuevoServicioDeTest(repo)

	err := svc.Aprobar(context.Background(), "u1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.usuarios["u1"].Estado != domain.EstadoAprobada {
		t.Errorf("estado final incorrecto: %s", repo.usuarios["u1"].Estado)
	}
}

func TestAprobar_TransicionInvalida_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Estado: domain.EstadoBaja} // terminal
	svc := nuevoServicioDeTest(repo)

	err := svc.Aprobar(context.Background(), "u1")

	if !errors.Is(err, domain.ErrTransicionInvalida) {
		t.Fatalf("esperaba ErrTransicionInvalida, obtuve %v", err)
	}
}

func TestRechazar_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Estado: domain.EstadoPendiente}
	svc := nuevoServicioDeTest(repo)

	err := svc.Rechazar(context.Background(), "u1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.usuarios["u1"].Estado != domain.EstadoRechazada {
		t.Errorf("estado final incorrecto: %s", repo.usuarios["u1"].Estado)
	}
}

// ── DarDeBaja + protección del último Admin ────────────────────────────

func TestDarDeBaja_Docente_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Rol: domain.RolDocente, Estado: domain.EstadoAprobada}
	svc := nuevoServicioDeTest(repo)

	err := svc.DarDeBaja(context.Background(), "d1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.usuarios["d1"].Estado != domain.EstadoBaja {
		t.Errorf("estado final incorrecto: %s", repo.usuarios["d1"].Estado)
	}
}

func TestDarDeBaja_Docente_MateriaHuerfana_CancelaEnCascadaYRemueveAsignaciones(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Rol: domain.RolDocente, Estado: domain.EstadoAprobada}

	gestor := nuevoFakeGestorMaterias()
	gestor.materias["d1"] = []string{"materia-huerfana"}
	gestor.quedaOtroPorMateria["materia-huerfana"] = false // era el único docente

	cancelador := nuevoFakeCanceladorReservas()
	cancelador.canceladasPorMateria["materia-huerfana"] = 3

	svc := nuevoServicioConCascada(repo, gestor, cancelador)

	err := svc.DarDeBaja(context.Background(), "d1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(cancelador.llamadoParaMateria) != 1 || cancelador.llamadoParaMateria[0] != "materia-huerfana" {
		t.Errorf("esperaba que se cancelen las reservas de materia-huerfana, se llamó para: %v", cancelador.llamadoParaMateria)
	}
	if len(gestor.removidoDe) != 1 || gestor.removidoDe[0] != "d1" {
		t.Errorf("esperaba que se remuevan las asignaciones de d1, se removió: %v", gestor.removidoDe)
	}
}

func TestDarDeBaja_Docente_OtroDocenteActivo_NoCancelaNada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Rol: domain.RolDocente, Estado: domain.EstadoAprobada}

	gestor := nuevoFakeGestorMaterias()
	gestor.materias["d1"] = []string{"materia-con-otro-docente"}
	gestor.quedaOtroPorMateria["materia-con-otro-docente"] = true // sigue habiendo otro

	cancelador := nuevoFakeCanceladorReservas()
	svc := nuevoServicioConCascada(repo, gestor, cancelador)

	err := svc.DarDeBaja(context.Background(), "d1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(cancelador.llamadoParaMateria) != 0 {
		t.Errorf("no debería haberse cancelado nada si sigue habiendo otro docente, se llamó para: %v", cancelador.llamadoParaMateria)
	}
	// Las asignaciones del docente dado de baja SÍ se remueven igual —
	// lo único que no pasa es la cascada de cancelación.
	if len(gestor.removidoDe) != 1 {
		t.Error("las asignaciones del docente deberían removerse de todas formas")
	}
}

func TestDarDeBaja_Docente_PublicaEventoDeMateriaHuerfana(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Rol: domain.RolDocente, Estado: domain.EstadoAprobada}

	gestor := nuevoFakeGestorMaterias()
	gestor.materias["d1"] = []string{"materia-huerfana"}
	gestor.quedaOtroPorMateria["materia-huerfana"] = false

	svc := nuevoServicioConCascada(repo, gestor, nuevoFakeCanceladorReservas())

	recibido := make(chan eventbus.Evento, 1)
	svc.bus.Subscribe("docente.baja.materia-huerfana", func(e eventbus.Evento) {
		recibido <- e
	})

	if err := svc.DarDeBaja(context.Background(), "d1"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	select {
	case e := <-recibido:
		payload := e.Payload.(map[string]any)
		if payload["materiaId"] != "materia-huerfana" {
			t.Errorf("payload incorrecto: %+v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("nunca se publicó el evento de materia huérfana")
	}
}

func TestDarDeBaja_Docente_PublicaEventoDeNotificarAdmin_SiSigueOtroDocente(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Rol: domain.RolDocente, Estado: domain.EstadoAprobada}

	gestor := nuevoFakeGestorMaterias()
	gestor.materias["d1"] = []string{"materia-con-otro-docente"}
	gestor.quedaOtroPorMateria["materia-con-otro-docente"] = true

	svc := nuevoServicioConCascada(repo, gestor, nuevoFakeCanceladorReservas())

	recibido := make(chan eventbus.Evento, 1)
	svc.bus.Subscribe("docente.baja.notificar_admin", func(e eventbus.Evento) {
		recibido <- e
	})

	if err := svc.DarDeBaja(context.Background(), "d1"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	select {
	case <-recibido:
		// esperado
	case <-time.After(time.Second):
		t.Fatal("nunca se publicó el evento de notificación a Admin")
	}
}

func TestDarDeBaja_Docente_ErrorListandoMaterias_SePropagaYNoCambiaEstado(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Rol: domain.RolDocente, Estado: domain.EstadoAprobada}

	gestor := nuevoFakeGestorMaterias()
	gestor.errMaterias = errors.New("academic caído")
	svc := nuevoServicioConCascada(repo, gestor, nuevoFakeCanceladorReservas())

	err := svc.DarDeBaja(context.Background(), "d1")

	if err == nil {
		t.Fatal("esperaba que el error se propague")
	}
	if repo.usuarios["d1"].Estado != domain.EstadoAprobada {
		t.Error("el estado no debería haber cambiado si falló la verificación previa")
	}
}

func TestDarDeBaja_Docente_ErrorCancelandoReservas_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Rol: domain.RolDocente, Estado: domain.EstadoAprobada}

	gestor := nuevoFakeGestorMaterias()
	gestor.materias["d1"] = []string{"materia-huerfana"}
	gestor.quedaOtroPorMateria["materia-huerfana"] = false

	cancelador := nuevoFakeCanceladorReservas()
	cancelador.err = errors.New("reservation caído")

	svc := nuevoServicioConCascada(repo, gestor, cancelador)

	err := svc.DarDeBaja(context.Background(), "d1")

	if err == nil {
		t.Fatal("esperaba que el error de cancelación se propague")
	}
	// La baja del usuario ya se aplicó — eso es intencional (ver comentario
	// en DarDeBaja): no debe quedar bloqueada esperando a que reservation
	// responda bien.
	if repo.usuarios["d1"].Estado != domain.EstadoBaja {
		t.Error("la baja del usuario ya debería haberse aplicado antes del error de cascada")
	}
	// Los vínculos, en cambio, tienen que sobrevivir: son el único registro
	// de qué materias quedaron con reservas por cancelar, y la operación no
	// se puede reintentar (BAJA→BAJA es inválida, RF-02.9). Borrarlos acá
	// dejaba el sistema en un estado imposible de arreglar salvo por SQL.
	if len(gestor.removidoDe) != 0 {
		t.Error("con la cascada fallada, las asignaciones no deberían haberse borrado")
	}
}

func TestDarDeBaja_Docente_UnaMateriaFalla_LasDemasSeCancelanIgual(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Rol: domain.RolDocente, Estado: domain.EstadoAprobada}

	gestor := nuevoFakeGestorMaterias()
	gestor.materias["d1"] = []string{"materia-a", "materia-b", "materia-c"}
	for _, m := range gestor.materias["d1"] {
		gestor.quedaOtroPorMateria[m] = false
	}

	cancelador := nuevoFakeCanceladorReservas()
	cancelador.errPorMateria = map[string]error{"materia-a": errors.New("reservation caído")}

	svc := nuevoServicioConCascada(repo, gestor, cancelador)

	err := svc.DarDeBaja(context.Background(), "d1")

	if err == nil {
		t.Fatal("esperaba error: quedó una materia sin cancelar")
	}
	// Cortar en la primera materia dejaba a las otras dos con reservas
	// vivas y sin docente, por una falla que no tenía nada que ver con
	// ellas.
	if len(cancelador.llamadoParaMateria) != 3 {
		t.Errorf("esperaba que se intenten las 3 materias, se intentó: %v", cancelador.llamadoParaMateria)
	}
	if !strings.Contains(err.Error(), "materia-a") {
		t.Errorf("el error debería nombrar la materia que quedó pendiente, dice: %v", err)
	}
}

func TestDarDeBaja_UltimoAdmin_Rechazado(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["a1"] = &domain.Usuario{ID: "a1", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada}
	repo.adminsAprobadosCount = 1 // es el único
	svc := nuevoServicioDeTest(repo)

	err := svc.DarDeBaja(context.Background(), "a1")

	if !errors.Is(err, ErrUltimoAdmin) {
		t.Fatalf("esperaba ErrUltimoAdmin, obtuve %v", err)
	}
	if repo.usuarios["a1"].Estado != domain.EstadoAprobada {
		t.Error("el estado no debería haber cambiado si se rechazó por ser el último admin")
	}
}

func TestDarDeBaja_AdminConOtrosAdmins_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["a1"] = &domain.Usuario{ID: "a1", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada}
	repo.adminsAprobadosCount = 2 // hay otro más
	svc := nuevoServicioDeTest(repo)

	err := svc.DarDeBaja(context.Background(), "a1")

	if err != nil {
		t.Fatalf("no debería fallar si hay otro admin: %v", err)
	}
	if repo.usuarios["a1"].Estado != domain.EstadoBaja {
		t.Errorf("estado final incorrecto: %s", repo.usuarios["a1"].Estado)
	}
}

// ── ResetearPassword ────────────────────────────────────────────────────

func TestResetearPassword_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", PasswordHash: "hash:viejo"}
	svc := nuevoServicioDeTest(repo)

	temporal, err := svc.ResetearPassword(context.Background(), "u1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if temporal != "temporal123" {
		t.Errorf("temporal incorrecta: %s", temporal)
	}
	if !repo.usuarios["u1"].DebeCambiarPassword {
		t.Error("debería marcar DebeCambiarPassword=true")
	}
	if repo.usuarios["u1"].PasswordHash != "hash:temporal123" {
		t.Errorf("el hash no se actualizó: %s", repo.usuarios["u1"].PasswordHash)
	}
}

// ── CambiarPassword ─────────────────────────────────────────────────────

func TestCambiarPassword_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", PasswordHash: "hash:actual", DebeCambiarPassword: true}
	svc := nuevoServicioDeTest(repo)

	token, err := svc.CambiarPassword(context.Background(), "u1", "actual", "nuevapassword123")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.usuarios["u1"].DebeCambiarPassword {
		t.Error("debería limpiar DebeCambiarPassword al cambiarla")
	}
	// El token viejo lleva DebeCambiarPassword=true congelado en los claims;
	// sin uno nuevo, quien acaba de cambiarla queda bloqueado por su propio
	// cambio exitoso hasta que expire (RF-01.6).
	if token == "" {
		t.Error("debería devolver un token nuevo")
	}
}

func TestCambiarPassword_ActualIncorrecta_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", PasswordHash: "hash:actual"}
	svc := nuevoServicioDeTest(repo)

	_, err := svc.CambiarPassword(context.Background(), "u1", "incorrecta", "nuevapassword123")

	// Propio y no ErrCredencialesInvalidas: ese mapea a 401, y un 401 con
	// token válido hace que el cliente cierre la sesión. Equivocarse
	// tipeando la contraseña actual no puede echar a nadie del sistema.
	if !errors.Is(err, ErrPasswordActualIncorrecta) {
		t.Fatalf("esperaba ErrPasswordActualIncorrecta, obtuve %v", err)
	}
	if errors.Is(err, ErrCredencialesInvalidas) {
		t.Fatal("no puede seguir matcheando ErrCredencialesInvalidas: volvería a dar 401")
	}
}

func TestCambiarPassword_NuevaCorta_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", PasswordHash: "hash:actual"}
	svc := nuevoServicioDeTest(repo)

	_, err := svc.CambiarPassword(context.Background(), "u1", "actual", "corta")

	if !errors.Is(err, ErrPasswordCorta) {
		t.Fatalf("esperaba ErrPasswordCorta, obtuve %v", err)
	}
}

// ── EliminarDefinitivamente ─────────────────────────────────────────────

// Los dos estados terminales se pueden eliminar. RECHAZADA importa tanto
// como BAJA: es el único camino que tiene un Admin para deshacer un rechazo
// equivocado y liberar el email, porque RECHAZADA no transiciona a ningún
// otro estado.
func TestEliminarDefinitivamente_DesdeEstadoTerminal_OK(t *testing.T) {
	casos := []domain.Estado{domain.EstadoBaja, domain.EstadoRechazada}
	for _, estado := range casos {
		repo := nuevoFakeRepo()
		repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Estado: estado}
		svc := nuevoServicioDeTest(repo)

		err := svc.EliminarDefinitivamente(context.Background(), "u1")

		if err != nil {
			t.Fatalf("estado %s: no debería fallar: %v", estado, err)
		}
		if _, existe := repo.usuarios["u1"]; existe {
			t.Errorf("estado %s: el usuario debería haberse eliminado", estado)
		}
	}
}

// Una cuenta viva no se borra: APROBADA hay que darla de baja primero (para
// que corra la cascada que cancela sus reservas) y PENDIENTE hay que
// resolverla, no hacerla desaparecer mientras alguien espera respuesta.
func TestEliminarDefinitivamente_CuentaNoCerrada_Rechazado(t *testing.T) {
	casos := []domain.Estado{domain.EstadoPendiente, domain.EstadoAprobada}
	for _, estado := range casos {
		repo := nuevoFakeRepo()
		repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Estado: estado}
		svc := nuevoServicioDeTest(repo)

		err := svc.EliminarDefinitivamente(context.Background(), "u1")

		if !errors.Is(err, ErrSoloDesdeBajaORechazada) {
			t.Errorf("estado %s: esperaba ErrSoloDesdeBajaORechazada, obtuve %v", estado, err)
		}
		if _, existe := repo.usuarios["u1"]; !existe {
			t.Errorf("estado %s: no debería haberse eliminado", estado)
		}
	}
}

// ── Listar ──────────────────────────────────────────────────────────────

func TestListar_SinFiltros_DevuelveTodos(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Rol: domain.RolDocente, Estado: domain.EstadoPendiente}
	repo.usuarios["u2"] = &domain.Usuario{ID: "u2", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada}
	svc := nuevoServicioDeTest(repo)

	usuarios, total, err := svc.Listar(context.Background(), nil, nil, paginacion.PorDefecto())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(usuarios) != 2 || total != 2 {
		t.Fatalf("esperaba 2 usuarios, obtuve %d (total %d)", len(usuarios), total)
	}
}

func TestListar_FiltraPorEstado(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Estado: domain.EstadoPendiente}
	repo.usuarios["u2"] = &domain.Usuario{ID: "u2", Estado: domain.EstadoAprobada}
	svc := nuevoServicioDeTest(repo)

	pendiente := domain.EstadoPendiente
	usuarios, _, err := svc.Listar(context.Background(), &pendiente, nil, paginacion.PorDefecto())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(usuarios) != 1 || usuarios[0].ID != "u1" {
		t.Fatalf("filtro por estado no funcionó como se esperaba: %+v", usuarios)
	}
}

func TestListar_FiltraPorRol(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Rol: domain.RolDocente}
	repo.usuarios["u2"] = &domain.Usuario{ID: "u2", Rol: domain.RolAdmin}
	svc := nuevoServicioDeTest(repo)

	admin := domain.RolAdmin
	usuarios, _, err := svc.Listar(context.Background(), nil, &admin, paginacion.PorDefecto())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(usuarios) != 1 || usuarios[0].ID != "u2" {
		t.Fatalf("filtro por rol no funcionó como se esperaba: %+v", usuarios)
	}
}

func TestListar_ErrorDelRepo_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.errListar = errors.New("la base está caída")
	svc := nuevoServicioDeTest(repo)

	_, _, err := svc.Listar(context.Background(), nil, nil, paginacion.PorDefecto())

	if err == nil {
		t.Fatal("esperaba que el error del repo se propague")
	}
}

// Una Pagina en cero es lo que recibe cualquier llamador que no la complete:
// sin este relleno saldría LIMIT 0, o sea cero usuarios y ningún error.
func TestListar_PaginaEnCero_UsaLaVentanaPorDefecto(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Rol: domain.RolDocente}
	svc := nuevoServicioDeTest(repo)

	usuarios, total, err := svc.Listar(context.Background(), nil, nil, paginacion.Pagina{})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(usuarios) != 1 || total != 1 {
		t.Fatalf("esperaba el único usuario, obtuve %d (total %d)", len(usuarios), total)
	}
}

// El total tiene que contar todos los que matchean el filtro, no los de la
// página: es lo único con lo que la pantalla sabe si hay una siguiente.
func TestListar_SegundaPagina_TotalEsElDeLaColeccion(t *testing.T) {
	repo := nuevoFakeRepo()
	for _, id := range []string{"u1", "u2", "u3", "u4", "u5"} {
		repo.usuarios[id] = &domain.Usuario{ID: id, Rol: domain.RolDocente}
	}
	svc := nuevoServicioDeTest(repo)

	usuarios, total, err := svc.Listar(context.Background(), nil, nil, paginacion.Pagina{Numero: 2, Tamanio: 2})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, esperaba 5", total)
	}
	if len(usuarios) != 2 || usuarios[0].ID != "u3" || usuarios[1].ID != "u4" {
		t.Fatalf("página equivocada: %+v", usuarios)
	}
}

// ── CrearAdmin ──────────────────────────────────────────────────────────

func TestCrearAdmin_OK_QuedaAprobadaDeInmediato(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	u, err := svc.CrearAdmin(context.Background(), "admin-actor", "Grace", "Hopper", "grace@escuela.edu.ar", "password123")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.Rol != domain.RolAdmin {
		t.Errorf("rol incorrecto: %s", u.Rol)
	}
	if u.Estado != domain.EstadoAprobada {
		t.Errorf("un admin creado por otro admin debería quedar APROBADA de inmediato, quedó: %s", u.Estado)
	}
	if u.FechaAprobacion == nil {
		t.Error("FechaAprobacion debería quedar seteada")
	}
	if u.AprobadoPor == nil || *u.AprobadoPor != "admin-actor" {
		t.Errorf("AprobadoPor incorrecto: %v", u.AprobadoPor)
	}
}

func TestCrearAdmin_NoPublicaEventoDeRegistroPendiente(t *testing.T) {
	// A diferencia de Registrar (docente), crear un Admin no dispara la
	// notificación de "cuenta pendiente" — porque no queda pendiente.
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	recibido := make(chan eventbus.Evento, 1)
	svc.bus.Subscribe("docente.registro.pendiente", func(e eventbus.Evento) {
		recibido <- e
	})

	_, err := svc.CrearAdmin(context.Background(), "admin-actor", "Grace", "Hopper", "grace@escuela.edu.ar", "password123")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	select {
	case e := <-recibido:
		t.Fatalf("no debería haberse publicado ningún evento, se publicó: %+v", e)
	case <-time.After(100 * time.Millisecond):
		// esperado: nada llegó
	}
}

func TestCrearAdmin_EmailYaExiste_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["existente"] = &domain.Usuario{ID: "existente", Email: "grace@escuela.edu.ar", Estado: domain.EstadoAprobada}
	svc := nuevoServicioDeTest(repo)

	_, err := svc.CrearAdmin(context.Background(), "admin-actor", "Grace", "Hopper", "grace@escuela.edu.ar", "password123")

	if !errors.Is(err, ErrEmailYaRegistrado) {
		t.Fatalf("esperaba ErrEmailYaRegistrado, obtuve %v", err)
	}
}

func TestCrearAdmin_PasswordCorta_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, err := svc.CrearAdmin(context.Background(), "admin-actor", "Grace", "Hopper", "grace@escuela.edu.ar", "corta")

	if !errors.Is(err, ErrPasswordCorta) {
		t.Fatalf("esperaba ErrPasswordCorta, obtuve %v", err)
	}
}

// ── ObtenerPerfil ─────────────────────────────────────────────────────

func TestObtenerPerfil_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Nombre: "Ada", Email: "ada@x.com"}
	svc := nuevoServicioDeTest(repo)

	u, err := svc.ObtenerPerfil(context.Background(), "u1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.Nombre != "Ada" {
		t.Errorf("perfil incorrecto: %+v", u)
	}
}

func TestObtenerPerfil_NoExiste_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, err := svc.ObtenerPerfil(context.Background(), "no-existe")

	if !errors.Is(err, ErrUsuarioNoEncontrado) {
		t.Fatalf("esperaba ErrUsuarioNoEncontrado, obtuve %v", err)
	}
}

// ── El email identifica la cuenta sin importar cómo se tipeó ───────────

// El bug: registrarse con una capitalización y entrar con otra devolvía
// "credenciales inválidas" aunque la contraseña fuera correcta.
func TestLogin_OtraCapitalizacionDelMismoEmail_Entra(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	if _, err := svc.Registrar(context.Background(), "Juan", "Perez", "Juan.Perez@escuela.edu.ar", "unaClave123", SolicitudDeAsignacion{}); err != nil {
		t.Fatalf("registro: %v", err)
	}
	// Aprobar para poder loguear (RF-01.3 deja la cuenta en PENDIENTE).
	for _, u := range repo.usuarios {
		u.Estado = domain.EstadoAprobada
	}

	for _, comoLoEscribe := range []string{
		"juan.perez@escuela.edu.ar",
		"JUAN.PEREZ@ESCUELA.EDU.AR",
		"  Juan.Perez@escuela.edu.ar  ",
	} {
		if _, err := svc.Login(context.Background(), comoLoEscribe, "unaClave123"); err != nil {
			t.Errorf("login con %q debería funcionar: %v", comoLoEscribe, err)
		}
	}
}

// Y el reverso: dos capitalizaciones del mismo buzón no pueden convivir
// como dos cuentas distintas.
func TestRegistrar_MismoEmailOtraCapitalizacion_Rechazado(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	if _, err := svc.Registrar(context.Background(), "Juan", "Perez", "Juan.Perez@escuela.edu.ar", "unaClave123", SolicitudDeAsignacion{}); err != nil {
		t.Fatalf("primer registro: %v", err)
	}

	_, err := svc.Registrar(context.Background(), "Juan", "Perez", "juan.perez@escuela.edu.ar", "unaClave123", SolicitudDeAsignacion{})
	if !errors.Is(err, ErrEmailYaRegistrado) {
		t.Fatalf("esperaba ErrEmailYaRegistrado, obtuve %v", err)
	}
}

// El email se guarda ya normalizado, así que lo que ve un Admin en el
// listado y lo que viaja en el evento de RF-05.6 es siempre la misma forma.
func TestRegistrar_GuardaElEmailNormalizado(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	u, err := svc.Registrar(context.Background(), " Juan ", " Perez ", "  Juan.Perez@Escuela.Edu.Ar ", "unaClave123", SolicitudDeAsignacion{})
	if err != nil {
		t.Fatalf("registro: %v", err)
	}
	if u.Email != "juan.perez@escuela.edu.ar" {
		t.Errorf("email guardado sin normalizar: %q", u.Email)
	}
	if u.Nombre != "Juan" || u.Apellido != "Perez" {
		t.Errorf("nombre/apellido sin recortar: %q %q", u.Nombre, u.Apellido)
	}
}

func TestRegistrar_EmailSinFormato_Rechazado(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, err := svc.Registrar(context.Background(), "Juan", "Perez", "no-es-un-email", "unaClave123", SolicitudDeAsignacion{})
	if !errors.Is(err, domain.ErrEmailInvalido) {
		t.Fatalf("esperaba ErrEmailInvalido, obtuve %v", err)
	}
}

// Tiene que llegar como sentinel: un fmt.Errorf suelto cae en el 500
// genérico de mapearError en vez de decir qué faltaba.
func TestRegistrar_NombreVacio_ErrorDeDatosObligatorios(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, err := svc.Registrar(context.Background(), "   ", "Perez", "juan@escuela.edu.ar", "unaClave123", SolicitudDeAsignacion{})
	if !errors.Is(err, ErrDatosObligatorios) {
		t.Fatalf("esperaba ErrDatosObligatorios, obtuve %v", err)
	}
}

// RF-01.3 + RF-02.6: lo que el docente declara al registrarse es lo que el
// Admin va a leer al aprobarlo, así que tiene que llegar hasta la fila.
func TestRegistrar_GuardaLaMateriaYElCursoSolicitados(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	u, err := svc.Registrar(context.Background(), "Ada", "Lovelace", "ada@escuela.edu.ar", "password123",
		SolicitudDeAsignacion{Curso: "  5°A  ", Materia: " Programación "})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	// Se guardan sin espacios de más: llegan de un formulario.
	if u.CursoSolicitado != "5°A" || u.MateriaSolicitada != "Programación" {
		t.Errorf("solicitud mal guardada: curso=%q materia=%q", u.CursoSolicitado, u.MateriaSolicitada)
	}
}

func TestRegistrar_GuardaElRolSolicitado(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	u, err := svc.Registrar(context.Background(), "Ada", "Lovelace", "ada@escuela.edu.ar", "password123",
		SolicitudDeAsignacion{Curso: "5°A", Materia: "Programación", Rol: "SUPLENTE"})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.RolSolicitado != "SUPLENTE" {
		t.Errorf("rolSolicitado = %q, esperaba SUPLENTE", u.RolSolicitado)
	}
}

// A diferencia del curso y la materia —texto libre, porque nombran cosas que
// quizás todavía no existen— el rol tiene una lista cerrada.
func TestRegistrar_RolSolicitadoInvalido_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	_, err := svc.Registrar(context.Background(), "Ada", "Lovelace", "ada@escuela.edu.ar", "password123",
		SolicitudDeAsignacion{Rol: "PROFESOR"})

	if !errors.Is(err, domain.ErrRolSolicitadoInvalido) {
		t.Fatalf("esperaba ErrRolSolicitadoInvalido, obtuve %v", err)
	}
}

// Son opcionales: quien todavía no sabe qué va a dictar se registra igual y
// lo arregla con el Admin.
func TestRegistrar_SinSolicitud_NoFalla(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	u, err := svc.Registrar(context.Background(), "Ada", "Lovelace", "ada@escuela.edu.ar", "password123",
		SolicitudDeAsignacion{})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.CursoSolicitado != "" || u.MateriaSolicitada != "" {
		t.Errorf("esperaba la solicitud vacía: %+v", u)
	}
}

// Un Admin lo crea otro Admin ya aprobado: no se autorregistra para dictar.
func TestCrearAdmin_NoLlevaSolicitud(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	u, err := svc.CrearAdmin(context.Background(), "admin-actor", "Grace", "Hopper", "grace@escuela.edu.ar", "password123")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.CursoSolicitado != "" || u.MateriaSolicitada != "" {
		t.Errorf("un Admin no declara curso ni materia: %+v", u)
	}
}

// ── PromoverAAdmin ──────────────────────────────────────────────────────

func TestPromoverAAdmin_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@escuela.edu.ar", Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	svc := nuevoServicioDeTest(repo)

	if err := svc.PromoverAAdmin(context.Background(), "u1"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if !repo.usuarios["u1"].EsAdmin() {
		t.Error("la promoción no se persistió")
	}
}

// Promover conserva todo lo demás de la cuenta. Un docente que pasa a
// coordinar suele seguir dando clase: borrarle nada por una promoción le
// cancelaría las clases que ya tiene tomadas.
func TestPromoverAAdmin_NoTocaNadaMasDeLaCuenta(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Nombre: "Ada", Apellido: "Lovelace", Email: "ada@escuela.edu.ar",
		PasswordHash: "hash:password123", GoogleSub: "112233",
		Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
		CursoSolicitado: "5°A", MateriaSolicitada: "Programación",
	}
	svc := nuevoServicioDeTest(repo)

	if err := svc.PromoverAAdmin(context.Background(), "u1"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	u := repo.usuarios["u1"]
	if u.Estado != domain.EstadoAprobada {
		t.Errorf("el estado no debería cambiar: %s", u.Estado)
	}
	if u.PasswordHash != "hash:password123" || u.GoogleSub != "112233" {
		t.Error("las formas de ingreso tienen que quedar intactas")
	}
	if u.Email != "ada@escuela.edu.ar" || u.Nombre != "Ada" {
		t.Error("los datos personales tienen que quedar intactos")
	}
}

func TestPromoverAAdmin_CuentaPendiente_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Rol: domain.RolDocente, Estado: domain.EstadoPendiente,
	}
	svc := nuevoServicioDeTest(repo)

	err := svc.PromoverAAdmin(context.Background(), "u1")

	if !errors.Is(err, domain.ErrPromocionInvalida) {
		t.Fatalf("esperaba ErrPromocionInvalida, hubo: %v", err)
	}
	if repo.usuarios["u1"].EsAdmin() {
		t.Error("no debería haber promovido nada")
	}
}

func TestPromoverAAdmin_YaEsAdmin_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada,
	}
	svc := nuevoServicioDeTest(repo)

	if err := svc.PromoverAAdmin(context.Background(), "u1"); !errors.Is(err, domain.ErrPromocionInvalida) {
		t.Fatalf("esperaba ErrPromocionInvalida, hubo: %v", err)
	}
}

func TestPromoverAAdmin_UsuarioInexistente(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	if err := svc.PromoverAAdmin(context.Background(), "no-existe"); !errors.Is(err, ErrUsuarioNoEncontrado) {
		t.Fatalf("esperaba ErrUsuarioNoEncontrado, hubo: %v", err)
	}
}

// Promover solo agrega Admins, nunca los saca, así que no puede dejar al
// sistema sin ninguno (RF-01.8). Este test fija que no se le agregue por
// error el guard del último Admin, que bloquearía promociones legítimas
// cuando hay un solo Admin — justamente el caso en que más se necesita.
func TestPromoverAAdmin_ConUnSoloAdminEnElSistema_Funciona(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["admin"] = &domain.Usuario{
		ID: "admin", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada,
	}
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	svc := nuevoServicioDeTest(repo)

	if err := svc.PromoverAAdmin(context.Background(), "u1"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
}

// ── DegradarADocente ────────────────────────────────────────────────────

// Dos Admins: sacarle los permisos a uno deja al otro, así que procede.
func repoConDosAdmins() *fakeRepo {
	repo := nuevoFakeRepo()
	repo.usuarios["quien-pide"] = &domain.Usuario{
		ID: "quien-pide", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada,
	}
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@escuela.edu.ar", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada,
	}
	return repo
}

func TestDegradarADocente_OK(t *testing.T) {
	repo := repoConDosAdmins()
	svc := nuevoServicioDeTest(repo)

	if err := svc.DegradarADocente(context.Background(), "u1", "quien-pide"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if !repo.usuarios["u1"].EsDocente() {
		t.Error("la degradación no se persistió")
	}
}

// Espejo de TestPromoverAAdmin_NoTocaNadaMasDeLaCuenta: quien deja de
// coordinar sigue dando clase, así que conserva materias y forma de
// ingreso. Es la diferencia con darle de baja la cuenta, que era la única
// forma que había de sacar a un Admin.
func TestDegradarADocente_NoTocaNadaMasDeLaCuenta(t *testing.T) {
	repo := repoConDosAdmins()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Nombre: "Ada", Apellido: "Lovelace", Email: "ada@escuela.edu.ar",
		PasswordHash: "hash:password123", GoogleSub: "112233",
		Rol: domain.RolAdmin, Estado: domain.EstadoAprobada,
		CursoSolicitado: "5°A", MateriaSolicitada: "Programación",
	}
	svc := nuevoServicioDeTest(repo)

	if err := svc.DegradarADocente(context.Background(), "u1", "quien-pide"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	u := repo.usuarios["u1"]
	if u.Estado != domain.EstadoAprobada {
		t.Errorf("el estado no debería cambiar: %s", u.Estado)
	}
	if u.PasswordHash != "hash:password123" || u.GoogleSub != "112233" {
		t.Error("las formas de ingreso tienen que quedar intactas")
	}
	if u.CursoSolicitado != "5°A" || u.MateriaSolicitada != "Programación" {
		t.Error("no se le quitan las materias por dejar de ser Admin")
	}
}

// RF-01.8. Este es el caso que hace falta que exista el guard: sin él,
// degradar al único Admin deja al sistema sin nadie que pueda aprobar
// cuentas ni volver a promover a nadie, y sin salida.
func TestDegradarADocente_UltimoAdmin_Rechazado(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["unico"] = &domain.Usuario{
		ID: "unico", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada,
	}
	svc := nuevoServicioDeTest(repo)

	err := svc.DegradarADocente(context.Background(), "unico", "otro-admin-que-no-existe-mas")

	if !errors.Is(err, ErrUltimoAdmin) {
		t.Fatalf("esperaba ErrUltimoAdmin, hubo: %v", err)
	}
	if !repo.usuarios["unico"].EsAdmin() {
		t.Error("el rol no debería haber cambiado si se rechazó por ser el último admin")
	}
}

// Quien se degrada a sí mismo pierde en el acto la pantalla desde la que lo
// hizo, y depende de otro Admin para volver atrás.
func TestDegradarADocente_ASiMismo_Rechazado(t *testing.T) {
	repo := repoConDosAdmins()
	svc := nuevoServicioDeTest(repo)

	err := svc.DegradarADocente(context.Background(), "u1", "u1")

	if !errors.Is(err, ErrAutoDegradacion) {
		t.Fatalf("esperaba ErrAutoDegradacion, hubo: %v", err)
	}
	if !repo.usuarios["u1"].EsAdmin() {
		t.Error("el rol no debería haber cambiado")
	}
}

// Un docente no tiene permisos que quitarle: el motivo que se devuelve es
// el del dominio y no el del guard, que sería engañoso.
func TestDegradarADocente_YaEsDocente_Error(t *testing.T) {
	repo := repoConDosAdmins()
	repo.usuarios["u1"].Rol = domain.RolDocente
	svc := nuevoServicioDeTest(repo)

	err := svc.DegradarADocente(context.Background(), "u1", "quien-pide")

	if !errors.Is(err, domain.ErrDegradacionInvalida) {
		t.Fatalf("esperaba ErrDegradacionInvalida, hubo: %v", err)
	}
}

// Una cuenta en BAJA no se cuenta como Admin activo, así que el guard no
// aplica: lo que rechaza es el dominio, y el mensaje tiene que hablar del
// estado y no del último Admin.
func TestDegradarADocente_CuentaNoAprobada_ErrorDelDominio(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Rol: domain.RolAdmin, Estado: domain.EstadoBaja,
	}
	svc := nuevoServicioDeTest(repo)

	err := svc.DegradarADocente(context.Background(), "u1", "quien-pide")

	if !errors.Is(err, domain.ErrDegradacionInvalida) {
		t.Fatalf("esperaba ErrDegradacionInvalida, hubo: %v", err)
	}
}

func TestDegradarADocente_UsuarioInexistente(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	err := svc.DegradarADocente(context.Background(), "no-existe", "quien-pide")

	if !errors.Is(err, ErrUsuarioNoEncontrado) {
		t.Fatalf("esperaba ErrUsuarioNoEncontrado, hubo: %v", err)
	}
}

// ── Por qué no entra: un mensaje por estado ─────────────────────────────

// Cada estado explica su motivo: con un "cuenta no habilitada" genérico,
// quien se acaba de registrar y quien fue rechazado leen lo mismo y ninguno
// sabe si tiene que esperar o hablar con alguien.
func TestLogin_CadaEstadoExplicaSuMotivo(t *testing.T) {
	casos := []struct {
		estado   domain.Estado
		esperado error
	}{
		{domain.EstadoPendiente, ErrIngresoCuentaPendiente},
		{domain.EstadoRechazada, ErrIngresoCuentaRechazada},
		{domain.EstadoBaja, ErrIngresoCuentaEnBaja},
	}

	for _, c := range casos {
		repo := nuevoFakeRepo()
		repo.usuarios["u1"] = &domain.Usuario{
			ID: "u1", Email: "ada@escuela.edu.ar", PasswordHash: "hash:password123",
			Rol: domain.RolDocente, Estado: c.estado,
		}
		svc := nuevoServicioDeTest(repo)

		_, err := svc.Login(context.Background(), "ada@escuela.edu.ar", "password123")

		if !errors.Is(err, c.esperado) {
			t.Errorf("estado %s: esperaba %v, hubo %v", c.estado, c.esperado, err)
		}
		// El paraguas sigue matcheando: quien pregunte por la condición
		// general no necesita enumerar estados.
		if !errors.Is(err, ErrCuentaNoHabilitada) {
			t.Errorf("estado %s: tendría que seguir matcheando ErrCuentaNoHabilitada", c.estado)
		}
	}
}

// Los tres mensajes tienen que ser distintos entre sí y no el genérico: si
// alguno se olvidara, el síntoma sería un texto inútil en pantalla que
// ningún test notaría.
func TestLogin_LosTresMotivosDicenCosasDistintas(t *testing.T) {
	mensajes := map[string]bool{}
	for _, err := range []error{
		ErrIngresoCuentaPendiente, ErrIngresoCuentaRechazada, ErrIngresoCuentaEnBaja,
	} {
		if err.Error() == ErrCuentaNoHabilitada.Error() {
			t.Errorf("%v quedó con el texto genérico", err)
		}
		if mensajes[err.Error()] {
			t.Errorf("mensaje repetido: %q", err.Error())
		}
		mensajes[err.Error()] = true
	}
}

// Decir el motivo es seguro porque para llegar hasta ahí hay que haber
// puesto la contraseña correcta. Con la contraseña equivocada, una cuenta
// pendiente tiene que seguir siendo indistinguible de cualquier otra.
func TestLogin_ConPasswordIncorrecta_NoRevelaElEstadoDeLaCuenta(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@escuela.edu.ar", PasswordHash: "hash:password123",
		Rol: domain.RolDocente, Estado: domain.EstadoPendiente,
	}
	svc := nuevoServicioDeTest(repo)

	_, err := svc.Login(context.Background(), "ada@escuela.edu.ar", "la-equivocada")

	if !errors.Is(err, ErrCredencialesInvalidas) {
		t.Fatalf("esperaba ErrCredencialesInvalidas, hubo: %v", err)
	}
	if errors.Is(err, ErrCuentaNoHabilitada) {
		t.Error("con la contraseña mal no se puede filtrar en qué estado está la cuenta")
	}
}

// ── Foto de perfil ──────────────────────────────────────────────────────
//
// El fake la guarda en un mapa: lo que verifican los tests de este paquete
// son las reglas de la cuenta, no dónde vive la imagen.

func (r *fakeRepo) GuardarFoto(_ context.Context, f *domain.FotoDePerfil) error {
	if r.fotos == nil {
		r.fotos = map[string]*domain.FotoDePerfil{}
	}
	r.fotos[f.UsuarioID] = f
	return nil
}

func (r *fakeRepo) BuscarFoto(_ context.Context, usuarioID string) (*domain.FotoDePerfil, error) {
	f, ok := r.fotos[usuarioID]
	if !ok {
		return nil, domain.ErrFotoNoExiste
	}
	return f, nil
}

func (r *fakeRepo) EliminarFoto(_ context.Context, usuarioID string) error {
	delete(r.fotos, usuarioID)
	return nil
}

func (r *fakeRepo) UsuariosConFoto(_ context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		if _, ok := r.fotos[id]; ok {
			out[id] = true
		}
	}
	return out, nil
}
