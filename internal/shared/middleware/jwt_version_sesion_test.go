package middleware

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Revocación de sesiones al cambiar la contraseña (RF-01.11).
//
// El middleware ya consulta la base en cada request, pero el estado de la
// cuenta no cambia al cambiar una contraseña. Sin la versión de sesión, un
// token emitido antes del cambio seguiría sirviendo hasta expirar, y el
// caso que motiva cambiarla —"creo que alguien entró a mi cuenta"— sería
// justo el que no funciona.

// tokenConVersion firma un token válido para user-1 con la versión de
// sesión indicada, como lo haría el firmador real.
func tokenConVersion(t *testing.T, secret []byte, version int) string {
	t.Helper()
	claims := &Claims{
		UserID:        "user-1",
		Rol:           "ADMIN",
		VersionSesion: version,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	firmado, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		t.Fatalf("no se pudo firmar el token de prueba: %v", err)
	}
	return firmado
}

// cuentaEnVersion responde como la base con una cuenta habilitada que está
// en esa versión de sesión.
func cuentaEnVersion(version int) CuentaVigente {
	return func(context.Context, string) (EstadoDeCuenta, error) {
		return EstadoDeCuenta{Vigente: true, Rol: "ADMIN", VersionSesion: version}, nil
	}
}

func statusConToken(t *testing.T, aut Autenticacion, token string) int {
	t.Helper()
	app := appConAutenticacion(aut)
	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	return resp.StatusCode
}

func TestRequerida_TokenAnteriorAlCambioDePassword_401(t *testing.T) {
	// El token es criptográficamente válido y la cuenta está habilitada:
	// lo único que cambió es que alguien cambió la contraseña, y con eso la
	// versión de sesión pasó de 0 a 1.
	aut := Autenticacion{Secret: testSecret, Vigente: cuentaEnVersion(1)}

	if got := statusConToken(t, aut, tokenConVersion(t, testSecret, 0)); got != fiber.StatusUnauthorized {
		t.Fatalf("un token de antes del cambio de contraseña no debe servir: esperaba 401, obtuve %d", got)
	}
}

func TestRequerida_TokenEmitidoDespuesDelCambio_PasaBien(t *testing.T) {
	// La contracara del test anterior: el token que devuelve el propio
	// cambio de contraseña ya viene con la versión nueva. Si esto fallara,
	// cambiar la contraseña dejaría afuera también a quien la cambió.
	aut := Autenticacion{Secret: testSecret, Vigente: cuentaEnVersion(1)}

	if got := statusConToken(t, aut, tokenConVersion(t, testSecret, 1)); got != fiber.StatusOK {
		t.Fatalf("el token emitido junto con el cambio tiene que seguir sirviendo: esperaba 200, obtuve %d", got)
	}
}

func TestRequerida_ElMensajeExplicaPorQueSeCayoLaSesion(t *testing.T) {
	// Quien cambió la contraseña en otro dispositivo tiene que entender que
	// esta sesión se cayó por eso, y no por una falla del sistema.
	aut := Autenticacion{Secret: testSecret, Vigente: cuentaEnVersion(2)}
	app := appConAutenticacion(aut)

	req := httptest.NewRequest("GET", "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+tokenConVersion(t, testSecret, 1))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	cuerpo, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("no se pudo leer la respuesta: %v", err)
	}
	if !strings.Contains(string(cuerpo), "contraseña") {
		t.Errorf("el mensaje tendría que explicar que la contraseña cambió, dice: %q", cuerpo)
	}
}

func TestRequerida_TokenSinElClaimSirveMientrasLaCuentaEsteEnCero(t *testing.T) {
	// Los tokens emitidos ANTES de la migración 010 no llevan el claim, así
	// que al deserializar quedan en 0 — igual que el DEFAULT de la columna.
	// Es lo que hace que desplegar esto NO desloguee a toda la escuela.
	aut := Autenticacion{Secret: testSecret, Vigente: cuentaEnVersion(0)}

	// tokenValido es el helper viejo: firma sin VersionSesion.
	if got := statusConToken(t, aut, tokenValido(t, testSecret, time.Now().Add(time.Hour))); got != fiber.StatusOK {
		t.Fatalf("desplegar la revocación no puede invalidar los tokens que ya estaban dando vueltas: obtuve %d", got)
	}
}

func TestRequeridaPermitiendoPasswordVencida_TambienRevoca(t *testing.T) {
	// La excepción de RF-01.6 es sobre la contraseña temporal, no sobre la
	// revocación: si /me y /cambiar-password no chequearan la versión,
	// quedaría un par de rutas usables con una sesión que ya se cerró — y
	// /cambiar-password es justamente la que permitiría quedarse adentro.
	aut := Autenticacion{Secret: testSecret, Vigente: cuentaEnVersion(1)}
	app := fiber.New()
	app.Get("/me", aut.RequeridaPermitiendoPasswordVencida(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenConVersion(t, testSecret, 0))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401 también en las rutas que toleran la contraseña vencida, obtuve %d", resp.StatusCode)
	}
}
