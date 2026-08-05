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
	auth.Delete("/usuarios/:id", autenticado, soloAdmin, h.EliminarDefinitivamente)
	auth.Post("/admins", autenticado, soloAdmin, h.CrearAdmin)
}
