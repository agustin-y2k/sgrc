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

// ── Rutas y codificación del camino ─────────────────────────────────────

// El RBAC de SGRC se aplica por grupo de rutas: `app.Group("/api/auditoria",
// RequireRol("ADMIN"))` y todo lo que cuelga de ahí queda protegido. Eso
// significa que la protección depende de cómo el router resuelve el camino del
// pedido, y hay una familia de vulnerabilidades —GO-2026-4950 en fasthttp es
// una— donde un camino codificado dos veces se resuelve distinto según quién
// lo mire, y termina llegando a un handler salteando el control que estaba
// adelante.
//
// En Fiber esto no puede pasar por construcción: el middleware del grupo y el
// handler los elige el mismo match, así que si el camino no matchea el grupo
// tampoco matchea el handler. Este test fija esa propiedad, que es una
// suposición de la que cuelga todo el modelo de permisos, para que no se rompa
// en silencio en una actualización.
func TestRutasProtegidas_NoSeLlegaConElCaminoCodificado(t *testing.T) {
	app := fiber.New()
	admin := app.Group("/api/auditoria", RequireRol("ADMIN")) // sin claims: 401
	admin.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	// Todas estas son formas de escribir /api/auditoria/ que un atacante
	// probaría esperando que el router las acepte y el grupo no las vea.
	caminos := []string{
		"/api/%61uditoria/",      // una letra percent-encoded
		"/api/%2561uditoria/",    // doble codificación del mismo byte
		"/api/../api/auditoria/", // travesía que vuelve al mismo lugar
		"/api/%2e%2e/api/auditoria/",
		"/api/%252e%252e/api/auditoria/",
		"/api//auditoria/", // barra de más
		"/api/./auditoria/",
		"/API/AUDITORIA/", // el router es sensible a mayúsculas
	}

	for _, camino := range caminos {
		resp, err := app.Test(httptest.NewRequest("GET", camino, nil))
		if err != nil {
			t.Fatalf("%s: error inesperado: %v", camino, err)
		}
		// 401 (lo agarró el RBAC) o 404 (no matcheó nada) están los dos bien.
		// Lo único inaceptable es 200: eso sería el handler corriendo sin que
		// el control de permisos se haya ejecutado.
		if resp.StatusCode == fiber.StatusOK {
			t.Fatalf("%s llegó al handler sin pasar por RequireRol", camino)
		}
	}

	// Y el control de la otra punta: el camino bien escrito sí llega al RBAC,
	// porque un test donde todo da 404 no probaría nada.
	resp, err := app.Test(httptest.NewRequest("GET", "/api/auditoria/", nil))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("el camino correcto tendría que caer en RequireRol (401), obtuve %d", resp.StatusCode)
	}
}
