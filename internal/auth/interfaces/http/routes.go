package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// RegisterRoutes monta todas las rutas de auth bajo /api/auth.
func RegisterRoutes(app *fiber.App, h *Handler, aut middleware.Autenticacion) {
	auth := app.Group("/api/auth")

	// Públicas, con rate limiting contra fuerza bruta (ver
	// docs/09-seguridad-rbac.md §4).
	auth.Post("/registro", middleware.RateLimit(5, time.Minute), h.Registrar)
	auth.Post("/login",
		middleware.RateLimit(30, time.Minute),
		middleware.RateLimitPorEmail(10, time.Minute),
		h.Login)

	// Ingreso con Google.
	auth.Post("/google", middleware.RateLimit(30, time.Minute), h.LoginConGoogle)
	auth.Post("/google/registro", middleware.RateLimit(5, time.Minute), h.RegistrarConGoogle)

	// Recuperación de contraseña por autoservicio.
	auth.Post("/password/olvide",
		middleware.RateLimit(5, time.Minute),
		middleware.RateLimitPorEmail(3, time.Minute),
		h.OlvidePassword)
	auth.Post("/password/restablecer",
		middleware.RateLimit(10, time.Minute),
		middleware.RateLimitPorEmail(10, time.Minute),
		h.RestablecerPassword)

	// Sin rate limit: es una lectura de configuración estática que la pantalla
	// de login hace una vez al abrirse, antes de que haya nadie autenticado.
	auth.Get("/config", h.Config)

	// Requieren estar autenticado, cualquier rol.
	conPasswordVencida := aut.RequeridaPermitiendoPasswordVencida()

	auth.Post("/cambiar-password", conPasswordVencida, h.CambiarPassword)

	autenticado := aut.Requerida()
	// Los datos propios: el nombre con el que figurás en todo el sistema.

	// Foto de perfil: la propia se sube y se borra; la de cualquiera se puede
	// ver estando autenticado (aparecen al lado del nombre en pantallas
	// compartidas).
	auth.Get("/usuarios/:id/foto", autenticado, h.VerFoto)

	// Solo ADMIN
	soloAdmin := middleware.RequireRol("ADMIN")
	auth.Get("/usuarios", autenticado, soloAdmin, h.ListarUsuarios)
	auth.Get("/usuarios/:id", autenticado, soloAdmin, h.ObtenerUsuario)
	auth.Patch("/usuarios/:id/estado", autenticado, soloAdmin, h.CambiarEstado)
	auth.Post("/usuarios/:id/reset-password", autenticado, soloAdmin, h.ResetearPassword)
	auth.Post("/usuarios/:id/promover-a-admin", autenticado, soloAdmin, h.PromoverAAdmin)
	auth.Post("/usuarios/:id/degradar-a-docente", autenticado, soloAdmin, h.DegradarADocente)
	auth.Delete("/usuarios/:id", autenticado, soloAdmin, h.EliminarDefinitivamente)
	auth.Post("/admins", autenticado, soloAdmin, h.CrearAdmin)

	// ── Lo propio ───────────────────────────────────────────────────────
	//
	// Todo lo que una persona tiene en el sistema cuelga de "mi-" (una cosa) o
	// "mis-" (una colección), y en la raíz de /api: no es parte de
	// autenticarse, es el usuario mirando lo suyo.
	//
	// Antes había OCHO formas de decir "mío" repartidas entre los módulos, y la
	// peor no era la variedad: `GET /auth/me` y `PATCH /auth/mi-perfil` eran el
	// MISMO recurso bajo dos nombres distintos. Ahora son dos verbos de
	// /api/mi-perfil, que es lo que son.
	//
	// El GET tolera la contraseña vencida y el PATCH no, a propósito: con la
	// contraseña vencida hay que poder leer el perfil —la pantalla que pide
	// cambiarla necesita saber quién sos— pero no editarlo.
	mio := app.Group("/api")
	mio.Get("/mi-perfil", conPasswordVencida, h.Me)
	mio.Patch("/mi-perfil", autenticado, h.ActualizarMisDatos)
	mio.Put("/mi-foto", autenticado, h.SubirMiFoto)
	mio.Delete("/mi-foto", autenticado, h.EliminarMiFoto)
}
