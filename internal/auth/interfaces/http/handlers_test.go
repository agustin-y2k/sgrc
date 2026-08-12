package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/auth/application"
	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/authtest"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// fakeAuditor descarta toda entrada de auditoría — los tests de handlers
// verifican el comportamiento HTTP, no la escritura en audit_log (eso lo
// cubre internal/shared/audit con testcontainers).
type fakeAuditor struct{}

func (fakeAuditor) Registrar(ctx context.Context, e audit.Entrada) error { return nil }

// ── fakeRepo — el mismo patrón que internal/auth/application/service_test.go ──

type fakeRepo struct {
	usuarios map[string]*domain.Usuario
	codigos  map[string]*domain.CodigoRecuperacion
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
		return nil, application.ErrCodigoNoEncontrado
	}
	return c, nil
}

func (r *fakeRepo) GuardarCodigoRecuperacion(ctx context.Context, c *domain.CodigoRecuperacion) error {
	r.codigos[c.UsuarioID] = c
	return nil
}

func (r *fakeRepo) BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error) {
	for _, u := range r.usuarios {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, application.ErrUsuarioNoEncontrado
}

func (r *fakeRepo) BuscarPorID(ctx context.Context, id string) (*domain.Usuario, error) {
	u, ok := r.usuarios[id]
	if !ok {
		return nil, application.ErrUsuarioNoEncontrado
	}
	return u, nil
}

func (r *fakeRepo) BuscarPorGoogleSub(ctx context.Context, sub string) (*domain.Usuario, error) {
	if sub == "" {
		return nil, application.ErrUsuarioNoEncontrado
	}
	for _, u := range r.usuarios {
		if u.GoogleSub == sub {
			return u, nil
		}
	}
	return nil, application.ErrUsuarioNoEncontrado
}

func (r *fakeRepo) Listar(ctx context.Context, filtroEstado *domain.Estado, filtroRol *domain.Rol, pagina paginacion.Pagina) ([]*domain.Usuario, int, error) {
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

// Estos tests verifican el contrato HTTP (códigos, permisos, parseo), no
// la atomicidad — alcanza con ejecutar fn tal cual.
func (r *fakeRepo) EnTransaccion(ctx context.Context, fn func(application.Repo) error) error {
	return fn(r)
}

func (r *fakeRepo) Crear(ctx context.Context, u *domain.Usuario) error {
	r.usuarios[u.ID] = u
	return nil
}
func (r *fakeRepo) Guardar(ctx context.Context, u *domain.Usuario) error {
	r.usuarios[u.ID] = u
	return nil
}
func (r *fakeRepo) ContarAdminsAprobados(ctx context.Context) (int, error) {
	n := 0
	for _, u := range r.usuarios {
		if u.EsAdmin() && u.Estado == domain.EstadoAprobada {
			n++
		}
	}
	return n, nil
}
func (r *fakeRepo) Eliminar(ctx context.Context, id string) error {
	delete(r.usuarios, id)
	return nil
}

func hashFalso(password string) (string, error) { return "hash:" + password, nil }
func verifyFalso(password, hash string) (bool, error) {
	return hash == "hash:"+password, nil
}
func firmarFalso(u *domain.Usuario) (string, error) { return "token-de-" + u.ID, nil }
func temporalFalso() (string, error)                { return "temporal123", nil }
func codigoFalso() (string, error)                  { return "123456", nil }

var contadorID int

func idSecuencial() string {
	contadorID++
	return fmt.Sprintf("id-%d", contadorID)
}

// fakeGestorMaterias / fakeCanceladorReservas: los tests HTTP de este
// archivo prueban ruteo y RBAC, no el detalle de la cascada de
// DarDeBaja (eso ya está cubierto en application/service_test.go) — así
// que estos fakes son no-op, sin materias asignadas por defecto.
type fakeGestorMaterias struct{}

func (f *fakeGestorMaterias) MateriasDeDocente(ctx context.Context, usuarioID string) ([]string, error) {
	return nil, nil
}
func (f *fakeGestorMaterias) QuedaOtroDocenteActivo(ctx context.Context, materiaID, usuarioIDExcluido string) (bool, error) {
	return true, nil
}
func (f *fakeGestorMaterias) RemoverAsignacionesDeDocente(ctx context.Context, usuarioID string) error {
	return nil
}

type fakeCanceladorReservas struct{}

func (f *fakeCanceladorReservas) CancelarReservasFuturasDeMateria(ctx context.Context, materiaID, motivo string) (int, error) {
	return 0, nil
}

// nuevaAppDeTest arma la app sin ingreso con Google — que es el modo en el
// que corre un despliegue sin GOOGLE_CLIENT_ID, y el que usaban todos los
// tests que ya existían.
func nuevaAppDeTest(repo *fakeRepo) *fiber.App {
	return nuevaAppDeTestConGoogle(repo, nil, "")
}

func nuevaAppDeTestConGoogle(repo *fakeRepo, verificador application.VerificadorGoogle, clientID string) *fiber.App {
	contadorID = 0
	svc := application.NewService(
		repo,
		eventbus.NewInMemoryEventBus(),
		hashFalso,
		verifyFalso,
		firmarFalso,
		idSecuencial,
		temporalFalso,
		codigoFalso,
		func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) },
		&fakeGestorMaterias{},
		&fakeCanceladorReservas{},
		verificador,
		true, // con correo: habilita la recuperación por autoservicio
	)
	h := NewHandler(svc, fakeAuditor{}, clientID, "avisos@escuela.edu.ar")

	app := fiber.New()
	RegisterRoutes(app, h, registroDePrueba.Autenticacion(testSecret))
	return app
}

