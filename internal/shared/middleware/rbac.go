package middleware

import "github.com/gofiber/fiber/v2"

// RequireRol bloquea el acceso si el rol del usuario autenticado no está en
// la lista permitida.
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
