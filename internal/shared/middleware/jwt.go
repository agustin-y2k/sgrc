// Package middleware agrupa JWT y RBAC — límites transversales que usan
// todos los paquetes de internal/, no lógica de dominio de ninguno en particular.
package middleware

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Claims son los datos que viajan en el JWT (ver docs/09-seguridad-rbac.md
// §2).
type Claims struct {
	UserID   string `json:"sub"`
	Rol      string `json:"rol"`
	Nombre   string `json:"nombre"`
	Apellido string `json:"apellido"`
	// DebeCambiarPassword viaja en el token para que RF-01.6 se pueda exigir sin
	// ir a la base en cada request.
	DebeCambiarPassword bool `json:"dcp,omitempty"`
	// VersionSesion es la versión que tenía la cuenta al emitirse este token.
	VersionSesion int `json:"vs,omitempty"`
	jwt.RegisteredClaims
}

// EstadoDeCuenta es lo que el middleware necesita de la base para decidir si
// un token sigue valiendo.
type EstadoDeCuenta struct {
	// Vigente es false para una cuenta en PENDIENTE/RECHAZADA/BAJA y para un
	// usuario que ya no existe (RF-01.9): en los dos casos el token no debe
	// seguir sirviendo.
	Vigente bool
	// Rol de AHORA, no el del momento del login.
	Rol string
	// VersionSesion actual. Un token con otra versión se emitió antes del
	// último cambio de contraseña.
	VersionSesion int
}

// CuentaVigente responde si la cuenta sigue habilitada para operar, con qué
// rol y con qué versión de sesión, en ESTE momento — no cuando se emitió el
// token.
type CuentaVigente func(ctx context.Context, usuarioID string) (EstadoDeCuenta, error)

// Autenticacion agrupa lo que hace falta para autenticar un request: el
// secreto con el que se verifica la firma, y la consulta que dice si la
// cuenta sigue habilitada.
type Autenticacion struct {
	Secret  []byte
	Vigente CuentaVigente
}

// Requerida valida el Bearer token, exige que la cuenta siga habilitada, que
// la contraseña esté al día (RF-01.6), y deja los Claims en
// c.Locals("claims") para los handlers y el RBAC. Que la exigencia de RF-01.6
// esté acá adentro y no en un middleware separado es deliberado: así una ruta
// nueva queda protegida por omisión y las únicas dos que se saltean la regla
// tienen que pedirlo explícitamente con RequeridaPermitiendoPasswordVencida.
func (a Autenticacion) Requerida() fiber.Handler {
	validar := a.validador()
	return func(c *fiber.Ctx) error {
		if err := validar(c); err != nil {
			return err
		}
		if claims := ClaimsFromCtx(c); claims != nil && claims.DebeCambiarPassword {
			return fiber.NewError(fiber.StatusForbidden,
				"tenés que cambiar tu contraseña temporal antes de usar el sistema")
		}
		return c.Next()
	}
}

// RequeridaPermitiendoPasswordVencida es Requerida sin la exigencia de
// RF-01.6 — pero SÍ con la verificación de cuenta vigente.
func (a Autenticacion) RequeridaPermitiendoPasswordVencida() fiber.Handler {
	validar := a.validador()
	return func(c *fiber.Ctx) error {
		if err := validar(c); err != nil {
			return err
		}
		return c.Next()
	}
}

// validador verifica la firma del token y que la cuenta siga habilitada, y
// deja los claims en el contexto.
func (a Autenticacion) validador() func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		// Guards defensivos: un secreto vacío firmaría/validaría cualquier token
		// con clave vacía, y sin Vigente el token de una cuenta dada de baja
		// seguiría sirviendo hasta expirar.
		if len(a.Secret) == 0 {
			return fiber.NewError(fiber.StatusInternalServerError, "JWT_SECRET no configurado")
		}
		if a.Vigente == nil {
			return fiber.NewError(fiber.StatusInternalServerError, "verificación de cuenta no configurada")
		}

		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return fiber.NewError(fiber.StatusUnauthorized, "falta el token")
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")
		if tokenStr == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "falta el token")
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
			// Defensa adicional: rechazar cualquier algoritmo que no sea HS256
			// explícitamente.
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("algoritmo de firma inesperado: %v", t.Header["alg"])
			}
			return a.Secret, nil
		})
		if err != nil || !token.Valid {
			return fiber.NewError(fiber.StatusUnauthorized, "token inválido o expirado")
		}

		// La firma solo prueba que el token lo emitimos nosotros, no que siga
		// valiendo.
		cuenta, err := a.Vigente(c.UserContext(), claims.UserID)
		if err != nil {
			// Fallar cerrado: si no podemos confirmar que la cuenta sigue
			// habilitada, no la dejamos pasar.
			return fiber.NewError(fiber.StatusServiceUnavailable,
				"no se pudo verificar la sesión, probá de nuevo en unos segundos")
		}
		if !cuenta.Vigente {
			return fiber.NewError(fiber.StatusUnauthorized, "la sesión ya no es válida")
		}

		// Un token emitido antes del último cambio de contraseña no vale más,
		// aunque la firma sea buena y la cuenta esté habilitada (RF-01.11).
		if claims.VersionSesion != cuenta.VersionSesion {
			return fiber.NewError(fiber.StatusUnauthorized,
				"tu sesión se cerró porque la contraseña de esta cuenta cambió; volvé a entrar")
		}

		// El rol de la base gana sobre el del token: el RBAC de cada ruta
		// decide con el valor de ahora, no con el del momento del login.
		claims.Rol = cuenta.Rol

		c.Locals("claims", claims)
		return nil
	}
}

// ClaimsFromCtx es el helper que usan los handlers para leer el usuario
// autenticado sin repetir el type assertion en cada uno.
func ClaimsFromCtx(c *fiber.Ctx) *Claims {
	claims, _ := c.Locals("claims").(*Claims)
	return claims
}
