// Package metricas expone el estado interno del proceso en el formato que lee
// Prometheus.
package metricas

import (
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Registro es el conjunto de métricas de este proceso.
type Registro struct {
	registro *prometheus.Registry

	peticiones *prometheus.CounterVec
	duracion   *prometheus.HistogramVec

	barridos         *prometheus.CounterVec
	duracionBarrido  *prometheus.HistogramVec
	ultimoExitoDesde *prometheus.GaugeVec
}

// Nuevo arma el registro con las métricas del sistema más las estándar del
// runtime de Go (memoria, goroutines, GC) y del proceso (CPU, descriptores).
func Nuevo() *Registro {
	r := prometheus.NewRegistry()
	r.MustRegister(collectors.NewGoCollector())
	r.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &Registro{
		registro: r,

		peticiones: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sgrc_peticiones_http_total",
			Help: "Peticiones HTTP atendidas, por método, ruta y código de respuesta.",
		}, []string{"metodo", "ruta", "codigo"}),

		duracion: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "sgrc_peticion_http_duracion_segundos",
			Help: "Cuánto tarda cada petición HTTP, por método y ruta.",
			// Los cortes están elegidos para este sistema, no por defecto: abajo de
			// 100ms es "instantáneo" para quien lo usa, y arriba de 2s alguien ya está
			// mirando la pantalla sin entender.
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		}, []string{"metodo", "ruta"}),

		barridos: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sgrc_barrido_ejecuciones_total",
			Help: "Ejecuciones de cada barrido de fondo, por resultado (exito/error).",
		}, []string{"barrido", "resultado"}),

		duracionBarrido: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "sgrc_barrido_duracion_segundos",
			Help:    "Cuánto tarda cada barrido de fondo.",
			Buckets: []float64{0.1, 0.5, 1, 5, 15, 60, 300},
		}, []string{"barrido"}),

		ultimoExitoDesde: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sgrc_barrido_ultimo_exito_timestamp",
			Help: "Momento (epoch en segundos) en que cada barrido terminó bien por última vez.",
		}, []string{"barrido"}),
	}

	r.MustRegister(m.peticiones, m.duracion, m.barridos, m.duracionBarrido, m.ultimoExitoDesde)
	return m
}

// ObservarPool publica el estado del pool de conexiones.
func (m *Registro) ObservarPool(pool *pgxpool.Pool) {
	descripciones := map[string]struct {
		ayuda string
		valor func(*pgxpool.Stat) float64
	}{
		"sgrc_pool_conexiones_totales": {
			"Conexiones abiertas contra Postgres (en uso más ociosas).",
			func(s *pgxpool.Stat) float64 { return float64(s.TotalConns()) },
		},
		"sgrc_pool_conexiones_en_uso": {
			"Conexiones actualmente entregadas a alguna consulta.",
			func(s *pgxpool.Stat) float64 { return float64(s.AcquiredConns()) },
		},
		"sgrc_pool_conexiones_ociosas": {
			"Conexiones abiertas y libres.",
			func(s *pgxpool.Stat) float64 { return float64(s.IdleConns()) },
		},
		"sgrc_pool_esperas_total": {
			"Veces que una consulta tuvo que esperar a que se liberara una conexión.",
			func(s *pgxpool.Stat) float64 { return float64(s.EmptyAcquireCount()) },
		},
	}

	for nombre, d := range descripciones {
		valor := d.valor
		m.registro.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: nombre,
			Help: d.ayuda,
		}, func() float64 { return valor(pool.Stat()) }))
	}
}

// MiddlewareHTTP mide cada petición.
func (m *Registro) MiddlewareHTTP() fiber.Handler {
	return func(c *fiber.Ctx) error {
		inicio := time.Now()
		err := c.Next()

		ruta := c.Route().Path
		codigo := c.Response().StatusCode()

		// Cuando un handler devuelve error, la respuesta TODAVÍA NO está escrita:
		// la arma el manejador de errores de Fiber después de que este middleware
		// termina.
		if err != nil {
			codigo = fiber.StatusInternalServerError
			var errFiber *fiber.Error
			if errors.As(err, &errFiber) {
				codigo = errFiber.Code
			}
		}

		// Una petición que no matchea ninguna ruta registrada tampoco tiene patrón:
		// Fiber reporta "/".
		if ruta == "" || (ruta == "/" && c.Path() != "/") {
			ruta = "desconocida"
		}

		// utils.CopyString y no c.Method() pelado: Fiber devuelve una vista sobre
		// el buffer de la petición, que fasthttp reusa apenas termina de atenderla.
		metodo := utils.CopyString(c.Method())
		m.duracion.WithLabelValues(metodo, ruta).Observe(time.Since(inicio).Seconds())
		m.peticiones.WithLabelValues(metodo, ruta, strconv.Itoa(codigo)).Inc()

		return err
	}
}

// InicializarBarrido deja las series de un barrido creadas desde el arranque,
// antes de que haya corrido ni una vez.
func (m *Registro) InicializarBarrido(nombre string) {
	m.ultimoExitoDesde.WithLabelValues(nombre).Set(float64(time.Now().Unix()))
	// Los contadores también: sin la serie en cero, un rate() sobre un barrido
	// que todavía no falló devuelve vacío en vez de 0, y un panel vacío se lee
	// igual que "no hay datos" cuando en realidad significa "no pasó nada malo".
	m.barridos.WithLabelValues(nombre, "exito").Add(0)
	m.barridos.WithLabelValues(nombre, "error").Add(0)
}

// MedirBarrido envuelve una corrida de un barrido de fondo: cuenta el
// resultado, mide cuánto tardó y, si salió bien, deja la marca de tiempo que
// permite alertar por ausencia («hace más de N minutos que este barrido no
// termina bien»).
func (m *Registro) MedirBarrido(nombre string, corrida func() error) error {
	inicio := time.Now()
	err := corrida()
	m.duracionBarrido.WithLabelValues(nombre).Observe(time.Since(inicio).Seconds())

	resultado := "exito"
	if err != nil {
		resultado = "error"
	}
	m.barridos.WithLabelValues(nombre, resultado).Inc()

	if err == nil {
		m.ultimoExitoDesde.WithLabelValues(nombre).Set(float64(time.Now().Unix()))
	}
	return err
}

// Registro expone el registro subyacente para montarlo en el servidor HTTP.
func (m *Registro) Coleccionador() *prometheus.Registry { return m.registro }