var testSecret = []byte("un-secreto-de-test-bastante-largo")

// registroDePrueba hace de tabla usuario para el middleware de
// autenticación: Token() deja registrado el rol de cada ID, y
// Autenticacion() se lo devuelve al middleware igual que lo haría la base.
var registroDePrueba = authtest.Nuevo()

// tokenPara genera un JWT válido para un usuario de prueba — reusa
// exactamente el mismo formato que produce infrastructure.JWTFirmador,
// para que estos tests ejerciten el middleware de autenticación real.
func tokenPara(id, rol string) string {
	return registroDePrueba.Token(testSecret, id, rol)
}

func jsonBody(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

// ── Registrar ───────────────────────────────────────────────────────────

func TestHTTP_Registrar_OK(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/auth/registro", jsonBody(registroRequest{
		Nombre: "Ada", Apellido: "Lovelace", Email: "ada@x.com", Password: "password123",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_Registrar_PasswordCorta_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/auth/registro", jsonBody(registroRequest{
		Nombre: "Ada", Apellido: "Lovelace", Email: "ada@x.com", Password: "corta",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_Registrar_EmailDuplicado_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["existente"] = &domain.Usuario{ID: "existente", Email: "ada@x.com", Estado: domain.EstadoAprobada}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/registro", jsonBody(registroRequest{
		Nombre: "Ada", Apellido: "Lovelace", Email: "ada@x.com", Password: "password123",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_Registrar_CuerpoMalformado_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/auth/registro", bytes.NewBufferString("{esto no es json"))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400 con JSON malformado, obtuve %d", resp.StatusCode)
	}
}

// ── Login ───────────────────────────────────────────────────────────────

func TestHTTP_Login_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Email: "ada@x.com", PasswordHash: "hash:password123", Estado: domain.EstadoAprobada}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/login", jsonBody(loginRequest{Email: "ada@x.com", Password: "password123"}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body loginResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Token == "" {
		t.Error("esperaba un token en la respuesta")
	}
}

