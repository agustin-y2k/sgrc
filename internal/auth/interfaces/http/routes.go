package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta todas las rutas de auth bajo /api/auth. Se llama
// una sola vez desde cmd/main.go, con la Autenticacion ya armada (el
// JWT_SECRET real del entorno + la verificación de cuenta vigente contra
// Postgres).
//
// Reglas de acceso (ver docs/09-seguridad-rbac.md §3):
//   - registro/login: públicos, sin JWT.
//   - me / cambiar-password: cualquier usuario autenticado.
//   - el resto (listar, cambiar estado, reset, eliminar, crear admin):
//     solo ADMIN.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	auth := app.Group("/api/auth")

	// Públicas, con rate limiting contra fuerza bruta (ver
	// docs/09-seguridad-rbac.md §4).
	//
	// Las ventanas son por minuto y no por segundo: 5 intentos por segundo
	// son 18.000 por hora, que no frena a nadie. El login lleva además un
	// límite por cuenta, porque el de IP solo no sirve — los docentes que
	// entran desde la escuela comparten NAT y se pisarían entre ellos,
	// mientras que quien prueba contraseñas contra una cuenta puntual
	// esquiva el límite de IP cambiando de red.
	auth.Post("/registro", middleware.RateLimit(5, time.Minute), h.Registrar)
	auth.Post("/login",
		middleware.RateLimit(30, time.Minute),
		middleware.RateLimitPorEmail(10, time.Minute),
		h.Login)

	// Ingreso con Google. Mismos límites que sus equivalentes con
	// contraseña, y por la misma razón: son endpoints públicos.
	//
	// No llevan el límite por email (RateLimitPorEmail) porque acá no hay
	// ninguno que leer — el email viaja firmado dentro del token, no en un
	// campo del cuerpo. Tampoco hace falta: el límite por cuenta existe
	// contra la prueba de contraseñas, y contra un token firmado por Google
	// no hay nada que probar. Quien no tiene la cuenta no obtiene el token.
	auth.Post("/google", middleware.RateLimit(30, time.Minute), h.LoginConGoogle)
	auth.Post("/google/registro", middleware.RateLimit(5, time.Minute), h.RegistrarConGoogle)

	// Recuperación de contraseña por autoservicio. Públicas: quien olvidó
	// la contraseña no puede autenticarse, ese es todo el punto.
	//
	// Los límites son los más bajos de la API, y los dos son necesarios por
	// motivos distintos:
	//
	//   - pedir el código manda un mail a una casilla ajena. Sin tope, el
	//     formulario es un botón para inundar de correo a cualquier docente
	//     —y para quemarle la reputación al Gmail de la escuela, que tiene
	//     su propio límite diario—. Va también por email además de por IP,
	//     porque si no cambiar de red basta para seguir.
	//   - restablecer es donde se prueban códigos. El tope por código (5
	//     intentos, ver domain.CodigoRecuperacion) ya frena la fuerza bruta
	//     contra UNA cuenta; el límite por IP frena probar un código fijo
	//     contra muchas cuentas distintas, que el otro no ve.
	auth.Post("/password/olvide",
		middleware.RateLimit(5, time.Minute),
		middleware.RateLimitPorEmail(3, time.Minute),
		h.OlvidePassword)
	auth.Post("/password/restablecer",
		middleware.RateLimit(10, time.Minute),
		middleware.RateLimitPorEmail(10, time.Minute),
		h.RestablecerPassword)

	// Sin rate limit: es una lectura de configuración estática que la
	// pantalla de login hace una vez al abrirse, antes de que haya nadie
	// autenticado. Limitarla rompería el login de una escuela entera detrás
	// del mismo NAT sin proteger nada.
	auth.Get("/config", h.Config)

	// Requieren estar autenticado, cualquier rol.
	//
	// Estas dos son las únicas rutas de todo el sistema que aceptan un token
	// con la contraseña temporal sin cambiar (RF-01.6): son justamente las
	// que hacen falta para salir de esa situación. El resto usa JWTAuth, que
	// devuelve 403 mientras debe_cambiar_password siga en true.
	conPasswordVencida := aut.RequeridaPermitiendoPasswordVencida()
	auth.Get("/me", conPasswordVencida, h.Me)
	auth.Post("/cambiar-password", conPasswordVencida, h.CambiarPassword)

	autenticado := aut.Requerida()

	// Solo ADMIN
	soloAdmin := middleware.RequireRol("ADMIN")
	auth.Get("/usuarios", autenticado, soloAdmin, h.ListarUsuarios)
	auth.Patch("/usuarios/:id/estado", autenticado, soloAdmin, h.CambiarEstado)
	auth.Post("/usuarios/:id/reset-password", autenticado, soloAdmin, h.ResetearPassword)
	auth.Post("/usuarios/:id/promover-a-admin", autenticado, soloAdmin, h.PromoverAAdmin)
	auth.Post("/usuarios/:id/degradar-a-docente", autenticado, soloAdmin, h.DegradarADocente)
	auth.Delete("/usuarios/:id", autenticado, soloAdmin, h.EliminarDefinitivamente)
	auth.Post("/admins", autenticado, soloAdmin, h.CrearAdmin)
}
