package monitoreo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// entorno arma un getenv de mentira con las variables que se le pasen.
func entorno(vars map[string]string) func(string) string {
	return func(clave string) string { return vars[clave] }
}

func TestAvisaAlaURLDelJob(t *testing.T) {
	var pedidos atomic.Int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidos.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer servidor.Close()

	avisador, err := DesdeEntorno(entorno(map[string]string{
		"PING_URL_RESERVAS_VENCIDAS": servidor.URL,
	}))
	if err != nil {
		t.Fatalf("configuración válida rechazada: %v", err)
	}

	avisador.Vive(context.Background(), JobReservasVencidas)

	if n := pedidos.Load(); n != 1 {
		t.Fatalf("se esperaba 1 aviso, llegaron %d", n)
	}
}

// El caso normal de una instalación que no usa monitoreo: sin variable, el
// job no tiene que intentar nada ni fallar.
func TestSinURLNoAvisaNiFalla(t *testing.T) {
	avisador, err := DesdeEntorno(entorno(nil))
	if err != nil {
		t.Fatalf("un entorno vacío no debería ser un error: %v", err)
	}

	avisador.Vive(context.Background(), JobBarridoEntregas)

	if jobs := avisador.JobsConAviso(); len(jobs) != 0 {
		t.Fatalf("no debería haber jobs con aviso, hay %v", jobs)
	}
}

// Cada job avisa a SU URL. Un cruce acá haría que un barrido muerto pareciera
// vivo porque otro sigue andando — el fallo que este paquete existe para
// detectar.
func TestCadaJobAvisaASuPropiaURL(t *testing.T) {
	var reservas, licencias atomic.Int32
	srvReservas := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reservas.Add(1)
	}))
	defer srvReservas.Close()
	srvLicencias := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		licencias.Add(1)
	}))
	defer srvLicencias.Close()

	avisador, err := DesdeEntorno(entorno(map[string]string{
		"PING_URL_RESERVAS_VENCIDAS": srvReservas.URL,
		"PING_URL_AVISO_LICENCIAS":   srvLicencias.URL,
	}))
	if err != nil {
		t.Fatalf("configuración válida rechazada: %v", err)
	}

	avisador.Vive(context.Background(), JobAvisoLicencias)

	if reservas.Load() != 0 {
		t.Error("el aviso de licencias llegó a la URL de reservas vencidas")
	}
	if licencias.Load() != 1 {
		t.Errorf("se esperaba 1 aviso de licencias, llegaron %d", licencias.Load())
	}

	// El barrido de entregas no está configurado: no avisa y no rompe.
	avisador.Vive(context.Background(), JobBarridoEntregas)

	if jobs := avisador.JobsConAviso(); len(jobs) != 2 {
		t.Errorf("se esperaban 2 jobs con aviso, hay %v", jobs)
	}
}

// Un servicio de monitoreo caído no puede tumbar ni demorar un barrido: lo
// que importa es el trabajo, el aviso es accesorio.
func TestUnServicioQueFallaNoRompeElBarrido(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer servidor.Close()

	avisador, err := DesdeEntorno(entorno(map[string]string{
		"PING_URL_BARRIDO_ENTREGAS": servidor.URL,
	}))
	if err != nil {
		t.Fatalf("configuración válida rechazada: %v", err)
	}

	hecho := make(chan struct{})
	go func() {
		avisador.Vive(context.Background(), JobBarridoEntregas)
		close(hecho)
	}()

	select {
	case <-hecho:
	case <-time.After(5 * time.Second):
		t.Fatal("Vive se quedó colgada con un servicio que responde 500")
	}
}

// Una URL mal escrita tiene que gritar al arrancar. Si se ignorara, alguien
// configuraría el monitoreo, creería estar cubierto, y no lo estaría.
func TestURLInvalidaEsErrorDeConfiguracion(t *testing.T) {
	casos := map[string]string{
		"sin esquema":            "hc-ping.com/abc",
		"esquema que no es http": "ftp://hc-ping.com/abc",
		"sin host":               "https://",
		"vacía con espacios":     "   ",
	}

	for nombre, valor := range casos {
		t.Run(nombre, func(t *testing.T) {
			_, err := DesdeEntorno(entorno(map[string]string{"PING_URL_RESERVAS_VENCIDAS": valor}))
			if err == nil {
				t.Fatalf("se aceptó una URL inválida: %q", valor)
			}
		})
	}
}
