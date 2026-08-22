// Punto de entrada del monolito.
package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	// tzdata embebe la base de zonas horarias en el binario.
	_ "time/tzdata"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	academicapp "github.com/ramiro/sgrc/internal/academic/application"
	academicinfra "github.com/ramiro/sgrc/internal/academic/infrastructure"
	academichttp "github.com/ramiro/sgrc/internal/academic/interfaces/http"
	authapp "github.com/ramiro/sgrc/internal/auth/application"
	authinfra "github.com/ramiro/sgrc/internal/auth/infrastructure"
	authhttp "github.com/ramiro/sgrc/internal/auth/interfaces/http"
	availabilityapp "github.com/ramiro/sgrc/internal/availability/application"
	availabilityinfra "github.com/ramiro/sgrc/internal/availability/infrastructure"
	availabilityhttp "github.com/ramiro/sgrc/internal/availability/interfaces/http"
	inventoryapp "github.com/ramiro/sgrc/internal/inventory/application"
	inventoryinfra "github.com/ramiro/sgrc/internal/inventory/infrastructure"
	inventoryhttp "github.com/ramiro/sgrc/internal/inventory/interfaces/http"
	notificationapp "github.com/ramiro/sgrc/internal/notification/application"
	notificationinfra "github.com/ramiro/sgrc/internal/notification/infrastructure"
	notificationhttp "github.com/ramiro/sgrc/internal/notification/interfaces/http"
	reportingapp "github.com/ramiro/sgrc/internal/reporting/application"
	reportinginfra "github.com/ramiro/sgrc/internal/reporting/infrastructure"
	reportinghttp "github.com/ramiro/sgrc/internal/reporting/interfaces/http"
	reservationapp "github.com/ramiro/sgrc/internal/reservation/application"
	reservationinfra "github.com/ramiro/sgrc/internal/reservation/infrastructure"
	reservationhttp "github.com/ramiro/sgrc/internal/reservation/interfaces/http"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/email"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/metricas"
	"github.com/ramiro/sgrc/internal/shared/middleware"
	"github.com/ramiro/sgrc/internal/shared/monitoreo"
	"github.com/ramiro/sgrc/internal/shared/security"
	sugerenciasapp "github.com/ramiro/sgrc/internal/sugerencias/application"
	sugerenciasinfra "github.com/ramiro/sgrc/internal/sugerencias/infrastructure"
	sugerenciashttp "github.com/ramiro/sgrc/internal/sugerencias/interfaces/http"
)

// zonaHorariaDeLaEscuela resuelve la zona en la que el sistema interpreta
// "ahora".
func proxiesConfiables() []string {
	crudo := os.Getenv("TRUSTED_PROXIES")
	if strings.TrimSpace(crudo) == "" {
		log.Print("TRUSTED_PROXIES vacío: se ignora CF-Connecting-IP y se usa la IP del socket " +
			"(el rate limiting por IP y audit_log.ip_origen van a ver el proxy, no al cliente)")
		return nil
	}
	var redes []string
	for _, p := range strings.Split(crudo, ",") {
		if p = strings.TrimSpace(p); p != "" {
			redes = append(redes, p)
		}
	}
	return redes
}

// minLongitudJWTSecret es el piso para HS256. RFC 8725 §3.5 pide que la clave
// sea al menos tan larga como la salida del hash (32 bytes para SHA-256); por
// debajo de eso el secreto es atacable por fuerza bruta offline con cualquier
// token que el servidor haya emitido.
const minLongitudJWTSecret = 32

// origenDelFrontend valida FRONTEND_ORIGIN antes de dárselo al middleware de
// CORS. Fiber valida lo mismo, pero con un panic: con el valor vacío lo
// reemplaza por "*", ve que AllowCredentials está en true, y tira "[CORS]
// Insecure setup" con un stack trace de Go; con un origen sin esquema tira
// "[CORS] Invalid origin format".
func origenDelFrontend() string {
	origen := strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN"))
	if origen == "" {
		log.Fatal("FRONTEND_ORIGIN está vacío. Es el dominio público desde el que se sirve la SPA " +
			"(ej. https://sgrc.tuinstitucion.edu.ar) y el CORS del backend lo necesita para no " +
			"quedar abierto a cualquier origen. Ponelo en el .env (ver .env.example).")
	}
	if !strings.HasPrefix(origen, "http://") && !strings.HasPrefix(origen, "https://") {
		log.Fatalf("FRONTEND_ORIGIN (%q) tiene que incluir el esquema: https://%s", origen, origen)
	}
	// Fiber compara el header Origin del navegador contra este valor tal cual, y
	// el navegador nunca manda la barra final.
	if strings.HasSuffix(origen, "/") {
		log.Fatalf("FRONTEND_ORIGIN (%q) no lleva barra al final: el navegador manda el Origin sin ella "+
			"y ningún request pasaría el chequeo. Usá %q.", origen, strings.TrimRight(origen, "/"))
	}
	return origen
}

