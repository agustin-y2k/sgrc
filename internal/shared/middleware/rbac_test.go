package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// appConRolFalso simula un pipeline donde ya corrió JWTAuth (inyectando
// claims directo en el contexto), para testear RequireRol de forma aislada
// sin necesitar generar JWTs reales acá.
func appConRolFalso(rol string, claimsPresentes bool) *fiber.App {
	app := fiber.New()
	app.Get("/solo-admin",
		func(c *fiber.Ctx) error {
			if claimsPresentes {
				c.Locals("claims", &Claims{Rol: rol})
			}
			return c.Next()
		},
		RequireRol("ADMIN"),
		func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		},
	)
	return app
}

func TestRequireRol_RolPermitido_Pasa(t *testing.T) {
	app := appConRolFalso("ADMIN", true)

	resp, err := app.Test(httptest.NewRequest("GET", "/solo-admin", nil))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

func TestRequireRol_RolNoPermitido_403(t *testing.T) {
	app := appConRolFalso("DOCENTE", true)

	resp, _ := app.Test(httptest.NewRequest("GET", "/solo-admin", nil))
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

func TestRequireRol_SinClaims_401NoPanikea(t *testing.T) {
	// Caso defensivo: si por algún bug de wiring RequireRol corre sin que
	// JWTAuth haya corrido antes, no debe panickear con un nil pointer — debe
	// responder 401 igual que "no autenticado".
	app := appConRolFalso("", false)

	resp, err := app.Test(httptest.NewRequest("GET", "/solo-admin", nil))
	if err != nil {
		t.Fatalf("no debería panickear sin claims: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401 sin claims, obtuve %d", resp.StatusCode)
	}
}

func TestRequireRol_MultiplesRolesPermitidos(t *testing.T) {
	app := fiber.New()
	app.Get("/compartido",
		func(c *fiber.Ctx) error {
			c.Locals("claims", &Claims{Rol: "DOCENTE"})
			return c.Next()
		},
		RequireRol("ADMIN", "DOCENTE"),
		func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) },
	)

	resp, _ := app.Test(httptest.NewRequest("GET", "/compartido", nil))
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("DOCENTE debería poder acceder a una ruta compartida, obtuve %d", resp.StatusCode)
	}
}

func TestRequireRol_ListaVaciaDeRoles_BloqueaATodos(t *testing.T) {
	// Caso límite: RequireRol() sin argumentos no debería dejar pasar a
	// nadie (es una configuración rara, pero no debería fallar abierto).
	app := fiber.New()
	app.Get("/nadie-puede",
		func(c *fiber.Ctx) error {
			c.Locals("claims", &Claims{Rol: "ADMIN"})
			return c.Next()
		},
		RequireRol(),
		func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) },
	)

	resp, _ := app.Test(httptest.NewRequest("GET", "/nadie-puede", nil))
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("RequireRol() sin roles debería bloquear a todos (403), obtuve %d", resp.StatusCode)
	}
}