func TestHTTP_Login_CredencialesInvalidas_401(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/auth/login", jsonBody(loginRequest{Email: "nadie@x.com", Password: "cualquiera"}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_Login_CuentaPendiente_403(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Email: "ada@x.com", PasswordHash: "hash:password123", Estado: domain.EstadoPendiente}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/login", jsonBody(loginRequest{Email: "ada@x.com", Password: "password123"}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

// ── Me ──────────────────────────────────────────────────────────────────

func TestHTTP_Me_SinToken_401(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/auth/me", nil))
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_Me_ConToken_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Nombre: "Ada", Email: "ada@x.com", Estado: domain.EstadoAprobada}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

// ── Rutas solo-Admin: RBAC real de punta a punta ───────────────────────

func TestHTTP_ListarUsuarios_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("GET", "/api/auth/usuarios", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("un DOCENTE no debería poder listar usuarios (403), obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ListarUsuarios_ComoAdmin_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Rol: domain.RolDocente, Estado: domain.EstadoPendiente}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/auth/usuarios", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body listarUsuariosResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Meta.Total != 1 {
		t.Errorf("esperaba total=1, obtuve %d", body.Meta.Total)
	}
}

// El meta de este endpoint era decorativo: se llenaba con
// {Total: len(data), Page: 1, PageSize: len(data)}, así que decía "página 1
// de 1" con cualquier cantidad de usuarios y no había LIMIT en ningún lado.
func TestHTTP_ListarUsuarios_PaginaYTotal(t *testing.T) {
	repo := nuevoFakeRepo()
	for _, id := range []string{"u1", "u2", "u3", "u4", "u5"} {
		repo.usuarios[id] = &domain.Usuario{ID: id, Rol: domain.RolDocente, Estado: domain.EstadoAprobada}
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/auth/usuarios?page=2&pageSize=2", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body listarUsuariosResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("esperaba 2 usuarios en la página, obtuve %d", len(body.Data))
	}
	if body.Meta.Total != 5 || body.Meta.Page != 2 || body.Meta.PageSize != 2 {
		t.Errorf("meta incorrecta: %+v", body.Meta)
	}
}

func TestHTTP_ListarUsuarios_PaginacionInvalida_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	for _, query := range []string{"?page=0", "?pageSize=abc", "?pageSize=100000"} {
		req := httptest.NewRequest("GET", "/api/auth/usuarios"+query, nil)
		req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

		resp, _ := app.Test(req)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("%s: esperaba 400, obtuve %d", query, resp.StatusCode)
		}
	}
}

func TestHTTP_ListarUsuarios_FiltroEstadoInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("GET", "/api/auth/usuarios?estado=NO_EXISTE", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400 con estado inválido en query, obtuve %d", resp.StatusCode)
	}
}

// ── CambiarEstado ───────────────────────────────────────────────────────

func TestHTTP_CambiarEstado_Aprobar_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Estado: domain.EstadoPendiente}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/auth/usuarios/d1/estado", jsonBody(cambiarEstadoRequest{Estado: "APROBADA"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if repo.usuarios["d1"].Estado != domain.EstadoAprobada {
		t.Errorf("el estado no se actualizó: %s", repo.usuarios["d1"].Estado)
	}
}

func TestHTTP_CambiarEstado_ValorInvalido_400(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Estado: domain.EstadoPendiente}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/auth/usuarios/d1/estado", jsonBody(cambiarEstadoRequest{Estado: "ESTADO_INVENTADO"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CambiarEstado_DesdeBaja_409(t *testing.T) {
	// Confirma de punta a punta que la terminalidad de BAJA (RF-02.9) se
	// respeta también a través de la capa HTTP, no solo en domain/.
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Estado: domain.EstadoBaja}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/auth/usuarios/d1/estado", jsonBody(cambiarEstadoRequest{Estado: "APROBADA"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409 (transición inválida desde BAJA), obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CambiarEstado_UltimoAdmin_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["a1"] = &domain.Usuario{ID: "a1", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/auth/usuarios/a1/estado", jsonBody(cambiarEstadoRequest{Estado: "BAJA"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409 (último admin), obtuve %d", resp.StatusCode)
	}
}

// ── ResetearPassword / EliminarDefinitivamente / CrearAdmin ────────────