// remitenteDeCorreo es la dirección desde la que salen los avisos, para
// publicarla en la configuración pública (GET /api/auth/config).
func remitenteDeCorreo() string {
	if strings.TrimSpace(os.Getenv("SMTP_HOST")) == "" {
		return ""
	}
	if desde := strings.TrimSpace(os.Getenv("SMTP_FROM")); desde != "" {
		return desde
	}
	return strings.TrimSpace(os.Getenv("SMTP_USER"))
}

// timeoutHealth acota el ping a Postgres.
const timeoutHealth = 2 * time.Second

// handlerHealth responde si el proceso puede hacer su trabajo, no solo si
// está vivo.
func handlerHealth(pool *pgxpool.Pool, ahora func() time.Time) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancelar := context.WithTimeout(c.UserContext(), timeoutHealth)
		defer cancelar()

		if err := pool.Ping(ctx); err != nil {
			log.Printf("health: postgres no responde: %v", err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "degradado",
				"error":  "la base de datos no responde",
				"time":   ahora(),
			})
		}
		return c.JSON(fiber.Map{"status": "ok", "time": ahora()})
	}
}

// tiempoDeApagado es lo que se le da a los requests en vuelo para terminar
// antes de cerrar.
const tiempoDeApagado = 15 * time.Second

func zonaHorariaDeLaEscuela() *time.Location {
	nombre := os.Getenv("APP_TIMEZONE")
	if nombre == "" {
		nombre = "America/Argentina/Buenos_Aires"
	}
	loc, err := time.LoadLocation(nombre)
	if err != nil {
		log.Fatalf("APP_TIMEZONE inválida (%q): %v", nombre, err)
	}
	return loc
}

// horaAvisoLicenciasPorDefecto son las 7 de la mañana: el aviso está en la
// casilla antes de que alguien llegue a la escuela, y no en mitad de la
// noche.
const horaAvisoLicenciasPorDefecto = 7

// horaAvisoLicencias es la hora (0-23, hora de la escuela) a partir de la
// cual el job de licencias puede mandar el aviso del día.
func horaAvisoLicencias() int {
	crudo := strings.TrimSpace(os.Getenv("LICENCIAS_HORA_AVISO"))
	if crudo == "" {
		return horaAvisoLicenciasPorDefecto
	}
	hora, err := strconv.Atoi(crudo)
	if err != nil || hora < 0 || hora > 23 {
		log.Fatalf("LICENCIAS_HORA_AVISO (%q) tiene que ser un número entero de 0 a 23 "+
			"(la hora, en la zona de la escuela, a partir de la cual sale el aviso de licencias por vencer)", crudo)
	}
	return hora
}

// configDeVigilancia lee los tres plazos del barrido de reservas y entregas
// (RF-08.10 a RF-08.13).
func configDeVigilancia() reservationapp.ConfigDeVigilancia {
	cfg := reservationapp.ConfigDeVigilanciaPorDefecto()

	if v := minutosDeEntorno("RETIRO_AVISO_MINUTOS"); v > 0 {
		cfg.DemoraDelAvisoDeNoRetiro = v
	}
	if v := minutosDeEntorno("RETIRO_GRACIA_MINUTOS"); v > 0 {
		cfg.GraciaDeRetiro = v
	}
	if v := minutosDeEntorno("RETIRO_PARCIAL_GRACIA_MINUTOS"); v > 0 {
		cfg.GraciaTrasEntregaParcial = v
	}
	if v := minutosDeEntorno("DEVOLUCION_DEMORA_MINUTOS"); v > 0 {
		cfg.DemoraParaReclamar = v
	}

	// El aviso tiene que llegar ANTES de que la reserva se libere, o no es un
	// aviso: sería un correo diciéndole al docente que a los X minutos pierde
	// unas máquinas que ya perdió.
	if cfg.DemoraDelAvisoDeNoRetiro >= cfg.GraciaDeRetiro {
		log.Fatalf("RETIRO_AVISO_MINUTOS (%v) tiene que ser menor que RETIRO_GRACIA_MINUTOS (%v): "+
			"el aviso de que la reserva va a quedar libre sale antes de que quede libre, "+
			"no después", cfg.DemoraDelAvisoDeNoRetiro, cfg.GraciaDeRetiro)
	}
	if crudo := strings.TrimSpace(os.Getenv("CIERRE_JORNADA")); crudo != "" {
		hora, err := strconv.Atoi(crudo)
		if err != nil || hora < 0 || hora > 23 {
			log.Fatalf("CIERRE_JORNADA (%q) tiene que ser un número entero de 0 a 23 "+
				"(la hora, en la zona de la escuela, a partir de la cual se avisa qué "+
				"computadoras quedaron afuera)", crudo)
		}
		cfg.HoraDeCierre = hora
	}
	return cfg
}

