// Punto de entrada del monolito. Arranca un solo proceso: conecta a Postgres,
// siembra el primer Admin si hace falta, crea el event bus in-process, monta
// el router HTTP y va sumando cada paquete de internal/ a medida que se
// implementa (ver docs/06-arquitectura.md).
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

	// tzdata embebe la base de zonas horarias en el binario. Hace falta
	// porque la imagen final es FROM scratch (ver Dockerfile): sin esto no
	// existe /usr/share/zoneinfo y time.LoadLocation falla, dejando al
	// proceso en UTC — tres horas adelante de la escuela.
	_ "time/tzdata"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

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
	"github.com/ramiro/sgrc/internal/shared/middleware"
	"github.com/ramiro/sgrc/internal/shared/security"
)

// zonaHorariaDeLaEscuela resuelve la zona en la que el sistema interpreta
// "ahora". Es un dato del negocio, no del servidor: las columnas de horario
// (reserva.hora_inicio, horario_admin.hora_inicio) son TIME sin zona y
// representan la hora de pared de la escuela. Si el proceso leyera la hora
// en UTC (que es lo que hace un contenedor scratch sin configurar), el job
// de vencimiento finalizaría las reservas tres horas antes y "disponible
// ahora" (RF-07.2) mostraría el bloque equivocado.
// proxiesConfiables devuelve las redes desde las que se acepta el header
// CF-Connecting-IP. Es la subred de sgrc-net, fijada en docker-compose.yml
// justamente para poder nombrarla acá: si Docker la asignara sola, cambiaría
// entre despliegues.
//
// Se configura por entorno (TRUSTED_PROXIES, lista separada por comas) para
// no hardcodear infraestructura en el binario. Sin valor, la lista queda
// vacía y Fiber ignora el header y usa la IP del socket — degradar a "no
// confío en nadie" es el default correcto: se pierde la IP real, no se gana
// una falsificable.
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

// minLongitudJWTSecret es el piso para HS256. RFC 8725 §3.5 pide que la
// clave sea al menos tan larga como la salida del hash (32 bytes para
// SHA-256); por debajo de eso el secreto es atacable por fuerza bruta
// offline con cualquier token que el servidor haya emitido.
const minLongitudJWTSecret = 32

// origenDelFrontend valida FRONTEND_ORIGIN antes de dárselo al middleware
// de CORS.
//
// Fiber valida lo mismo, pero con un panic: con el valor vacío lo reemplaza
// por "*", ve que AllowCredentials está en true, y tira
// "[CORS] Insecure setup" con un stack trace de Go; con un origen sin
// esquema tira "[CORS] Invalid origin format". En los dos casos el proceso
// se cae al arrancar y lo único que queda en los logs del contenedor es el
// stack.
//
// JWT_SECRET ya tenía este cuidado y FRONTEND_ORIGIN no, aunque es más
// fácil de dejar mal: el .env.example lo describe en el mismo párrafo que
// VITE_API_URL, que sí va vacío a propósito en un deploy same-origin.
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
	// Fiber compara el header Origin del navegador contra este valor tal
	// cual, y el navegador nunca manda la barra final. Con ella, todos los
	// requests del frontend fallarían el chequeo de CORS sin que nada lo
	// avise en el arranque.
	if strings.HasSuffix(origen, "/") {
		log.Fatalf("FRONTEND_ORIGIN (%q) no lleva barra al final: el navegador manda el Origin sin ella "+
			"y ningún request pasaría el chequeo. Usá %q.", origen, strings.TrimRight(origen, "/"))
	}
	return origen
}

// timeoutHealth acota el ping a Postgres. Un healthcheck que se cuelga es
// peor que uno que falla: el orquestador se queda esperando en vez de
// reiniciar, y con el pool saturado el chequeo pasa a ser otra conexión más
// haciendo cola.
const timeoutHealth = 2 * time.Second

