// Package metricas expone el estado interno del proceso en el formato que
// lee Prometheus.
//
// Qué preguntas viene a contestar, que hoy no tienen respuesta:
//
//   - ¿La aplicación está lenta, o es la impresión de una persona? (duración
//     de los requests, por ruta)
//   - ¿Qué se rompe y cada cuánto? (respuestas 5xx, por ruta)
//   - ¿Los barridos de fondo están corriendo? (marca de último éxito)
//   - ¿El pool de conexiones alcanza, o hay requests esperando una libre?
//
// Es DISTINTO del aviso de vida de internal/shared/monitoreo, y no lo
// reemplaza: aquel avisa desde afuera cuando el sistema se calla, y sigue
// funcionando aunque este proceso muera. Esto de acá vive adentro del
// proceso, así que si el proceso se muere, se muere con él. Uno alerta, el
// otro explica.
//
// El endpoint /metrics NO se publica hacia internet: nginx solo proxea /api
// y /health, y cualquier otra ruta cae en la SPA (ver frontend/nginx.conf).
// Prometheus lo consulta por la red interna de Docker. Eso importa porque
// las métricas cuentan cosas del funcionamiento interno que no tienen por
// qué ser públicas.
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

// Registro es el conjunto de métricas de este proceso. Se usa uno propio en
// vez del registro global del paquete de Prometheus para que quede explícito
// qué se expone: con el global, cualquier dependencia que se agregue en el
// futuro puede sumar métricas sin que nadie lo decida.
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
// Esas dos últimas vienen gratis y son las que contestan "¿se está comiendo
// la memoria del servidor?", que en una máquina compartida con otros usos es
// una pregunta real.
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
			// Los cortes están elegidos para este sistema, no por defecto:
			// abajo de 100ms es "instantáneo" para quien lo usa, y arriba de
			// 2s alguien ya está mirando la pantalla sin entender. Los
			// buckets por defecto de la biblioteca llegan hasta 10s, que acá
			// es todo el mismo caso: inaceptable.
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

// ObservarPool publica el estado del pool de conexiones. Se lee en el
// momento de cada consulta de Prometheus y no en un ticker propio, que es
// para lo que sirve un collector de función: un gauge que alguien tiene que
// refrescar termina mostrando un valor viejo justo cuando importa.
//
// La métrica que más dice es la de esperas: si crece, hay peticiones
// haciendo cola por una conexión libre, y eso se siente como lentitud sin
// que ninguna consulta sea lenta.
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
//
// La etiqueta `ruta` es el PATRÓN de la ruta (`/api/reservation/:id`), no la
// URL que llegó. Usar la URL cruda haría una serie temporal nueva por cada
// identificador distinto —y por cada ruta inventada que pruebe un escaneo
// automático—, que es la forma clásica de que Prometheus termine consumiendo
// toda la memoria del servidor.
func (m *Registro) MiddlewareHTTP() fiber.Handler {
	return func(c *fiber.Ctx) error {
		inicio := time.Now()
		err := c.Next()

		ruta := c.Route().Path
		codigo := c.Response().StatusCode()

		// Cuando un handler devuelve error, la respuesta TODAVÍA NO está
		// escrita: la arma el manejador de errores de Fiber después de que
		// este middleware termina. Leer el código en ese momento devuelve
		// 200, así que sin esto los errores se contarían como éxitos —
		// justo la métrica por la que uno mira el tablero.
		if err != nil {
			codigo = fiber.StatusInternalServerError
			var errFiber *fiber.Error
			if errors.As(err, &errFiber) {
				codigo = errFiber.Code
			}
		}

		// Una petición que no matchea ninguna ruta registrada tampoco tiene
		// patrón: Fiber reporta "/". Contarla con la URL cruda haría una
		// serie por cada dirección inventada que pruebe un escaneo
		// automático, así que se agrupan todas juntas.
		//
		// La condición mira además la URL pedida, y no alcanza con comparar
		// el error contra fiber.ErrNotFound: para una ruta inexistente Fiber
		// devuelve un error NUEVO con el código 404, no ese centinela, así
		// que errors.Is no lo reconoce. Y un "/" solo es un patrón de verdad
		// cuando lo que se pidió fue efectivamente "/", de modo que esto
		// sigue etiquetando bien el día que se registre una ruta raíz.
		if ruta == "" || (ruta == "/" && c.Path() != "/") {
			ruta = "desconocida"
		}

		// utils.CopyString y no c.Method() pelado: Fiber devuelve una vista
		// sobre el buffer de la petición, que fasthttp reusa apenas termina
		// de atenderla. Prometheus guarda ese string como CLAVE de la serie,
		// así que cuando el buffer se reescribe, la clave ya guardada cambia
		// abajo del mapa: aparecían métodos inventados como "GETT" y dos
		// entradas distintas para la misma serie, y desde ahí /metrics
		// devolvía 500 ("was collected before with the same name and label
		// values") en vez de métricas. O sea: toda la observabilidad caída,
		// justo por la ruta que la mira.
		//
		// La otra etiqueta, `ruta`, no necesita copia: es el patrón con el
		// que se registró la ruta al arrancar, no algo que venga del buffer.
		metodo := utils.CopyString(c.Method())
		m.duracion.WithLabelValues(metodo, ruta).Observe(time.Since(inicio).Seconds())
		m.peticiones.WithLabelValues(metodo, ruta, strconv.Itoa(codigo)).Inc()

		return err
	}
}

// InicializarBarrido deja las series de un barrido creadas desde el
// arranque, antes de que haya corrido ni una vez.
//
// No es cosmético: en Prometheus la AUSENCIA de una serie no es un valor, así
// que una alerta escrita como «hace más de 15 minutos que no termina bien»
// no dispara nunca si la métrica no existe. O sea que el caso más grave —la
// goroutine que se murió al arrancar y nunca corrió— sería justamente el
// único que pasa desapercibido. Con la serie creada, el reloj empieza a
// correr desde el arranque y la alerta salta sola.
//
// La marca arranca en "ahora" y no en cero: al reiniciar el sistema se
// asume que está bien, y si el primer barrido no termina dentro del umbral,
// ahí sí salta. Con cero, cada reinicio dispararía una alerta.
func (m *Registro) InicializarBarrido(nombre string) {
	m.ultimoExitoDesde.WithLabelValues(nombre).Set(float64(time.Now().Unix()))
	// Los contadores también: sin la serie en cero, un rate() sobre un
	// barrido que todavía no falló devuelve vacío en vez de 0, y un panel
	// vacío se lee igual que "no hay datos" cuando en realidad significa
	// "no pasó nada malo".
	m.barridos.WithLabelValues(nombre, "exito").Add(0)
	m.barridos.WithLabelValues(nombre, "error").Add(0)
}

// MedirBarrido envuelve una corrida de un barrido de fondo: cuenta el
// resultado, mide cuánto tardó y, si salió bien, deja la marca de tiempo que
// permite alertar por ausencia («hace más de N minutos que este barrido no
// termina bien»).
//
// Devuelve el error tal cual para que el llamador siga tratándolo como
// siempre: medir no es manejar.
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
