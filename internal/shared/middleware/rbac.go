package middleware

import "github.com/gofiber/fiber/v2"

// RequireRol bloquea el acceso si el rol del usuario autenticado no está
// en la lista permitida. Se usa DESPUÉS de Autenticacion.Requerida() en la
// cadena de middlewares de cada ruta (ver docs/09-seguridad-rbac.md §3 para
// la matriz completa de qué rol puede hacer qué).
//
// El rol que lee acá es el que Requerida() ya refrescó contra la base, no
// el que venía congelado en el token.
//
// Ejemplo: router.Post("/carros", aut.Requerida(), middleware.RequireRol("ADMIN"), handler)
func RequireRol(roles ...string) fiber.Handler {
	permitido := make(map[string]bool, len(roles))
	for _, r := range roles {
		permitido[r] = true
	}

	return func(c *fiber.Ctx) error {
		claims := ClaimsFromCtx(c)
		if claims == nil {
			return fiber.NewError(fiber.StatusUnauthorized, "no autenticado")
		}
		if !permitido[claims.Rol] {
			return fiber.NewError(fiber.StatusForbidden, "no tenés permiso para esta acción")
		}
		return c.Next()
	}
}