// minutosDeEntorno devuelve 0 si la variable no está.
func minutosDeEntorno(clave string) time.Duration {
	crudo := strings.TrimSpace(os.Getenv(clave))
	if crudo == "" {
		return 0
	}
	n, err := strconv.Atoi(crudo)
	if err != nil || n <= 0 {
		log.Fatalf("%s (%q) tiene que ser un número entero de minutos mayor que cero", clave, crudo)
	}
	return time.Duration(n) * time.Minute
}

// puertoHTTP es el puerto en el que escucha la API. Lo comparten el servidor
// y el autochequeo del contenedor, que necesita saber a dónde pegar (ver
// healthcheck.go).
func puertoHTTP() string {
	if p := os.Getenv("APP_PORT"); p != "" {
		return p
	}
	return "8080"
}

func main() {
	// Modo autochequeo del contenedor: no arranca nada, solo consulta el /health
	// del proceso que ya está corriendo y sale con 0 o 1. Va primero porque no
	// necesita ni zona horaria ni base de datos.
	if esInvocacionDeHealthcheck(os.Args) {
		os.Exit(ejecutarHealthcheck(puertoHTTP()))
	}

	// Operación del esquema a mano: `sgrc-app migrate status` para ver en qué
	// versión está la base. Tampoco arranca la aplicación.
	if esInvocacionDeMigrate(os.Args) {
		os.Exit(ejecutarMigrate(os.Args, buildDSN()))
	}

	// El contexto se cancela con SIGTERM (lo que manda `docker compose down` /
	// un redeploy) o Ctrl-C. De él cuelgan el job de vencimiento y el apagado
	// del servidor.
	ctx, detenerSeñales := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer detenerSeñales()

	// ── Zona horaria de la escuela ─────────────────────────────────
	tz := zonaHorariaDeLaEscuela()
	ahora := func() time.Time { return time.Now().In(tz) }
	log.Printf("zona horaria de la escuela: %s (ahora: %s)", tz, ahora().Format(time.RFC3339))

	// El origen del frontend se valida ACÁ, antes de conectar a Postgres y de
	// sembrar nada: es config, y si está mal el proceso no va a poder atender un
	// solo request útil.
	frontendOrigin := origenDelFrontend()

	// ── Correo saliente (opcional) ───────────────────────────────── Se
	// resuelve junto al origen del frontend y por lo mismo: es config, y una
	// config de correo a medias no se detecta más tarde.
	enviadorDeEmail, err := email.DesdeEntorno(os.Getenv, ahora)
	if err != nil {
		log.Fatalf("configuración de correo inválida: %v (ver SMTP_* en .env.example)", err)
	}
	_, correoHabilitado := enviadorDeEmail.(*email.EnviadorSMTP)
	if correoHabilitado {
		log.Printf("correo saliente habilitado vía %s", os.Getenv("SMTP_HOST"))
	} else {
		log.Print("correo saliente deshabilitado: no hay SMTP_HOST configurado (ver .env.example). " +
			"La recuperación de contraseña por autoservicio queda fuera de servicio; " +
			"un Admin puede resetear contraseñas igual que antes")
	}

	// ── Aviso de vida de los barridos (ver internal/shared/monitoreo) ──
	// Opcional: sin las variables configuradas el sistema arranca igual y no
	// avisa a nadie.
	avisadorDeVida, err := monitoreo.DesdeEntorno(os.Getenv)
	if err != nil {
		log.Fatalf("configuración de monitoreo inválida: %v (ver PING_URL_* en .env.example)", err)
	}
	if jobs := avisadorDeVida.JobsConAviso(); len(jobs) > 0 {
		log.Printf("aviso de vida configurado para: %s", strings.Join(jobs, ", "))
	} else {
		log.Print("aviso de vida de los barridos deshabilitado: no hay PING_URL_* configuradas (ver .env.example). " +
			"Si una goroutine de fondo muere, el sistema sigue respondiendo y nada lo avisa")
	}

	// ── Métricas del proceso (ver internal/shared/metricas) ──────── Siempre
	// activas: recolectar cuesta microsegundos y no sirven de nada el día que
	// hacen falta si hubo que acordarse de encenderlas antes.
	metricasDelProceso := metricas.Nuevo()

	// ── Base de datos ──────────────────────────────────────────────
	dsn := buildDSN()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("no se pudo conectar a postgres: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("postgres no responde: %v", err)
	}
	log.Println("conectado a sgrc_db")

	// El estado del pool se lee en cada consulta de Prometheus, así que hay
	// que enseñárselo recién cuando el pool existe.
	metricasDelProceso.ObservarPool(pool)

	// ── Esquema al día (ver cmd/migrate.go) ──────────────────────── Antes del
	// seed a propósito: el Admin inicial se escribe en una tabla que esta
	// llamada puede estar creando recién ahora.
	if err := aplicarMigraciones(ctx, dsn); err != nil {
		log.Fatalf("no se pudo poner la base al día: %v", err)
	}

	// ── Seed del primer Admin (RF-01.4), idempotente ────────────────
	if err := seedAdminSiHaceFalta(ctx, pool, os.Getenv); err != nil {
		log.Fatalf("no se pudo sembrar el admin inicial: %v", err)
	}

	// ── Event bus in-process (ver internal/shared/eventbus) ────────
	bus := eventbus.NewInMemoryEventBus()

	// ── Auditoría (docs/09-seguridad-rbac.md §5) ────────────────────
	auditor := audit.NewPostgresAuditor(pool)

	// ── availability ───────────────────────────────────────────── RF-07
	// (disponibilidad de los Admin, informativa) más la jornada de la
	// institución, que es normativa: dice qué días y en qué horas abre la
	// escuela, y reservation valida contra ella.
	availabilityRepo := availabilityinfra.NewPostgresRepo(pool)
	availabilityListadorAdmins := availabilityinfra.NewListadorAdminsPostgres(pool)

	// Las dos flechas entre availability y reservation se cruzan: reservation
	// pregunta si un horario entra en la jornada, y availability pregunta qué
	// reservas quedarían afuera si la jornada cambiara. Como los dos Service
	// no pueden construirse a la vez, el adaptador se crea vacío acá y se
	// completa unas líneas más abajo, apenas existe reservationSvc.
	//
	// Queda a la vista y no escondido detrás de un contenedor de dependencias
	// a propósito: es el único lugar del programa donde hay que leer con
	// cuidado el orden, y esconderlo no lo haría menos cierto.
	availabilityReservas := &availabilityReservasAdapter{}

	availabilitySvc := availabilityapp.NewService(
		availabilityRepo,
		availabilityListadorAdmins,
		availabilityReservas,
		availabilityinfra.NuevoID,
		ahora,
	)
	availabilityHandler := availabilityhttp.NewHandler(availabilitySvc, auditor)

	// ── reservation ─────────────────────────────────────────────── Se arma
	// temprano a propósito: tanto auth (cascada de DarDeBaja, RF-02.8) como
	// inventory (cascada de cambio de estado/baja de PC, RF-03.8/03.9) necesitan
	// envolver reservationSvc en un adaptador para sus respectivos puertos — Go
	// exige que la dependencia exista antes de poder referenciarla al construir
	// esos otros Service.
	reservationRepo := reservationinfra.NewPostgresRepo(pool)
	validadorMateria := reservationinfra.NewValidadorMateriaPostgres(pool)
	validadorEquipo := reservationinfra.NewValidadorEquipoPostgres(pool)
	obtenedorNombre := reservationinfra.NewObtenedorNombrePostgres(pool)

	reservationSvc := reservationapp.NewService(
		reservationRepo,
		validadorMateria,
		validadorEquipo,
		&reservationValidadorJornadaAdapter{availabilitySvc: availabilitySvc},
		obtenedorNombre,
		reservationinfra.NuevoID,
		ahora,
		bus,
	)
	// La punta que faltaba del cruce de arriba. Antes de esta línea,
	// availabilitySvc existe pero no puede preguntar por reservas; después,
	// sí. Nada se atiende en el medio: el servidor todavía no levantó.
	availabilityReservas.reservationSvc = reservationSvc

	reservationHandler := reservationhttp.NewHandler(reservationSvc, auditor)

	// ── auth ───────────────────────────────────────────────────── El secreto
	// se valida ACÁ y no en el primer request: middleware.jwtAuth tiene su
	// propio guard, pero para cuando dispara el proceso ya arrancó, el
	// HEALTHCHECK del contenedor lo da por sano —solo mira que /health conteste,
	// y /health no sabe nada del secreto— y toda la API responde 500 sin que
	// nada apunte a la causa.
	jwtSecret := []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) < minLongitudJWTSecret {
		log.Fatalf("JWT_SECRET tiene %d bytes: hacen falta al menos %d. "+
			"Generá uno con `openssl rand -base64 48` y ponelo en el .env (ver .env.example).",
			len(jwtSecret), minLongitudJWTSecret)
	}
	jwtTTL, err := time.ParseDuration(os.Getenv("JWT_ACCESS_TTL"))
	if err != nil {
		jwtTTL = time.Hour // default razonable si el .env no lo especifica
	}

	authRepo := authinfra.NewPostgresRepo(pool)
	firmador := authinfra.NewJWTFirmador(jwtSecret, jwtTTL)
	gestorMaterias := authinfra.NewGestorMateriasDocentePostgres(pool)

	// autenticacion es lo que cada RegisterRoutes usa para proteger sus rutas.
	autenticacion := middleware.Autenticacion{
		Secret:  jwtSecret,
		Vigente: authinfra.NewVerificadorCuentaVigente(pool).Vigente,
	}
	// authCanceladorReservasAdapter envuelve reservationSvc para satisfacer
	// auth/application.CanceladorReservasDeMateria — auth/ nunca importa
	// reservation directamente (ver cmd/wiring_adapters.go).
	canceladorReservas := &authCanceladorReservasAdapter{reservationSvc: reservationSvc}

	// Ingreso con Google (opcional).
	googleClientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	var verificadorGoogle authapp.VerificadorGoogle
	if googleClientID != "" {
		dominios := authinfra.DominiosPermitidos(os.Getenv("GOOGLE_DOMINIOS_PERMITIDOS"))
		verificadorGoogle = authinfra.NewVerificadorGoogle(googleClientID, dominios, ahora)
		if len(dominios) > 0 {
			log.Printf("ingreso con Google habilitado, restringido a los dominios: %s", strings.Join(dominios, ", "))
		} else {
			log.Print("ingreso con Google habilitado para cualquier cuenta de Google " +
				"(las cuentas quedan PENDIENTE hasta que un Admin las apruebe; " +
				"para limitar quién puede pedirlo, ver GOOGLE_DOMINIOS_PERMITIDOS en .env.example)")
		}
	} else {
		log.Print("ingreso con Google deshabilitado: no hay GOOGLE_CLIENT_ID configurado (ver .env.example)")
	}

	// correoHabilitado (resuelto arriba, con el resto de la config) es lo que
	// hace que los dos endpoints de recuperación respondan 503 en vez de aceptar
	// un pedido cuyo mail no va a salir nunca.
	authSvc := authapp.NewService(
		authRepo,
		bus,
		security.HashPassword,
		security.VerifyPassword,
		firmador.Firmar,
		authinfra.NuevoID,
		authinfra.GenerarPasswordTemporal,
		authinfra.GenerarCodigoRecuperacion,
		ahora,
		gestorMaterias,
		canceladorReservas,
		verificadorGoogle,
		correoHabilitado,
	)
	// La dirección del remitente sale de la misma variable que usa el enviador,
	// no de una copia: si la instalación cambia de casilla, las pantallas que la
	// nombran cambian con ella.
	authHandler := authhttp.NewHandler(authSvc, auditor, googleClientID, remitenteDeCorreo())

	// ── reporting ───────────────────────────────────────────────── Se arma
	// ANTES que academic a propósito: academic necesita envolver reportingSvc
	// (junto con reservationSvc, ya armado más arriba) en un adaptador para su
	// puerto ArchivadorHistorico — ver más abajo.
	reportingRepo := reportinginfra.NewPostgresRepo(pool)
	infoEquipo := reportinginfra.NewInfoEquipoPostgres(pool)
	infoUsuario := reportinginfra.NewInfoUsuarioPostgres(pool)

	reportingSvc := reportingapp.NewService(
		reportingRepo,
		infoEquipo,
		infoUsuario,
		reportinginfra.NuevoID,
	)
	reportingHandler := reportinghttp.NewHandler(reportingSvc)

	// ── academic ─────────────────────────────────────────────────
	academicRepo := academicinfra.NewPostgresRepo(pool)
	validadorUsuario := academicinfra.NewValidadorUsuarioPostgres(pool)
	validadorReservas := academicinfra.NewValidadorReservasPostgres(pool)
	// academicArchivadorHistoricoAdapter envuelve reportingSvc + reservationSvc
	// para satisfacer academic/application.ArchivadorHistorico — academic/ nunca
	// importa reporting ni reservation directamente (ver
	// cmd/wiring_adapters.go).
	archivadorHistorico := &academicArchivadorHistoricoAdapter{reportingSvc: reportingSvc, reservationSvc: reservationSvc}

	academicSvc := academicapp.NewService(
		academicRepo,
		validadorUsuario,
		validadorReservas,
		archivadorHistorico,
		// El MISMO adaptador que usa auth: quitar la asignación y dar de baja al
		// docente son dos caminos al mismo estado (RF-02.8), y los dos tienen que
		// cancelar las reservas de la materia que queda sin nadie.
		&authCanceladorReservasAdapter{reservationSvc: reservationSvc},
		// Nombre y correo de quien pide una materia, y de quienes ya la
		// dictan: los necesitan los avisos de un pedido (service_pedidos.go).
		academicinfra.NewDatosDeUsuarioPostgres(pool),
		academicinfra.NuevoID,
		ahora,
		bus,
	)
	academicHandler := academichttp.NewHandler(academicSvc, auditor)

	// ── inventory ─────────────────────────────────────────────────
	inventoryRepo := inventoryinfra.NewPostgresRepo(pool)
	// inventoryValidadorReservasAdapter envuelve reservationSvc para satisfacer
	// inventory/application.ValidadorReservas — inventory/ nunca importa
	// reservation directamente (ver cmd/wiring_adapters.go).
	inventoryValidadorReservas := &inventoryValidadorReservasAdapter{reservationSvc: reservationSvc}

	inventorySvc := inventoryapp.NewService(
		inventoryRepo,
		inventoryValidadorReservas,
		inventoryinfra.NuevoID,
		ahora,
	)
	inventoryHandler := inventoryhttp.NewHandler(inventorySvc, auditor)

	// El barrido de reservas y entregas (RF-08.10 a RF-08.13).
	vigilante := reservationapp.NewVigilante(reservationRepo, bus, &reservationValidadorJornadaAdapter{availabilitySvc: availabilitySvc}, ahora, configDeVigilancia())

	// El avisador de licencias es un tipo aparte del Service porque no lo
	// dispara un request sino un reloj (ver el job más abajo).
	avisadorDeLicencias := inventoryapp.NewAvisadorDeLicencias(inventoryRepo, bus, ahora)

	// ── notification ───────────────────────────────────────────── Se arma
	// DESPUÉS de auth y reservation a propósito: sus suscriptores
	// (RegisterEventHandlers) necesitan estar registrados en el bus antes de que
	// el servidor HTTP empiece a aceptar pedidos, para no perderse ningún evento
	// que auth/reservation ya venían publicando sin que nadie los escuchara.
	notificationRepo := notificationinfra.NewPostgresRepo(pool)
	listadorAdmins := notificationinfra.NewListadorAdminsPostgres(pool)
	preferenciasDeEmail := notificationinfra.NewPreferenciasEmailPostgres(pool)

	notificationSvc := notificationapp.NewService(
		notificationRepo,
		listadorAdmins,
		preferenciasDeEmail,
		notificationinfra.NuevoID,
		ahora,
	)
	// Con espera: la entrega de notificaciones es asincrónica (una goroutine por
	// evento, ver subscribers.go), así que sin este WaitGroup un apagado se
	// llevaba puestos los avisos de las cancelaciones que acababa de disparar el
	// último request atendido.
	var notificacionesPendientes sync.WaitGroup
	notificationapp.RegisterEventHandlersConEspera(bus, notificationSvc, &notificacionesPendientes)

	// Las copias por correo son suscriptores APARTE de los avisos internos,
	// aunque escuchen algunos de los mismos eventos: el aviso interno es la
	// fuente de verdad y el mail una cortesía, así que si el envío falla o
	// tarda, el aviso ya se escribió.
	mensajero := notificationapp.NewMensajero(enviadorDeEmail, listadorAdmins, preferenciasDeEmail, frontendOrigin)
	notificationapp.RegisterEmailHandlersConEspera(bus, mensajero, &notificacionesPendientes)

	notificationHandler := notificationhttp.NewHandler(notificationSvc)

	// ── Soporte: pedir ayuda, contar que algo no anda, sugerir ────── Va
	// después de notification a propósito: publica eventos que aquellos
	// suscriptores ya tienen que estar escuchando cuando llegue el primero.
	sugerenciasSvc := sugerenciasapp.NewService(
		sugerenciasinfra.NewPostgresRepo(pool),
		sugerenciasinfra.NewUsuarioPostgres(pool),
		sugerenciasinfra.NuevoID,
		ahora,
		bus,
	)
	sugerenciasHandler := sugerenciashttp.NewHandler(sugerenciasSvc)

	// ── Job de vencimiento de reservas (RF-04.9) ──────────────────── Corre
	// como goroutine desde el arranque, sin infraestructura extra (ver
	// internal/shared/eventbus/eventbus.go para el mismo criterio de "sin piezas
	// de más mientras sea un monolito").
	for _, barrido := range []string{
		monitoreo.JobReservasVencidas,
		monitoreo.JobBarridoEntregas,
		monitoreo.JobAvisoLicencias,
	} {
		metricasDelProceso.InicializarBarrido(barrido)
	}

	var jobTerminado sync.WaitGroup
	jobTerminado.Add(1)
	go func() {
		defer jobTerminado.Done()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var n int
				err := metricasDelProceso.MedirBarrido(monitoreo.JobReservasVencidas, func() error {
					var err error
					n, err = reservationSvc.FinalizarVencidas(ctx)
					return err
				})
				if err != nil {
					log.Printf("job de vencimiento de reservas: %v", err)
					continue
				}
				if n > 0 {
					log.Printf("job de vencimiento: %d reservas finalizadas", n)
				}
				avisadorDeVida.Vive(ctx, monitoreo.JobReservasVencidas)
			}
		}
	}()

	// ── Barrido de reservas y entregas (RF-08.10 a RF-08.13) ──────── Cada
	// cinco minutos, como el de vencimiento de reservas.
	jobTerminado.Add(1)
	go func() {
		defer jobTerminado.Done()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var resumen reservationapp.ResumenDelBarrido
				err := metricasDelProceso.MedirBarrido(monitoreo.JobBarridoEntregas, func() error {
					var err error
					resumen, err = vigilante.Barrer(ctx)
					return err
				})
				if err != nil {
					log.Printf("barrido de reservas y entregas: %v", err)
					continue
				}
				if resumen.HizoAlgo() {
					log.Printf("barrido: %d recordatorios, %d avisos de no retiro, %d reservas liberadas, "+
						"%d avisos de equipo faltante, %d reclamos de devolución, %d avisos de cierre",
						resumen.Recordatorios, resumen.AvisosDeNoRetiro, resumen.Liberadas,
						resumen.AvisosDeEquipoFaltante, resumen.Reclamos, resumen.AvisosDeCierre)
				}
				avisadorDeVida.Vive(ctx, monitoreo.JobBarridoEntregas)
			}
		}
	}()

	// ── Job de aviso de licencias de software (RF-03.14) ──────────── Cada
	// hora, pero solo actúa a partir de horaAvisoLicencias: un mail a las 00:05
	// se lee a la mañana igual, pero llega con cara de que algo se rompió de
	// madrugada.
	horaDeAviso := horaAvisoLicencias()
	jobTerminado.Add(1)
	go func() {
		defer jobTerminado.Done()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// El aviso de vida va ANTES de la guarda de horario, y por eso llega
				// también en las horas en que este job no hace nada.
				avisadorDeVida.Vive(ctx, monitoreo.JobAvisoLicencias)

				if ahora().Hour() < horaDeAviso {
					continue
				}
				var n int
				err := metricasDelProceso.MedirBarrido(monitoreo.JobAvisoLicencias, func() error {
					var err error
					n, err = avisadorDeLicencias.Barrer(ctx)
					return err
				})
				if err != nil {
					log.Printf("job de aviso de licencias: %v", err)
					continue
				}
				if n > 0 {
					log.Printf("job de aviso de licencias: %d licencias por vencer o vencidas", n)
				}
			}
		}
	}()

	// ── HTTP ─────────────────────────────────────────────────────
	// ProxyHeader/TrustedProxies: sin esto c.IP() devuelve la IP del salto
	// anterior —nginx— en todos los requests, porque el tráfico entra Cloudflare
	// → cloudflared → nginx → acá.
	app := fiber.New(fiber.Config{
		AppName:                 "sgrc-app",
		ProxyHeader:             "CF-Connecting-IP",
		EnableTrustedProxyCheck: true,
		TrustedProxies:          proxiesConfiables(),
	})

	// ── Seguridad de borde (RNF-04, ver docs/09-seguridad-rbac.md §4) ── CORS
	// restringido al dominio del frontend (sin wildcard) y headers de seguridad
	// en toda respuesta.
	app.Use(middleware.SecurityHeaders())
	app.Use(middleware.CORS(frontendOrigin))

	// Va después de los de seguridad y antes de las rutas, para medir todo
	// lo que efectivamente se atiende (ver internal/shared/metricas).
	app.Use(metricasDelProceso.MiddlewareHTTP())

	app.Get("/health", handlerHealth(pool, ahora))

	// /metrics no se publica hacia internet: nginx solo proxea /api y /health, y
	// el resto cae en la SPA (ver frontend/nginx.conf).
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.HandlerFor(
		metricasDelProceso.Coleccionador(),
		promhttp.HandlerOpts{},
	)))

	authhttp.RegisterRoutes(app, authHandler, autenticacion)
	academichttp.RegisterRoutes(app, academicHandler, autenticacion)
	inventoryhttp.RegisterRoutes(app, inventoryHandler, autenticacion)
	reservationhttp.RegisterRoutes(app, reservationHandler, autenticacion)
	notificationhttp.RegisterRoutes(app, notificationHandler, autenticacion)
	reportinghttp.RegisterRoutes(app, reportingHandler, autenticacion)
	availabilityhttp.RegisterRoutes(app, availabilityHandler, autenticacion)
	sugerenciashttp.RegisterRoutes(app, sugerenciasHandler, autenticacion)

	port := puertoHTTP()

	// ── Apagado ordenado ─────────────────────────────────────────── Listen
	// bloquea, así que va a su propia goroutine y main se queda esperando la
	// señal.
	erroresDelServidor := make(chan error, 1)
	go func() {
		// ErrServerClosed es la salida normal de Shutdown, no una falla.
		if err := app.Listen(":" + port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			erroresDelServidor <- err
		}
		close(erroresDelServidor)
	}()

	select {
	case err, abierto := <-erroresDelServidor:
		if abierto && err != nil {
			log.Fatalf("el servidor HTTP no pudo levantar: %v", err)
		}
	case <-ctx.Done():
		log.Printf("señal de apagado recibida, cerrando en hasta %s", tiempoDeApagado)
	}

	// El orden importa: primero dejar de aceptar y terminar lo que está en
	// vuelo, después esperar los efectos que ese trabajo dejó colgando
	// (notificaciones), y recién ahí cerrar el pool que todos usan.
	if err := app.ShutdownWithTimeout(tiempoDeApagado); err != nil {
		log.Printf("apagado del servidor HTTP: %v", err)
	}
	jobTerminado.Wait()
	notificacionesPendientes.Wait()
	log.Println("apagado ordenado completo")
}

// buildDSN arma la URL de conexión con net/url en vez de concatenar.
func buildDSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD")),
		Host:     net.JoinHostPort(os.Getenv("POSTGRES_HOST"), os.Getenv("POSTGRES_PORT")),
		Path:     "/" + os.Getenv("POSTGRES_DB"),
		RawQuery: url.Values{"sslmode": {"disable"}}.Encode(),
	}
	return u.String()
}
