package metricas

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// El caso que este archivo cuida es sutil y ya se coló una vez: cuando un
// handler DEVUELVE un error en vez de escribir la respuesta, Fiber arma la
// respuesta después, en su manejador de errores — o sea, después de que el
// middleware de métricas terminó. Leer el código en ese momento devuelve
// 200, así que las fallas se contaban como éxitos.
//
// Es el peor modo de falla posible para una métrica: el tablero muestra
// "todo verde" justamente cuando hay que mirarlo.

func TestUnHandlerQueDevuelveErrorNoSeCuentaComo200(t *testing.T) {
	m := Nuevo()
	app := fiber.New()
	app.Use(m.MiddlewareHTTP())
	app.Get("/api/reventar", func(c *fiber.Ctx) error {
		return errors.New("se rompió algo adentro")
	})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/reventar", nil))
	if err != nil {
		t.Fatalf("la petición falló: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("Fiber debería responder 500, respondió %d", resp.StatusCode)
	}
	if n := testutil.ToFloat64(m.peticiones.WithLabelValues("GET", "/api/reventar", "500")); n != 1 {
		t.Errorf("se esperaba 1 petición contada como 500, hay %v", n)
	}
	if n := testutil.ToFloat64(m.peticiones.WithLabelValues("GET", "/api/reventar", "200")); n != 0 {
		t.Errorf("un error se contó como 200: %v", n)
	}
}

// Los errores con código propio de Fiber (403, 404, 409…) tienen que
// conservar SU código, no convertirse todos en 500.
func TestUnErrorConCodigoPropioConservaSuCodigo(t *testing.T) {
	m := Nuevo()
	app := fiber.New()
	app.Use(m.MiddlewareHTTP())
	app.Get("/api/prohibido", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusForbidden, "no es tuyo")
	})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/prohibido", nil))
	if err != nil {
		t.Fatalf("la petición falló: %v", err)
	}
	defer resp.Body.Close()

	if n := testutil.ToFloat64(m.peticiones.WithLabelValues("GET", "/api/prohibido", "403")); n != 1 {
		t.Errorf("se esperaba 1 petición contada como 403, hay %v", n)
	}
}

// Una respuesta de error escrita a mano —el estilo que usa el sistema en sus
// handlers— tampoco tiene que confundirse.
func TestUnaRespuestaDeErrorEscritaAManoSeCuentaBien(t *testing.T) {
	m := Nuevo()
	app := fiber.New()
	app.Use(m.MiddlewareHTTP())
	app.Get("/api/degradado", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "degradado"})
	})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/degradado", nil))
	if err != nil {
		t.Fatalf("la petición falló: %v", err)
	}
	defer resp.Body.Close()

	if n := testutil.ToFloat64(m.peticiones.WithLabelValues("GET", "/api/degradado", "503")); n != 1 {
		t.Errorf("se esperaba 1 petición contada como 503, hay %v", n)
	}
}