func TestHTTP_ResetearPassword_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", PasswordHash: "hash:vieja"}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/usuarios/d1/reset-password", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body resetPasswordResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.PasswordTemporal == "" {
		t.Error("esperaba una contraseña temporal en la respuesta")
	}
}

func TestHTTP_EliminarDefinitivamente_NoEstaEnBaja_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Estado: domain.EstadoAprobada}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("DELETE", "/api/auth/usuarios/d1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_EliminarDefinitivamente_DesdeBaja_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Estado: domain.EstadoBaja}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("DELETE", "/api/auth/usuarios/d1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearAdmin_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/auth/admins", jsonBody(crearAdminRequest{
		Nombre: "Grace", Apellido: "Hopper", Email: "grace@x.com", Password: "password123",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearAdmin_ComoAdmin_OK(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/auth/admins", jsonBody(crearAdminRequest{
		Nombre: "Grace", Apellido: "Hopper", Email: "grace@x.com", Password: "password123",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}
}

// TestRutasProtegidas_TokenDeCuentaDadaDeBaja_401 es el mismo agujero que
// cubren los tests de internal/shared/middleware, pero verificado sobre las
// rutas REALES que monta RegisterRoutes: lo que garantiza que el wiring de
// este paquete no se saltee la verificación de cuenta vigente.
func TestRutasProtegidas_TokenDeCuentaDadaDeBaja_401(t *testing.T) {
	app := nuevaAppDeTest(&fakeRepo{usuarios: map[string]*domain.Usuario{}})
	tok := tokenPara("docente-baja", "DOCENTE")

	// Antes de la baja el token sirve.
	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	if resp, _ := app.Test(req); resp.StatusCode == fiber.StatusUnauthorized {
		t.Fatalf("el token debería servir antes de la baja")
	}

	registroDePrueba.DarDeBaja("docente-baja")

	// El token no cambió — sigue firmado y sin expirar. Lo que cambió es el
	// estado de la cuenta, y eso alcanza para que deje de valer.
	for _, ruta := range []string{"/api/auth/me", "/api/auth/usuarios"} {
		req := httptest.NewRequest("GET", ruta, nil)
		req.Header.Set("Authorization", "Bearer "+tok)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s: error inesperado: %v", ruta, err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("%s: una cuenta en BAJA no debe poder usar su token viejo: esperaba 401, obtuve %d",
				ruta, resp.StatusCode)
		}
	}
}

// ── Promover a Admin ────────────────────────────────────────────────────

func TestHTTP_PromoverAAdmin_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@x.com", Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/usuarios/u1/promover-a-admin", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin-1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if !repo.usuarios["u1"].EsAdmin() {
		t.Error("la promoción no se aplicó")
	}
}

// Es la ruta más sensible del panel: dar permisos de Admin. Un docente no
// puede promoverse a sí mismo ni a nadie.
func TestHTTP_PromoverAAdmin_DocenteNoPuede_403(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@x.com", Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/usuarios/u1/promover-a-admin", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
	if repo.usuarios["u1"].EsAdmin() {
		t.Error("un docente no puede promoverse solo")
	}
}

func TestHTTP_PromoverAAdmin_SinAutenticar_401(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	resp, _ := app.Test(httptest.NewRequest("POST", "/api/auth/usuarios/u1/promover-a-admin", nil))

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_PromoverAAdmin_CuentaPendiente_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@x.com", Rol: domain.RolDocente, Estado: domain.EstadoPendiente,
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/usuarios/u1/promover-a-admin", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin-1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_PromoverAAdmin_Inexistente_404(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/auth/usuarios/nadie/promover-a-admin", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin-1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d", resp.StatusCode)
	}
}

// ── Degradar a docente ──────────────────────────────────────────────────

// Dos Admins aprobados: sacarle los permisos a "u1" deja al que pide.
func repoConDosAdminsHTTP() *fakeRepo {
	repo := nuevoFakeRepo()
	repo.usuarios["admin-1"] = &domain.Usuario{
		ID: "admin-1", Email: "jefe@x.com", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada,
	}
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@x.com", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada,
	}
	return repo
}

