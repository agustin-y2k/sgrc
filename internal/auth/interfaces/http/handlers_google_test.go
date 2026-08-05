package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/auth/application"
	"github.com/ramiro/sgrc/internal/auth/domain"
)

// Estos tests verifican el contrato HTTP del ingreso con Google: qué
// código sale para cada situación. La verificación de la firma se prueba
// en infrastructure/, y las reglas de negocio en application/ — acá el
// verificador es un doble que devuelve lo que cada caso necesita.

const clientIDDePrueba = "123-abc.apps.googleusercontent.com"

type fakeVerificadorGoogle struct {
	identidad *application.IdentidadGoogle
	err       error
}

func (f *fakeVerificadorGoogle) Verificar(ctx context.Context, idToken string) (*application.IdentidadGoogle, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.identidad, nil
}

func verificadorQueDevuelve(identidad *application.IdentidadGoogle) *fakeVerificadorGoogle {
	return &fakeVerificadorGoogle{identidad: identidad}
}

func identidadDePrueba() *application.IdentidadGoogle {
	return &application.IdentidadGoogle{
		Sub:             "112233445566",
		Email:           "ada@escuela.edu.ar",
		EmailVerificado: true,
		Nombre:          "Ada",
		Apellido:        "Lovelace",
	}
}

func postJSON(t *testing.T, app *fiber.App, ruta string, cuerpo any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", ruta, jsonBody(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// app.Test devuelve un *http.Response; se envuelve para poder leer el
	// body más de una vez en los asserts sin repetir el manejo de error.
	rec := httptest.NewRecorder()
	rec.Code = resp.StatusCode
	_, _ = rec.Body.ReadFrom(resp.Body)
	return rec
}

// ── GET /api/auth/config ──────────────────────────────────────────────

// La pantalla de login lo consulta antes de dibujarse: sin client ID no
// muestra el botón de Google.
func TestHTTP_Config_SinGoogle_DevuelveIDVacio(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	resp, err := app.Test(httptest.NewRequest("GET", "/api/auth/config", nil))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body configPublicaResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if body.GoogleClientID != "" {
		t.Errorf("sin configurar tendría que venir vacío, vino %q", body.GoogleClientID)
	}
}

func TestHTTP_Config_ConGoogle_PublicaElClientID(t *testing.T) {
	app := nuevaAppDeTestConGoogle(nuevoFakeRepo(), verificadorQueDevuelve(identidadDePrueba()), clientIDDePrueba)

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/auth/config", nil))

	var body configPublicaResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if body.GoogleClientID != clientIDDePrueba {
		t.Errorf("client ID inesperado: %q", body.GoogleClientID)
	}
}

// La config es pública: se consulta antes de que haya nadie autenticado.
func TestHTTP_Config_NoRequiereAutenticacion(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/auth/config", nil))

	if resp.StatusCode == fiber.StatusUnauthorized {
		t.Fatal("la config pública no puede pedir autenticación")
	}
}

// ── POST /api/auth/google ─────────────────────────────────────────────

func TestHTTP_LoginConGoogle_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@escuela.edu.ar", GoogleSub: "112233445566",
		Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	app := nuevaAppDeTestConGoogle(repo, verificadorQueDevuelve(identidadDePrueba()), clientIDDePrueba)

	rec := postJSON(t, app, "/api/auth/google", googleLoginRequest{Credential: "un-token"})

	if rec.Code != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", rec.Code, rec.Body)
	}
	var body loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if body.Token == "" {
		t.Error("esperaba un token en la respuesta")
	}
}

// El 404 es parte del contrato, no un fallo: es lo que le dice al frontend
// que tiene que llevar a la persona a completar el registro.
func TestHTTP_LoginConGoogle_SinCuenta_404(t *testing.T) {
	app := nuevaAppDeTestConGoogle(nuevoFakeRepo(), verificadorQueDevuelve(identidadDePrueba()), clientIDDePrueba)

	rec := postJSON(t, app, "/api/auth/google", googleLoginRequest{Credential: "un-token"})

	if rec.Code != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d: %s", rec.Code, rec.Body)
	}
}

func TestHTTP_LoginConGoogle_TokenInvalido_401(t *testing.T) {
	verificador := &fakeVerificadorGoogle{err: application.ErrTokenGoogleInvalido}
	app := nuevaAppDeTestConGoogle(nuevoFakeRepo(), verificador, clientIDDePrueba)

	rec := postJSON(t, app, "/api/auth/google", googleLoginRequest{Credential: "token-falsificado"})

	if rec.Code != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d: %s", rec.Code, rec.Body)
	}
}

// El detalle de qué chequeo falló queda en el log del servidor, no en la
// respuesta.
func TestHTTP_LoginConGoogle_TokenInvalido_NoFiltraElMotivo(t *testing.T) {
	verificador := &fakeVerificadorGoogle{
		err: application.ErrTokenGoogleInvalido, // el service lo envuelve con el detalle
	}
	app := nuevaAppDeTestConGoogle(nuevoFakeRepo(), verificador, clientIDDePrueba)

	rec := postJSON(t, app, "/api/auth/google", googleLoginRequest{Credential: "x"})

	if cuerpo := rec.Body.String(); cuerpo != "el token de Google no es válido" {
		t.Errorf("mensaje inesperado: %q", cuerpo)
	}
}

