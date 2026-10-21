package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/auth/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/authtest"
	"github.com/gofiber/fiber/v2"
)

func TestHTTP_PerfilConVersionDeSesion(t *testing.T) {
	for _, caso := range []struct {
		nombre             string
		versionTokenActual bool
		codigo             int
	}{
		{"sesion vigente", true, fiber.StatusOK},
		{"reset despues de autenticar", false, fiber.StatusConflict},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			anterior := registroDePrueba
			registroDePrueba = authtest.Nuevo()
			t.Cleanup(func() { registroDePrueba = anterior })
			repo := nuevoFakeRepo()
			repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Nombre: "Ada", Apellido: "Byron", Estado: domain.EstadoAprobada, Rol: domain.RolDocente, VersionSesion: 1}
			// Simula las dos lecturas de la cuenta: middleware y servicio. Si hubo un
			// reset entre ellas, el middleware aún había autenticado la versión 0.
			if caso.versionTokenActual {
				registroDePrueba.InvalidarSesiones("u1")
			}
			app := nuevaAppDeTest(repo)
			req := httptest.NewRequest("PATCH", "/api/mi-perfil", jsonBody(actualizarMisDatosRequest{Nombre: "Ada", Apellido: "Lovelace"}))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != caso.codigo {
				t.Fatalf("esperaba %d, obtuve %d", caso.codigo, resp.StatusCode)
			}
			if caso.versionTokenActual {
				var body actualizarMisDatosResponse
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body.Token == "" || body.Usuario.Apellido != "Lovelace" {
					t.Fatalf("respuesta incorrecta: %+v", body)
				}
			} else if repo.usuarios["u1"].Apellido != "Byron" {
				t.Fatal("una sesion revocada modifico el perfil")
			}
		})
	}
}
