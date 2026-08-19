package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

var testSecret = []byte("un-secreto-de-test-bastante-largo")

func tokenValido(t *testing.T, secret []byte, exp time.Time) string {
	t.Helper()
	claims := &Claims{
		UserID:   "user-1",
		Rol:      "ADMIN",
		Nombre:   "Ada",
		Apellido: "Lovelace",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("no se pudo firmar el token de prueba: %v", err)
	}
	return signed
}

// cuentaVigenteFalsa responde que la cuenta existe y está habilitada, con el
// rol que se le indique.
func cuentaVigenteFalsa(rol string) CuentaVigente {
	return func(context.Context, string) (EstadoDeCuenta, error) {
		return EstadoDeCuenta{Vigente: true, Rol: rol}, nil
	}
}

func autenticacionDePrueba(secret []byte) Autenticacion {
	return Autenticacion{Secret: secret, Vigente: cuentaVigenteFalsa("ADMIN")}
}

func appConProteccion(secret []byte) *fiber.App {
	return appConAutenticacion(autenticacionDePrueba(secret))
}

func appConAutenticacion(aut Autenticacion) *fiber.App {
	app := fiber.New()
	app.Get("/protegido", aut.Requerida(), func(c *fiber.Ctx) error {
		claims := ClaimsFromCtx(c)
		return c.JSON(fiber.Map{"rol": claims.Rol})
	})
	return app
}

func TestJWTAuth_TokenValido_Pasa(t *testing.T) {
	app := appConProteccion(testSecret)
	tok := tokenValido(t, testSecret, time.Now().Add(time.Hour))

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

func TestJWTAuth_SinHeaderAuthorization_401(t *testing.T) {
	app := appConProteccion(testSecret)

	req := httptest.NewRequest("GET", "/protegido", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestJWTAuth_HeaderSinPrefijoBearer_401(t *testing.T) {
	app := appConProteccion(testSecret)
	tok := tokenValido(t, testSecret, time.Now().Add(time.Hour))

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", tok) // sin "Bearer "

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestJWTAuth_BearerVacio_401(t *testing.T) {
	app := appConProteccion(testSecret)

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer ")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401 con 'Bearer ' vacío, obtuve %d", resp.StatusCode)
	}
}

func TestJWTAuth_TokenExpirado_401(t *testing.T) {
	app := appConProteccion(testSecret)
	tok := tokenValido(t, testSecret, time.Now().Add(-time.Hour)) // ya expiró

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401 con token expirado, obtuve %d", resp.StatusCode)
	}
}

func TestJWTAuth_FirmadoConOtroSecreto_401(t *testing.T) {
	app := appConProteccion(testSecret)
	tok := tokenValido(t, []byte("secreto-incorrecto-pero-largo"), time.Now().Add(time.Hour))

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401 con secreto incorrecto, obtuve %d", resp.StatusCode)
	}
}

func TestJWTAuth_TokenMalformado_401(t *testing.T) {
	app := appConProteccion(testSecret)

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer esto-no-es-un-jwt-valido")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401 con token malformado, obtuve %d", resp.StatusCode)
	}
}

func TestJWTAuth_SecretoVacioEnElServidor_500NoPanikea(t *testing.T) {
	// Configuración inválida (JWT_SECRET vacío) — no debe panickear, debe
	// responder un error claro.
	app := appConProteccion([]byte{})
	tok := tokenValido(t, testSecret, time.Now().Add(time.Hour))

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("no debería panickear con secreto vacío: %v", err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("esperaba 500 con JWT_SECRET vacío, obtuve %d", resp.StatusCode)
	}
}

func TestJWTAuth_AlgoritmoNoHMAC_Rechazado(t *testing.T) {
	// Token firmado con "none" (alg: none) — debe rechazarse explícitamente,
	// no solo "dar la casualidad" de que no matchea.
	app := appConProteccion(testSecret)

	claims := &Claims{
		UserID: "atacante",
		Rol:    "ADMIN",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("no se pudo construir el token 'none' de prueba: %v", err)
	}

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+signed)

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("un token con alg=none debe rechazarse (401), obtuve %d", resp.StatusCode)
	}
}

// ── RF-01.6: contraseña temporal sin cambiar ────────────────────────────

func tokenConPasswordVencida(t *testing.T, secret []byte) string {
	t.Helper()
	claims := &Claims{
		UserID:              "user-1",
		Rol:                 "DOCENTE",
		Nombre:              "Ada",
		Apellido:            "Lovelace",
		DebeCambiarPassword: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("no se pudo firmar el token de prueba: %v", err)
	}
	return signed
}

// Verificarlo solo en el navegador (<ProtectedRoute>) no alcanzaría: el token
// que devuelve el login con contraseña temporal serviría contra toda la API.
func TestJWTAuth_PasswordTemporalSinCambiar_403(t *testing.T) {
	app := appConProteccion(testSecret)

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+tokenConPasswordVencida(t, testSecret))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

