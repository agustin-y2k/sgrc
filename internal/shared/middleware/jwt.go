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

// Claims son los datos que viajan en el JWT (ver docs/09-seguridad-rbac.md §2).
// HS256, no RS256: un solo proceso firma y verifica, así que un secreto
// simétrico alcanza (ver docs/06-arquitectura.md §7).
type Claims struct {
	UserID   string `json:"sub"`
	Rol      string `json:"rol"`
	Nombre   string `json:"nombre"`
	Apellido string `json:"apellido"`
	// DebeCambiarPassword viaja en el token para que RF-01.6 se pueda
	// exigir sin ir a la base en cada request. Como el valor queda
	// congelado en el token, CambiarPassword devuelve uno nuevo: si no,
	// quien acaba de cambiarla seguiría bloqueado hasta que expire.
	DebeCambiarPassword bool `json:"dcp,omitempty"`
	jwt.RegisteredClaims
}

// CuentaVigente responde si la cuenta sigue habilitada para operar, y con
// qué rol, en ESTE momento — no cuando se emitió el token.
//
// Es un puerto hacia auth: middleware/ no puede importar internal/auth (ver
// docs/06-arquitectura.md §3), así que la implementación concreta vive en
// auth/infrastructure y se inyecta desde cmd/main.go, igual que los demás
// validadores que cruzan paquetes.
//
// Devuelve vigente=false tanto para una cuenta en PENDIENTE/RECHAZADA/BAJA
// como para un usuario que ya no existe (RF-01.9, hard delete): en los dos
// casos el token no debe seguir sirviendo.
type CuentaVigente func(ctx context.Context, usuarioID string) (vigente bool, rol string, err error)

// Autenticacion agrupa lo que hace falta para autenticar un request: el
// secreto con el que se verifica la firma, y la consulta que dice si la
// cuenta sigue habilitada.
//
// Van juntos a propósito, en un solo valor que se pasa a cada
// RegisterRoutes: separarlos permitiría montar una ruta con firma válida
// pero sin verificación de estado, que es justamente el agujero que esto
// cierra.
type Autenticacion struct {
	Secret  []byte
	Vigente CuentaVigente
}

// Requerida valida el Bearer token, exige que la cuenta siga habilitada,
// que la contraseña esté al día (RF-01.6), y deja los Claims en
// c.Locals("claims") para los handlers y el RBAC.
//
// Que la exigencia de RF-01.6 esté acá adentro y no en un middleware
// separado es deliberado: así una ruta nueva queda protegida por omisión y
// las únicas dos que se saltean la regla tienen que pedirlo explícitamente
// con RequeridaPermitiendoPasswordVencida. Antes esto solo lo verificaba
// <ProtectedRoute> en el navegador, así que el token que devolvía el login
// con contraseña temporal servía contra toda la API.
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
// RF-01.6 — pero SÍ con la verificación de cuenta vigente. Solo para las dos
// rutas que alguien con contraseña temporal necesita justamente para salir
// de esa situación: GET /me y POST /cambiar-password. Una cuenta dada de
// baja tampoco puede usarlas.
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
// deja los claims en el contexto. Devuelve nil sin llamar a c.Next(): de eso
// se encargan sus dos envoltorios, que primero deciden si además hay que
// exigir el cambio de contraseña.
func (a Autenticacion) validador() func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		// Guards defensivos: un secreto vacío firmaría/validaría cualquier
		// token con clave vacía, y sin Vigente el token de una cuenta dada
		// de baja seguiría sirviendo hasta expirar. Más vale fallar
		// ruidosamente acá que dejar pasar un wiring incompleto.
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
			// Defensa adicional: rechazar cualquier algoritmo que no sea
			// HS256 explícitamente. Sin esto, un token firmado con "none"
			// o con un algoritmo asimétrico podría colarse dependiendo de
			// cómo se implemente el keyFunc (ataque conocido sobre JWT).
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("algoritmo de firma inesperado: %v", t.Header["alg"])
			}
			return a.Secret, nil
		})
		if err != nil || !token.Valid {
			return fiber.NewError(fiber.StatusUnauthorized, "token inválido o expirado")
		}

		// La firma solo prueba que el token lo emitimos nosotros, no que
		// siga valiendo. Sin esta consulta, dar de baja una cuenta (RF-02.8),
		// rechazarla, o eliminarla (RF-01.9) no tenía ningún efecto sobre el
		// token que esa persona ya tenía en el navegador: seguía leyendo y
		// ESCRIBIENDO por hasta JWT_ACCESS_TTL. Es una lectura por PK en
		// cada request autenticado — el precio de que "dar de baja" quiera
		// decir lo que dice.
		vigente, rolActual, err := a.Vigente(c.UserContext(), claims.UserID)
		if err != nil {
			// Fallar cerrado: si no podemos confirmar que la cuenta sigue
			// habilitada, no la dejamos pasar.
			return fiber.NewError(fiber.StatusServiceUnavailable,
				"no se pudo verificar la sesión, probá de nuevo en unos segundos")
		}
		if !vigente {
			return fiber.NewError(fiber.StatusUnauthorized, "la sesión ya no es válida")
		}
		// El rol de la base gana sobre el del token: el RBAC de cada ruta
		// decide con el valor de ahora, no con el del momento del login.
		claims.Rol = rolActual

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
