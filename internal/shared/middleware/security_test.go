package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func appConHandler(mw fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Get("/x", mw, func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	return app
}

func TestSecurityHeaders_SetHeadersEsperados(t *testing.T) {
	app := appConHandler(SecurityHeaders())

	resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options: esperaba DENY, obtuve %q", got)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options: esperaba nosniff, obtuve %q", got)
	}
	if got := resp.Header.Get("Content-Security-Policy"); got != "default-src 'self'" {
		t.Fatalf("CSP inesperada: %q", got)
	}
	if got := resp.Header.Get("Strict-Transport-Security"); got == "" {
		t.Fatal("esperaba header HSTS presente")
	}
}

func TestCORS_OrigenPermitido_AgregaHeader(t *testing.T) {
	app := appConHandler(CORS("https://sgrc.tuinstitucion.edu.ar"))

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "https://sgrc.tuinstitucion.edu.ar")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://sgrc.tuinstitucion.edu.ar" {
		t.Fatalf("esperaba el origen permitido reflejado, obtuve %q", got)
	}
}

func TestCORS_OrigenNoPermitido_SinHeader(t *testing.T) {
	app := appConHandler(CORS("https://sgrc.tuinstitucion.edu.ar"))

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "https://sitio-malicioso.com")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("no debería reflejar un origen no permitido, obtuve %q", got)
	}
}

func TestRateLimit_PermiteHastaElMaximo(t *testing.T) {
	app := appConHandler(RateLimit(2, time.Minute))

	for i := 0; i < 2; i++ {
		resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
		if err != nil {
			t.Fatalf("error inesperado en request %d: %v", i, err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("request %d: esperaba 200 dentro del límite, obtuve %d", i, resp.StatusCode)
		}
	}
}

func TestRateLimit_BloqueaAlSuperarElMaximo(t *testing.T) {
	app := appConHandler(RateLimit(1, time.Minute))

	primera, _ := app.Test(httptest.NewRequest("GET", "/x", nil))
	if primera.StatusCode != fiber.StatusOK {
		t.Fatalf("primera request: esperaba 200, obtuve %d", primera.StatusCode)
	}

	segunda, _ := app.Test(httptest.NewRequest("GET", "/x", nil))
	if segunda.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("segunda request: esperaba 429, obtuve %d", segunda.StatusCode)
	}
}

// ── RateLimitPorEmail ───────────────────────────────────────────────────

// Limitar el login solo por IP falla en las dos direcciones: los docentes
// que entran desde el wifi de la escuela comparten NAT y se consumen la
// cuota entre ellos, y quien prueba contraseñas contra una cuenta puntual
// esquiva el límite cambiando de red. La cuenta atacada es lo único
// constante.
func TestRateLimitPorEmail_LimitaPorCuentaNoPorIP(t *testing.T) {
	app := fiber.New()
	app.Post("/login", RateLimitPorEmail(2, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	postear := func(email string) int {
		req := httptest.NewRequest("POST", "/login",
			strings.NewReader(`{"email":"`+email+`","password":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		return resp.StatusCode
	}

	if got := postear("victima@escuela.edu.ar"); got != fiber.StatusOK {
		t.Fatalf("primer intento: esperaba 200, obtuve %d", got)
	}
	if got := postear("victima@escuela.edu.ar"); got != fiber.StatusOK {
		t.Fatalf("segundo intento: esperaba 200, obtuve %d", got)
	}
	if got := postear("victima@escuela.edu.ar"); got != fiber.StatusTooManyRequests {
		t.Fatalf("tercer intento contra la misma cuenta: esperaba 429, obtuve %d", got)
	}

	// Otro docente desde la misma IP no debería verse afectado.
	if got := postear("otro@escuela.edu.ar"); got != fiber.StatusOK {
		t.Fatalf("otra cuenta desde la misma IP: esperaba 200, obtuve %d", got)
	}
}

// El email se normaliza: con mayúsculas o espacios sigue siendo la misma
// cuenta, y si no se normalizara alcanzaría con variar el casing para
// esquivar el límite.
func TestRateLimitPorEmail_NormalizaMayusculasYEspacios(t *testing.T) {
	app := fiber.New()
	app.Post("/login", RateLimitPorEmail(1, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	postear := func(email string) int {
		req := httptest.NewRequest("POST", "/login",
			strings.NewReader(`{"email":"`+email+`","password":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		return resp.StatusCode
	}

	if got := postear("victima@escuela.edu.ar"); got != fiber.StatusOK {
		t.Fatalf("primer intento: esperaba 200, obtuve %d", got)
	}
	if got := postear("  Victima@Escuela.Edu.Ar "); got != fiber.StatusTooManyRequests {
		t.Fatalf("misma cuenta con otro casing: esperaba 429, obtuve %d", got)
	}
}