// Las dos rutas que hacen falta justamente para salir de esa situación
// (GET /me y POST /cambiar-password) tienen que seguir aceptando el token.
func TestJWTAuthPermitiendoPasswordVencida_PasswordTemporalSinCambiar_Pasa(t *testing.T) {
	app := fiber.New()
	app.Get("/cambiar-password", autenticacionDePrueba(testSecret).RequeridaPermitiendoPasswordVencida(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/cambiar-password", nil)
	req.Header.Set("Authorization", "Bearer "+tokenConPasswordVencida(t, testSecret))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

// Pero sigue exigiendo un token válido: "permitiendo password vencida" no
// es "sin autenticar".
func TestJWTAuthPermitiendoPasswordVencida_SinToken_401(t *testing.T) {
	app := fiber.New()
	app.Get("/me", autenticacionDePrueba(testSecret).RequeridaPermitiendoPasswordVencida(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/me", nil))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestClaimsFromCtx_SinClaimsEnContexto_NoPanikea(t *testing.T) {
	app := fiber.New()
	app.Get("/sin-auth", func(c *fiber.Ctx) error {
		claims := ClaimsFromCtx(c)
		if claims != nil {
			t.Error("esperaba nil cuando no hay claims en el contexto")
		}
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/sin-auth", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("no debería panickear: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

// ── Cuenta vigente: la firma no alcanza ─────────────────────────────────
// Estos cuatro tests cubren el agujero que tenía el middleware: validaba la
// firma y la expiración del token, pero nunca preguntaba si la cuenta seguía
// habilitada.

func TestRequerida_CuentaDadaDeBaja_401(t *testing.T) {
	// El token es criptográficamente válido: se emitió cuando la cuenta
	// estaba aprobada. Lo que cambió es el estado en la base.
	aut := Autenticacion{
		Secret: testSecret,
		Vigente: func(context.Context, string) (EstadoDeCuenta, error) {
			return EstadoDeCuenta{}, nil
		},
	}
	app := appConAutenticacion(aut)

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+tokenValido(t, testSecret, time.Now().Add(time.Hour)))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("una cuenta dada de baja no debe poder usar su token viejo: esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestRequerida_RolDeLaBaseGanaSobreElDelToken(t *testing.T) {
	// El token dice ADMIN; la base dice DOCENTE. Manda la base — si no, el RBAC
	// de cada ruta decidiría con un rol congelado en el momento del login.
	aut := Autenticacion{Secret: testSecret, Vigente: cuentaVigenteFalsa("DOCENTE")}
	app := appConAutenticacion(aut)

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+tokenValido(t, testSecret, time.Now().Add(time.Hour)))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	var cuerpo struct {
		Rol string `json:"rol"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("no se pudo leer la respuesta: %v", err)
	}
	if cuerpo.Rol != "DOCENTE" {
		t.Fatalf("esperaba que ganara el rol de la base (DOCENTE), obtuve %q", cuerpo.Rol)
	}
}

func TestRequerida_ErrorAlVerificarLaCuenta_FallaCerrado(t *testing.T) {
	// Si Postgres no responde no se puede confirmar que la cuenta siga
	// habilitada. Ante la duda no se deja pasar: 503, no 200.
	aut := Autenticacion{
		Secret: testSecret,
		Vigente: func(context.Context, string) (EstadoDeCuenta, error) {
			return EstadoDeCuenta{}, errors.New("postgres no responde")
		},
	}
	app := appConAutenticacion(aut)

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+tokenValido(t, testSecret, time.Now().Add(time.Hour)))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("esperaba 503 cuando no se puede verificar la cuenta, obtuve %d", resp.StatusCode)
	}
}

func TestRequeridaPermitiendoPasswordVencida_CuentaDadaDeBaja_401(t *testing.T) {
	// La excepción de RF-01.6 es sobre la contraseña temporal, no sobre el
	// estado de la cuenta: /me y /cambiar-password tampoco deben servirle a
	// alguien dado de baja.
	aut := Autenticacion{
		Secret: testSecret,
		Vigente: func(context.Context, string) (EstadoDeCuenta, error) {
			return EstadoDeCuenta{}, nil
		},
	}
	app := fiber.New()
	app.Get("/me", aut.RequeridaPermitiendoPasswordVencida(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenConPasswordVencida(t, testSecret))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestRequerida_SinVerificadorConfigurado_500NoPanikea(t *testing.T) {
	// Wiring incompleto (Vigente nil): mismo criterio que el secreto vacío —
	// error ruidoso, nunca dejar pasar el request.
	app := appConAutenticacion(Autenticacion{Secret: testSecret})

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+tokenValido(t, testSecret, time.Now().Add(time.Hour)))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("no debería panickear sin verificador: %v", err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("esperaba 500 sin verificador de cuenta, obtuve %d", resp.StatusCode)
	}
}
