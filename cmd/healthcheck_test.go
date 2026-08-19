package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── /health ────────────────────────────────────────────────────────────

// El caso que importa: con la base caída, /health tiene que decirlo — un
// healthcheck que no puede fallar no es un healthcheck.
func TestHandlerHealth_SinBaseDeDatos_Responde503(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://nadie:nadie@127.0.0.1:1/nada?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("armando el pool de test: %v", err)
	}
	defer pool.Close()

	app := fiber.New()
	app.Get("/health", handlerHealth(pool, time.Now))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/health", nil), 10*1000)
	if err != nil {
		t.Fatalf("no debería fallar la request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("esperaba 503, obtuve %d", resp.StatusCode)
	}

	cuerpo, _ := io.ReadAll(resp.Body)
	var payload map[string]any
	if err := json.Unmarshal(cuerpo, &payload); err != nil {
		t.Fatalf("respuesta no es JSON: %s", cuerpo)
	}
	if payload["status"] != "degradado" {
		t.Errorf("status = %v, esperaba \"degradado\": %s", payload["status"], cuerpo)
	}
}

// ── autochequeo del contenedor ─────────────────────────────────────────

func TestEsInvocacionDeHealthcheck(t *testing.T) {
	casos := map[string]struct {
		args     []string
		esperado bool
	}{
		"sin argumentos":   {[]string{"/sgrc-app"}, false},
		"healthcheck":      {[]string{"/sgrc-app", "healthcheck"}, true},
		"otro argumento":   {[]string{"/sgrc-app", "migrate"}, false},
		"lista vacía":      {nil, false},
		"con guiones":      {[]string{"/sgrc-app", "--healthcheck"}, false},
		"en otra posición": {[]string{"/sgrc-app", "algo", "healthcheck"}, false},
	}
	for nombre, c := range casos {
		if got := esInvocacionDeHealthcheck(c.args); got != c.esperado {
			t.Errorf("%s: obtuve %v, esperaba %v", nombre, got, c.esperado)
		}
	}
}

// El autochequeo tiene que distinguir el 503 del 200: si tomara cualquier
// respuesta como buena, el HEALTHCHECK del contenedor volvería a dar por sano
// a un proceso que no llega a la base — exactamente el problema que este
// endpoint vino a resolver.
func TestEjecutarHealthcheck_SegunLaRespuesta(t *testing.T) {
	casos := map[string]struct {
		estado   int
		esperado int
	}{
		"200 sano":       {http.StatusOK, 0},
		"503 degradado":  {http.StatusServiceUnavailable, 1},
		"500 inesperado": {http.StatusInternalServerError, 1},
	}

	for nombre, c := range casos {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.estado)
		}))

		if got := ejecutarHealthcheck(puertoDe(t, srv.URL)); got != c.esperado {
			t.Errorf("%s: código de salida %d, esperaba %d", nombre, got, c.esperado)
		}
		srv.Close()
	}
}

func TestEjecutarHealthcheck_ProcesoCaido(t *testing.T) {
	// Un puerto sin nadie escuchando: es lo que ve Docker cuando el proceso
	// murió pero el contenedor sigue arriba.
	if got := ejecutarHealthcheck("1"); got != 1 {
		t.Errorf("código de salida %d, esperaba 1", got)
	}
}

func puertoDe(t *testing.T, url string) string {
	t.Helper()
	// httptest sirve en 127.0.0.1:PUERTO, que es a donde pega el autochequeo.
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == ':' {
			return url[i+1:]
		}
	}
	t.Fatalf("no pude extraer el puerto de %q", url)
	return ""
}