// handlerHealth responde si el proceso puede hacer su trabajo, no solo si
// está vivo.
//
// La versión anterior devolvía {"status":"ok"} sin tocar el pool: con
// Postgres apagado seguía respondiendo 200, así que el único chequeo
// automático posible daba por sano a un proceso que no podía atender ni un
// login. Un healthcheck que no puede fallar no es un healthcheck.
//
// El 503 es deliberado: es lo que hace que Docker (y cualquier proxy que
// mire el estado) trate al contenedor como no disponible en vez de mandarle
// tráfico que va a terminar en 500.
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
// antes de cerrar. Cloudflare Tunnel reintenta contra el contenedor nuevo,
// así que estirarlo más solo alarga el despliegue.
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
//
// Es una hora y no un horario exacto porque el job no necesita precisión:
// las marcas de cada licencia hacen que el aviso salga una sola vez, así
// que esto es "no antes de", no "a las".
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
// (RF-08.10 a RF-08.13). Sin variables, quedan los del dominio.
//
// Se validan en el arranque y no en cada barrida: un valor mal escrito tiene
// que impedir levantar, no descubrirse tres horas después cuando el aviso no
// salga o salga cuando no corresponde.
func configDeVigilancia() reservationapp.ConfigDeVigilancia {
	cfg := reservationapp.ConfigDeVigilanciaPorDefecto()

	if v := minutosDeEntorno("RETIRO_GRACIA_MINUTOS"); v > 0 {
		cfg.GraciaDeRetiro = v
	}
	if v := minutosDeEntorno("DEVOLUCION_DEMORA_MINUTOS"); v > 0 {
		cfg.DemoraParaReclamar = v
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

// minutosDeEntorno devuelve 0 si la variable no está. Un valor que no sea un
// entero positivo aborta el arranque: "0" o "-5" no son configuraciones, son
// errores de tipeo que dejarían el barrido haciendo cualquier cosa.
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

// puertoHTTP es el puerto en el que escucha la API. Lo comparten el
// servidor y el autochequeo del contenedor, que necesita saber a dónde
// pegar (ver healthcheck.go).
func puertoHTTP() string {
	if p := os.Getenv("APP_PORT"); p != "" {
		return p
	}
	return "8080"
}

func main() {
	// Modo autochequeo del contenedor: no arranca nada, solo consulta el
	// /health del proceso que ya está corriendo y sale con 0 o 1. Va primero
	// porque no necesita ni zona horaria ni base de datos.
	if esInvocacionDeHealthcheck(os.Args) {
		os.Exit(ejecutarHealthcheck(puertoHTTP()))
	}

	// El contexto se cancela con SIGTERM (lo que manda `docker compose down`
	// / un redeploy) o Ctrl-C. De él cuelgan el job de vencimiento y el
	// apagado del servidor.
	ctx, detenerSeñales := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer detenerSeñales()

	// ── Zona horaria de la escuela ─────────────────────────────────
	tz := zonaHorariaDeLaEscuela()
	ahora := func() time.Time { return time.Now().In(tz) }
	log.Printf("zona horaria de la escuela: %s (ahora: %s)", tz, ahora().Format(time.RFC3339))

	// El origen del frontend se valida ACÁ, antes de conectar a Postgres y
	// de sembrar nada: es config, y si está mal el proceso no va a poder
	// atender un solo request útil. Mejor fallar en el primer segundo con un
	// mensaje que diga qué corregir.
	frontendOrigin := origenDelFrontend()

	// ── Correo saliente (opcional) ─────────────────────────────────
	// Se resuelve junto al origen del frontend y por lo mismo: es config, y
	// una config de correo a medias no se detecta más tarde. Con SMTP_HOST
	// puesto y SMTP_FROM vacío el proceso arrancaría bien y cada envío
	// fallaría dentro de una goroutine cuyo error solo se loguea — el modo
	// de falla que nadie mira.
	//
	// Sin SMTP_HOST los avisos siguen llegando a la campana de
	// notificaciones y lo único fuera de servicio es la recuperación por
	// autoservicio (RF-01.10), para la que existe el rescate de RF-01.6.
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

	// ── Seed del primer Admin (RF-01.4), idempotente ────────────────
	if err := seedAdminSiHaceFalta(ctx, pool, os.Getenv); err != nil {
		log.Fatalf("no se pudo sembrar el admin inicial: %v", err)
	}

	// ── Event bus in-process (ver internal/shared/eventbus) ────────
	bus := eventbus.NewInMemoryEventBus()

	// ── Auditoría (docs/09-seguridad-rbac.md §5) ────────────────────
	auditor := audit.NewPostgresAuditor(pool)

	// ── reservation ───────────────────────────────────────────────
	// Se arma PRIMERO a propósito: tanto auth (cascada de DarDeBaja,
	// RF-02.8) como inventory (cascada de cambio de estado/baja de PC,
	// RF-03.8/03.9) necesitan envolver reservationSvc en un adaptador
	// para sus respectivos puertos — Go exige que la dependencia exista
	// antes de poder referenciarla al construir esos otros Service.
	reservationRepo := reservationinfra.NewPostgresRepo(pool)
	validadorMateria := reservationinfra.NewValidadorMateriaPostgres(pool)
	validadorPC := reservationinfra.NewValidadorEquipoPostgres(pool)
	obtenedorNombre := reservationinfra.NewObtenedorNombrePostgres(pool)

	reservationSvc := reservationapp.NewService(
		reservationRepo,
		validadorMateria,
		validadorPC,
		obtenedorNombre,
		reservationinfra.NuevoID,
		ahora,
		bus,
	)
	reservationHandler := reservationhttp.NewHandler(reservationSvc, auditor)

	// ── auth ─────────────────────────────────────────────────────
	// El secreto se valida ACÁ y no en el primer request: middleware.jwtAuth
	// tiene su propio guard, pero para cuando dispara el proceso ya arrancó,
	// el HEALTHCHECK del contenedor lo da por sano —solo mira que /health
	// conteste, y /health no sabe nada del secreto— y toda la API responde 500
	// sin que nada apunte a la causa. Con un JWT_SECRET faltante o de juguete
	// no hay nada que el sistema pueda hacer bien, así que no arranca.
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

	// autenticacion es lo que cada RegisterRoutes usa para proteger sus
	// rutas. El secreto solo prueba que el token lo emitimos nosotros; la
	// consulta de cuenta vigente es lo que hace que dar de baja (RF-02.8),
	// rechazar o eliminar (RF-01.9) una cuenta surta efecto de inmediato en
	// vez de recién cuando expire el token que esa persona ya tenía.
	autenticacion := middleware.Autenticacion{
		Secret:  jwtSecret,
		Vigente: authinfra.NewVerificadorCuentaVigente(pool).Vigente,
	}
	// authCanceladorReservasAdapter envuelve reservationSvc para
	// satisfacer auth/application.CanceladorReservasDeMateria — auth/
	// nunca importa reservation directamente (ver cmd/wiring_adapters.go).
	canceladorReservas := &authCanceladorReservasAdapter{reservationSvc: reservationSvc}

	// Ingreso con Google (opcional). Sin GOOGLE_CLIENT_ID el sistema arranca
	// igual y se entra solo con email y contraseña: el verificador queda
	// nil, los dos endpoints de Google responden 503 y el frontend ni
	// siquiera dibuja el botón (GET /api/auth/config devuelve el ID vacío).
	//
	// No se valida el formato del client ID —Google no promete uno estable—
	// pero sí se avisa por log qué modo quedó activo: si alguien lo escribe
	// mal en el .env, el síntoma sería "el botón no aparece" sin ninguna
	// pista, y ese es exactamente el tipo de cosa que cuesta horas.
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

	// correoHabilitado (resuelto arriba, con el resto de la config) es lo
	// que hace que los dos endpoints de recuperación respondan 503 en vez de
	// aceptar un pedido cuyo mail no va a salir nunca.
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
	authHandler := authhttp.NewHandler(authSvc, auditor, googleClientID)

	// ── reporting ─────────────────────────────────────────────────
	// Se arma ANTES que academic a propósito: academic necesita envolver
	// reportingSvc (junto con reservationSvc, ya armado más arriba) en un
	// adaptador para su puerto ArchivadorHistorico — ver más abajo.
	reportingRepo := reportinginfra.NewPostgresRepo(pool)
	infoPC := reportinginfra.NewInfoEquipoPostgres(pool)
	infoUsuario := reportinginfra.NewInfoUsuarioPostgres(pool)

	reportingSvc := reportingapp.NewService(
		reportingRepo,
		infoPC,
		infoUsuario,
		reportinginfra.NuevoID,
	)
	reportingHandler := reportinghttp.NewHandler(reportingSvc)

	// ── academic ─────────────────────────────────────────────────
	academicRepo := academicinfra.NewPostgresRepo(pool)
	validadorUsuario := academicinfra.NewValidadorUsuarioPostgres(pool)
	validadorReservas := academicinfra.NewValidadorReservasPostgres(pool)
	// academicArchivadorHistoricoAdapter envuelve reportingSvc +
	// reservationSvc para satisfacer academic/application.ArchivadorHistorico
	// — academic/ nunca importa reporting ni reservation directamente
	// (ver cmd/wiring_adapters.go).
	archivadorHistorico := &academicArchivadorHistoricoAdapter{reportingSvc: reportingSvc, reservationSvc: reservationSvc}

	academicSvc := academicapp.NewService(
		academicRepo,
		validadorUsuario,
		validadorReservas,
		archivadorHistorico,
		// El MISMO adaptador que usa auth: quitar la asignación y dar de baja
		// al docente son dos caminos al mismo estado (RF-02.8), y los dos
		// tienen que cancelar las reservas de la materia que queda sin nadie.
		&authCanceladorReservasAdapter{reservationSvc: reservationSvc},
		academicinfra.NuevoID,
		bus,
	)
	academicHandler := academichttp.NewHandler(academicSvc, auditor)

	// ── inventory ─────────────────────────────────────────────────
	inventoryRepo := inventoryinfra.NewPostgresRepo(pool)
	// inventoryValidadorReservasAdapter envuelve reservationSvc para
	// satisfacer inventory/application.ValidadorReservas — inventory/
	// nunca importa reservation directamente (ver cmd/wiring_adapters.go).
	inventoryValidadorReservas := &inventoryValidadorReservasAdapter{reservationSvc: reservationSvc}

	inventorySvc := inventoryapp.NewService(
		inventoryRepo,
		inventoryValidadorReservas,
		inventoryinfra.NuevoID,
		ahora,
	)
	inventoryHandler := inventoryhttp.NewHandler(inventorySvc, auditor)

	// El barrido de reservas y entregas (RF-08.10 a RF-08.13). Como el
	// avisador de licencias, es un tipo aparte del Service porque lo
	// dispara un reloj y no un request.
	vigilante := reservationapp.NewVigilante(reservationRepo, bus, ahora, configDeVigilancia())

	// El avisador de licencias es un tipo aparte del Service porque no lo
	// dispara un request sino un reloj (ver el job más abajo). Se arma acá,
	// junto al resto de inventory, pero el ticker que lo llama vive con los
	// demás jobs.
	avisadorDeLicencias := inventoryapp.NewAvisadorDeLicencias(inventoryRepo, bus, ahora)

	// ── notification ─────────────────────────────────────────────
	// Se arma DESPUÉS de auth y reservation a propósito: sus suscriptores
	// (RegisterEventHandlers) necesitan estar registrados en el bus antes
	// de que el servidor HTTP empiece a aceptar pedidos, para no perderse
	// ningún evento que auth/reservation ya venían publicando sin que
	// nadie los escuchara.
	notificationRepo := notificationinfra.NewPostgresRepo(pool)
	listadorAdmins := notificationinfra.NewListadorAdminsPostgres(pool)

	notificationSvc := notificationapp.NewService(
		notificationRepo,
		listadorAdmins,
		notificationinfra.NuevoID,
		ahora,
	)
	// Con espera: la entrega de notificaciones es asincrónica (una goroutine
	// por evento, ver subscribers.go), así que sin este WaitGroup un apagado
	// se llevaba puestos los avisos de las cancelaciones que acababa de
	// disparar el último request atendido.
	var notificacionesPendientes sync.WaitGroup
	notificationapp.RegisterEventHandlersConEspera(bus, notificationSvc, &notificacionesPendientes)

	// Las copias por correo son suscriptores APARTE de los avisos internos,
	// aunque escuchen algunos de los mismos eventos: el aviso interno es la
	// fuente de verdad y el mail una cortesía, así que si el envío falla o
	// tarda, el aviso ya se escribió.
	//
	// Se registran incluso sin SMTP: el enviador deshabilitado deja una
	// línea en el log por cada mail que no se mandó, que es lo que hace
	// falta para diagnosticar "no me llega nada".
	mensajero := notificationapp.NewMensajero(enviadorDeEmail, listadorAdmins, frontendOrigin)
	notificationapp.RegisterEmailHandlersConEspera(bus, mensajero, &notificacionesPendientes)

	notificationHandler := notificationhttp.NewHandler(notificationSvc)

	// ── availability ─────────────────────────────────────────────
	// RF-07, puramente informativo — no depende de ningún otro paquete de
	// dominio, solo de auth (vía ListadorAdmins, SQL directo sobre
	// usuario) para RF-07.2.
	availabilityRepo := availabilityinfra.NewPostgresRepo(pool)
	availabilityListadorAdmins := availabilityinfra.NewListadorAdminsPostgres(pool)

	availabilitySvc := availabilityapp.NewService(
		availabilityRepo,
		availabilityListadorAdmins,
		availabilityinfra.NuevoID,
		ahora,
	)
	availabilityHandler := availabilityhttp.NewHandler(availabilitySvc)

	// ── Job de vencimiento de reservas (RF-04.9) ────────────────────
	// Corre como goroutine desde el arranque, sin infraestructura extra
	// (ver internal/shared/eventbus/eventbus.go para el mismo criterio
	// de "sin piezas de más mientras sea un monolito"). Los errores se
	// loguean pero no detienen el ticker — un fallo puntual (ej. Postgres
	// momentáneamente no disponible) no debe tirar abajo el proceso
	// entero ni dejar de reintentar en el siguiente ciclo.
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
				n, err := reservationSvc.FinalizarVencidas(ctx)
				if err != nil {
					log.Printf("job de vencimiento de reservas: %v", err)
					continue
				}
				if n > 0 {
					log.Printf("job de vencimiento: %d reservas finalizadas", n)
				}
			}
		}
	}()

	// ── Barrido de reservas y entregas (RF-08.10 a RF-08.13) ────────
	// Cada cinco minutos, como el de vencimiento de reservas. No hace falta
	// más precisión: cada aviso deja su marca en la fila, así que la
	// frecuencia solo define cuánto puede tardar en salir, nunca cuántas
	// veces sale.
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
				resumen, err := vigilante.Barrer(ctx)
				if err != nil {
					log.Printf("barrido de reservas y entregas: %v", err)
					continue
				}
				if resumen.HizoAlgo() {
					log.Printf("barrido: %d recordatorios, %d reservas liberadas, "+
						"%d avisos de equipo faltante, %d reclamos de devolución, %d avisos de cierre",
						resumen.Recordatorios, resumen.Liberadas, resumen.AvisosDePCFaltante,
						resumen.Reclamos, resumen.AvisosDeCierre)
				}
			}
		}
	}()

	// ── Job de aviso de licencias de software (RF-03.14) ────────────
	// Cada hora, pero solo actúa a partir de horaAvisoLicencias: un mail a
	// las 00:05 se lee a la mañana igual, pero llega con cara de que algo
	// se rompió de madrugada.
	//
	// No hace falta que dispare exactamente una vez por día. La
	// idempotencia la dan las dos marcas de cada licencia (ver
	// inventory/domain/licencia.go): la primera barrida después de esa hora
	// manda el aviso y las siguientes no encuentran nada. Eso es lo que
	// permite que sea un ticker pelado y no un cron, y que un reinicio del
	// contenedor no duplique nada.
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
				if ahora().Hour() < horaDeAviso {
					continue
				}
				n, err := avisadorDeLicencias.Barrer(ctx)
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
	// anterior —nginx— en todos los requests, porque el tráfico entra
	// Cloudflare → cloudflared → nginx → acá. Eso rompía dos cosas de
	// RNF-04: el rate limiting por IP pasaba a ser un único balde para toda
	// la institución, y audit_log.ip_origen guardaba la IP de un contenedor
	// en cada fila, o sea nada útil para saber quién hizo qué.
	//
	// Se usa CF-Connecting-IP y no X-Forwarded-For porque Cloudflare la
	// sobrescribe siempre con la IP real del cliente: un atacante no la
	// puede inventar mientras el único camino hasta acá sea el túnel (por
	// eso el compose de producción ya no publica el 8080 al host).
	// X-Forwarded-For, en cambio, llega como una lista que se acumula en
	// cada salto y es más frágil de interpretar.
	app := fiber.New(fiber.Config{
		AppName:                 "sgrc-app",
		ProxyHeader:             "CF-Connecting-IP",
		EnableTrustedProxyCheck: true,
		TrustedProxies:          proxiesConfiables(),
	})

	// ── Seguridad de borde (RNF-04, ver docs/09-seguridad-rbac.md §4) ──
	// CORS restringido al dominio del frontend (sin wildcard) y headers
	// de seguridad en toda respuesta. El rate limiting de login/registro
	// se aplica puntualmente en auth/interfaces/http/routes.go.
	app.Use(middleware.SecurityHeaders())
	app.Use(middleware.CORS(frontendOrigin))

	app.Get("/health", handlerHealth(pool, ahora))

	authhttp.RegisterRoutes(app, authHandler, autenticacion)
	academichttp.RegisterRoutes(app, academicHandler, autenticacion)
	inventoryhttp.RegisterRoutes(app, inventoryHandler, autenticacion)
	reservationhttp.RegisterRoutes(app, reservationHandler, autenticacion)
	notificationhttp.RegisterRoutes(app, notificationHandler, autenticacion)
	reportinghttp.RegisterRoutes(app, reportingHandler, autenticacion)
	availabilityhttp.RegisterRoutes(app, availabilityHandler, autenticacion)

	port := puertoHTTP()

	// ── Apagado ordenado ───────────────────────────────────────────
	// Listen bloquea, así que va a su propia goroutine y main se queda
	// esperando la señal. Con un `log.Fatal(app.Listen(...))` el proceso
	// moriría de golpe en cada despliegue, cortando los requests en vuelo
	// —una reserva a medio commitear se ve como un error de red del lado del
	// docente— y descartando las notificaciones que todavía estén en sus
	// goroutines.
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
//
// La concatenación a mano rompía con cualquier contraseña que contuviera un
// carácter con significado dentro de una URL: un `@` parte el host, un `/`
// parte la base, un `#` trunca el resto. Como POSTGRES_PASSWORD es
// justamente el campo donde se espera que alguien pegue algo aleatorio y
// largo en producción, el modo de falla era "generé una contraseña fuerte y
// la app no arranca". url.UserPassword escapa el userinfo, y net.JoinHostPort
// arma bien el host aunque sea IPv6.
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
