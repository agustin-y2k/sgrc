// Package middleware agrupa JWT y RBAC — límites transversales que usan
// todos los paquetes de internal/, no lógica de dominio de ninguno en particular.
package middleware

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// HeaderMotivoSesion acompaña a cada 401 diciendo POR QUÉ no valió el token.
// El cliente ya recibe un mensaje distinto en cada caso, pero un mensaje es
// texto para una persona: decidir con él implicaría compararlo carácter por
// carácter, y cualquier retoque de redacción rompería la lógica en silencio.
//
// Existe porque no todos los rechazos son igual de interesantes para quien los
// sufre. Que la sesión venza es lo que pasa siempre y no hay nada que
// explicar; que la hayan cerrado desde afuera sí necesita una explicación. Sin
// distinguirlos, el frontend tiene que elegir entre avisar de más —un cartel
// de error cada vez que alguien vuelve al día siguiente— o de menos.
//
// Va en un header y no en el cuerpo porque el cuerpo de estos 401 es el
// mensaje pelado (Fiber v2 responde los fiber.NewError con
// SendString(err.Error()), sin ErrorHandler propio en cmd/main.go), y
// convertirlo en JSON cambiaría el contrato de TODOS los errores del sistema
// para resolver un caso. Está en Access-Control-Expose-Headers (ver CORS en
// internal/shared/middleware/security.go); sin eso el navegador lo recibe pero
// no deja leerlo desde otro origen.
const HeaderMotivoSesion = "X-Sesion-Motivo"

// Los valores posibles de HeaderMotivoSesion.
const (
	// MotivoExpirada: el token venció por su propio `exp`. Es el final normal
	// de toda sesión, no un problema.
	MotivoExpirada = "expirada"
	// MotivoInvalida: la firma no cierra, el algoritmo no es el nuestro, o el
	// token está mal formado. No pasa por accidente.
	MotivoInvalida = "invalida"
	// MotivoRevocada: la cuenta ya no está habilitada para operar — dada de
	// baja, rechazada, o borrada (RF-02.8).
	MotivoRevocada = "revocada"
	// MotivoPasswordCambiada: el token es anterior al último cambio de
	// contraseña de esa cuenta (RF-01.11).
	MotivoPasswordCambiada = "password-cambiada"
)

// sesionRechazada arma el 401 y deja el motivo en el header. Todos los
// rechazos de sesión salen por acá para que ninguno se olvide de marcarlo.
func sesionRechazada(c *fiber.Ctx, motivo, mensaje string) error {
	c.Set(HeaderMotivoSesion, motivo)
	return fiber.NewError(fiber.StatusUnauthorized, mensaje)
}

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
	// Recordarme marca que este token se emitió con "mantener la sesión
	// iniciada" tildado: vale JWT_REMEMBER_TTL en vez de JWT_ACCESS_TTL. Viaja
	// en el token porque las operaciones que vuelven a firmar (cambiar la
	// contraseña, editar el perfil) no tienen otra forma de saber que la sesión
	// era larga, y sin esto la degradarían a corta sin avisar.
	Recordarme bool `json:"rec,omitempty"`
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
			return sesionRechazada(c, MotivoInvalida, "falta el token")
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")
		if tokenStr == "" {
			return sesionRechazada(c, MotivoInvalida, "falta el token")
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
			// Vencido y falsificado son el mismo 401, pero no la misma
			// situación: lo primero le pasa a todo el mundo todos los días y lo
			// segundo no debería pasar nunca.
			if errors.Is(err, jwt.ErrTokenExpired) {
				return sesionRechazada(c, MotivoExpirada, "la sesión venció")
			}
			return sesionRechazada(c, MotivoInvalida, "token inválido o expirado")
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
			return sesionRechazada(c, MotivoRevocada, "la sesión ya no es válida")
		}

		// Un token emitido antes del último cambio de contraseña no vale más,
		// aunque la firma sea buena y la cuenta esté habilitada (RF-01.11).
		if claims.VersionSesion != cuenta.VersionSesion {
			return sesionRechazada(c, MotivoPasswordCambiada,
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
