package metricas

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCuentaPeticionesPorRutaYCodigo(t *testing.T) {
	m := Nuevo()
	app := fiber.New()
	app.Use(m.MiddlewareHTTP())
	app.Get("/api/equipos/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })

	for _, id := range []string{"1", "2", "3"} {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/equipos/"+id, nil))
		if err != nil {
			t.Fatalf("la petición falló: %v", err)
		}
		resp.Body.Close()
	}

	n := testutil.ToFloat64(m.peticiones.WithLabelValues("GET", "/api/equipos/:id", "200"))
	if n != 3 {
		t.Fatalf("se esperaban 3 peticiones contadas en el patrón de ruta, hay %v", n)
	}
}

// La etiqueta tiene que ser el patrón y no la URL: si cada identificador
// creara su propia serie, un inventario grande —o un escaneo automático—
// haría crecer la memoria de Prometheus sin techo.
func TestNoCreaUnaSeriePorCadaIdentificador(t *testing.T) {
	m := Nuevo()
	app := fiber.New()
	app.Use(m.MiddlewareHTTP())
	app.Get("/api/equipos/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })

	for _, id := range []string{"a", "b", "c", "d", "e"} {
		resp, _ := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/equipos/"+id, nil))
		if resp != nil {
			resp.Body.Close()
		}
	}

	if series := testutil.CollectAndCount(m.peticiones); series != 1 {
		t.Fatalf("cinco identificadores distintos generaron %d series; debería ser 1", series)
	}
}

// Lo mismo para las rutas que no existen: un escaneo que prueba cientos de
// URLs no debe dejar cientos de series.
func TestLasRutasDesconocidasSeAgrupan(t *testing.T) {
	m := Nuevo()
	app := fiber.New()
	app.Use(m.MiddlewareHTTP())
	app.Get("/api/equipos", func(c *fiber.Ctx) error { return c.SendString("ok") })

	for _, ruta := range []string{"/wp-admin", "/.env", "/phpmyadmin"} {
		resp, _ := app.Test(httptest.NewRequest(fiber.MethodGet, ruta, nil))
		if resp != nil {
			resp.Body.Close()
		}
	}

	if series := testutil.CollectAndCount(m.peticiones); series != 1 {
		t.Fatalf("tres rutas inexistentes generaron %d series; deberían agruparse en 1", series)
	}
	if n := testutil.ToFloat64(m.peticiones.WithLabelValues("GET", "desconocida", "404")); n != 3 {
		t.Fatalf("se esperaban 3 peticiones agrupadas como desconocidas, hay %v", n)
	}
}

func TestMedirBarridoCuentaExitoYDejaLaMarcaDeTiempo(t *testing.T) {
	m := Nuevo()

	if err := m.MedirBarrido("prueba", func() error { return nil }); err != nil {
		t.Fatalf("no debería devolver error: %v", err)
	}

	if n := testutil.ToFloat64(m.barridos.WithLabelValues("prueba", "exito")); n != 1 {
		t.Errorf("se esperaba 1 ejecución exitosa, hay %v", n)
	}
	if marca := testutil.ToFloat64(m.ultimoExitoDesde.WithLabelValues("prueba")); marca <= 0 {
		t.Error("no quedó la marca de último éxito, que es la que permite alertar por ausencia")
	}
}

// Un barrido que falla NO debe mover la marca de último éxito: esa marca es
// justamente lo que distingue "corrió y funcionó" de "corrió y se rompió".
func TestUnBarridoQueFallaNoMueveLaMarcaDeExito(t *testing.T) {
	m := Nuevo()
	fallo := errors.New("postgres no responde")

	err := m.MedirBarrido("prueba", func() error { return fallo })

	if !errors.Is(err, fallo) {
		t.Fatalf("el error tiene que llegar intacto al llamador, llegó: %v", err)
	}
	if n := testutil.ToFloat64(m.barridos.WithLabelValues("prueba", "error")); n != 1 {
		t.Errorf("se esperaba 1 ejecución con error, hay %v", n)
	}
	if marca := testutil.ToFloat64(m.ultimoExitoDesde.WithLabelValues("prueba")); marca != 0 {
		t.Error("un barrido fallido movió la marca de último éxito")
	}
}

// El formato que sirve Prometheus tiene que traer las métricas del sistema
// junto con las del runtime de Go.
func TestElRegistroExponeLasMetricasEsperadas(t *testing.T) {
	m := Nuevo()
	_ = m.MedirBarrido("prueba", func() error { return nil })

	familias, err := m.Coleccionador().Gather()
	if err != nil {
		t.Fatalf("no se pudieron recolectar las métricas: %v", err)
	}

	var nombres []string
	for _, f := range familias {
		nombres = append(nombres, f.GetName())
	}
	juntos := strings.Join(nombres, " ")

	for _, esperado := range []string{
		"sgrc_barrido_ejecuciones_total",
		"sgrc_barrido_ultimo_exito_timestamp",
		"go_goroutines",
	} {
		if !strings.Contains(juntos, esperado) {
			t.Errorf("falta la métrica %q en la salida", esperado)
		}
	}
}

// El caso más grave es el que Prometheus no ve solo: una goroutine que muere
// al arrancar y nunca corre. Si la serie no existe, una alerta por "hace más
// de N minutos que no termina bien" no dispara nunca, porque en Prometheus
// la ausencia no es un valor. Por eso las series se crean al arrancar.
func TestLasSeriesDeUnBarridoExistenAntesDeLaPrimeraCorrida(t *testing.T) {
	m := Nuevo()

	m.InicializarBarrido("reservas-vencidas")

	if marca := testutil.ToFloat64(m.ultimoExitoDesde.WithLabelValues("reservas-vencidas")); marca <= 0 {
		t.Error("la marca de último éxito no quedó inicializada: una alerta por ausencia no dispararía")
	}
	if n := testutil.ToFloat64(m.barridos.WithLabelValues("reservas-vencidas", "error")); n != 0 {
		t.Errorf("el contador de errores debería existir en 0, vale %v", n)
	}
	if series := testutil.CollectAndCount(m.barridos); series != 2 {
		t.Errorf("se esperaban las dos series (exito y error) creadas, hay %d", series)
	}
}