func TestHTTP_LoginConGoogle_EmailNoVerificado_403(t *testing.T) {
	identidad := identidadDePrueba()
	identidad.EmailVerificado = false
	app := nuevaAppDeTestConGoogle(nuevoFakeRepo(), verificadorQueDevuelve(identidad), clientIDDePrueba)

	rec := postJSON(t, app, "/api/auth/google", googleLoginRequest{Credential: "un-token"})

	if rec.Code != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d: %s", rec.Code, rec.Body)
	}
}

func TestHTTP_LoginConGoogle_DominioNoPermitido_403(t *testing.T) {
	verificador := &fakeVerificadorGoogle{err: application.ErrDominioNoPermitido}
	app := nuevaAppDeTestConGoogle(nuevoFakeRepo(), verificador, clientIDDePrueba)

	rec := postJSON(t, app, "/api/auth/google", googleLoginRequest{Credential: "un-token"})

	if rec.Code != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d: %s", rec.Code, rec.Body)
	}
}

func TestHTTP_LoginConGoogle_CuentaPendiente_403(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@escuela.edu.ar", GoogleSub: "112233445566",
		Rol: domain.RolDocente, Estado: domain.EstadoPendiente,
	}
	app := nuevaAppDeTestConGoogle(repo, verificadorQueDevuelve(identidadDePrueba()), clientIDDePrueba)

	rec := postJSON(t, app, "/api/auth/google", googleLoginRequest{Credential: "un-token"})

	if rec.Code != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d: %s", rec.Code, rec.Body)
	}
}

// Un despliegue sin GOOGLE_CLIENT_ID responde 503, no 500: el pedido está
// bien formado, es el sistema el que no tiene esta capacidad.
func TestHTTP_LoginConGoogle_NoConfigurado_503(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	rec := postJSON(t, app, "/api/auth/google", googleLoginRequest{Credential: "un-token"})

	if rec.Code != fiber.StatusServiceUnavailable {
		t.Fatalf("esperaba 503, obtuve %d: %s", rec.Code, rec.Body)
	}
}

func TestHTTP_LoginConGoogle_CuerpoMalformado_400(t *testing.T) {
	app := nuevaAppDeTestConGoogle(nuevoFakeRepo(), verificadorQueDevuelve(identidadDePrueba()), clientIDDePrueba)

	req := httptest.NewRequest("POST", "/api/auth/google", jsonBody("{esto no es un objeto"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

// ── POST /api/auth/google/registro ────────────────────────────────────

func TestHTTP_RegistrarConGoogle_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTestConGoogle(repo, verificadorQueDevuelve(identidadDePrueba()), clientIDDePrueba)

	rec := postJSON(t, app, "/api/auth/google/registro", googleRegistroRequest{
		Credential:        "un-token",
		CursoSolicitado:   "5°A",
		MateriaSolicitada: "Programación",
	})

	if rec.Code != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d: %s", rec.Code, rec.Body)
	}

	if len(repo.usuarios) != 1 {
		t.Fatalf("esperaba una cuenta creada, hay %d", len(repo.usuarios))
	}
	for _, u := range repo.usuarios {
		if u.Estado != domain.EstadoPendiente {
			t.Errorf("la cuenta tiene que quedar PENDIENTE: %s", u.Estado)
		}
		if u.CursoSolicitado != "5°A" || u.MateriaSolicitada != "Programación" {
			t.Errorf("no se guardó lo que va a dictar: %q / %q", u.CursoSolicitado, u.MateriaSolicitada)
		}
		if u.PasswordHash != "" {
			t.Errorf("una cuenta de Google no tiene contraseña: %q", u.PasswordHash)
		}
	}
}

func TestHTTP_RegistrarConGoogle_YaExiste_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@escuela.edu.ar", GoogleSub: "112233445566",
		Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	app := nuevaAppDeTestConGoogle(repo, verificadorQueDevuelve(identidadDePrueba()), clientIDDePrueba)

	rec := postJSON(t, app, "/api/auth/google/registro", googleRegistroRequest{Credential: "un-token"})

	if rec.Code != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d: %s", rec.Code, rec.Body)
	}
}

func TestHTTP_RegistrarConGoogle_NoConfigurado_503(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	rec := postJSON(t, app, "/api/auth/google/registro", googleRegistroRequest{Credential: "un-token"})

	if rec.Code != fiber.StatusServiceUnavailable {
		t.Fatalf("esperaba 503, obtuve %d: %s", rec.Code, rec.Body)
	}
}

// ── Convivencia con el resto de auth ──────────────────────────────────

// GET /api/auth/me tiene que decir cómo entra la cuenta, para que la
// pantalla de perfil no le ofrezca "cambiar contraseña" a quien no tiene
// ninguna.
func TestHTTP_Me_InformaComoIngresaLaCuenta(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@escuela.edu.ar", GoogleSub: "112233445566",
		Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	var body usuarioResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if body.TienePassword {
		t.Error("una cuenta de Google no tiene contraseña")
	}
	if !body.VinculadaAGoogle {
		t.Error("la cuenta está vinculada a Google")
	}
}

func TestHTTP_CambiarPassword_CuentaDeGoogle_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@escuela.edu.ar", GoogleSub: "112233445566",
		Rol: domain.RolDocente, Estado: domain.EstadoAprobada,
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/auth/cambiar-password", jsonBody(cambiarPasswordRequest{
		PasswordActual: "loquesea", PasswordNueva: "password-nueva",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
}