func TestHTTP_DegradarADocente_OK(t *testing.T) {
	repo := repoConDosAdminsHTTP()
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/usuarios/u1/degradar-a-docente", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin-1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if !repo.usuarios["u1"].EsDocente() {
		t.Error("la degradación no se aplicó")
	}
}

// La inversa de la ruta más sensible del panel es igual de sensible: un
// docente no puede sacarle los permisos a ningún Admin.
func TestHTTP_DegradarADocente_DocenteNoPuede_403(t *testing.T) {
	repo := repoConDosAdminsHTTP()
	repo.usuarios["docente"] = &domain.Usuario{
		ID: "docente", Email: "d@x.com", Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/usuarios/u1/degradar-a-docente", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
	if !repo.usuarios["u1"].EsAdmin() {
		t.Error("un docente no puede degradar a un Admin")
	}
}

func TestHTTP_DegradarADocente_SinAutenticar_401(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	resp, _ := app.Test(httptest.NewRequest("POST", "/api/auth/usuarios/u1/degradar-a-docente", nil))

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

// RF-01.8 desde la ruta: con un solo Admin, la respuesta es 409 y no un 500.
func TestHTTP_DegradarADocente_UltimoAdmin_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["admin-1"] = &domain.Usuario{
		ID: "admin-1", Email: "jefe@x.com", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada,
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/usuarios/admin-1/degradar-a-docente", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("otro", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
	if !repo.usuarios["admin-1"].EsAdmin() {
		t.Error("el sistema no puede quedarse sin ningún Admin")
	}
}

// El ID del token es el que decide: aunque la ruta y el rol estén bien,
// quitarse los permisos a uno mismo se rechaza.
func TestHTTP_DegradarADocente_ASiMismo_409(t *testing.T) {
	repo := repoConDosAdminsHTTP()
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/usuarios/admin-1/degradar-a-docente", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin-1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
	if !repo.usuarios["admin-1"].EsAdmin() {
		t.Error("nadie se degrada a sí mismo")
	}
}

func TestHTTP_DegradarADocente_Inexistente_404(t *testing.T) {
	app := nuevaAppDeTest(repoConDosAdminsHTTP())

	req := httptest.NewRequest("POST", "/api/auth/usuarios/nadie/degradar-a-docente", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin-1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d", resp.StatusCode)
	}
}

// ── El motivo llega hasta la pantalla ───────────────────────────────────

// El frontend muestra el texto del backend tal cual (ver api-client.ts), así
// que el mensaje que sale de acá es literalmente el que lee la persona.
func TestHTTP_Login_ElMotivoLlegaEnElCuerpo(t *testing.T) {
	casos := []struct {
		estado     domain.Estado
		contiene   string
		noContiene string
	}{
		{domain.EstadoPendiente, "esperando la aprobación", "rechazada"},
		{domain.EstadoRechazada, "rechazada", "esperando"},
		{domain.EstadoBaja, "dada de baja", "esperando"},
	}

	for _, c := range casos {
		repo := nuevoFakeRepo()
		repo.usuarios["u1"] = &domain.Usuario{
			ID: "u1", Email: "ada@x.com", PasswordHash: "hash:password123", Estado: c.estado,
		}
		app := nuevaAppDeTest(repo)

		req := httptest.NewRequest("POST", "/api/auth/login",
			jsonBody(loginRequest{Email: "ada@x.com", Password: "password123"}))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("estado %s: esperaba 403, obtuve %d", c.estado, resp.StatusCode)
		}

		cuerpo := make([]byte, 512)
		n, _ := resp.Body.Read(cuerpo)
		texto := string(cuerpo[:n])

		if !strings.Contains(texto, c.contiene) {
			t.Errorf("estado %s: el mensaje %q no explica el motivo", c.estado, texto)
		}
		if strings.Contains(texto, c.noContiene) {
			t.Errorf("estado %s: el mensaje %q es el de otro estado", c.estado, texto)
		}
	}
}
