package http

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/agustin-y2k/sgrc/internal/reporting/application"
	"github.com/agustin-y2k/sgrc/internal/reporting/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/authtest"
)

type actividadHTTP struct {
	llamadas int
	err      error
}

func (a *actividadHTTP) CalcularActividad(context.Context, string, *time.Time, *time.Time) (domain.Actividad, error) {
	a.llamadas++
	return domain.Actividad{Version: 1, Anio: 2026, Disponible: true}, a.err
}
func TestActividad_RespetaPermisosYRangos(t *testing.T) {
	for _, tc := range []struct {
		nombre, rol, query string
		fallo              error
		estado             int
		llamadas           int
	}{
		{"sin-sesion", "", "", nil, 401, 0}, {"docente", "DOCENTE", "", nil, 403, 0}, {"admin", "ADMIN", "", nil, 200, 1},
		{"rango-invertido", "ADMIN", "?desde=2026-05-10&hasta=2026-05-01", nil, 400, 0},
		{"historico-con-rango", "ADMIN", "?desde=2026-05-01", application.ErrHistoricoSinRango, 400, 1},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			lector := &actividadHTTP{err: tc.fallo}
			app := fiber.New()
			registro := authtest.Nuevo()
			secret := []byte("secreto-de-prueba-para-reportes-de-actividad")
			RegisterRoutes(app, NewHandler(nil).ConActividad(lector), registro.Autenticacion(secret))
			req := httptest.NewRequest("GET", "/api/ciclos/ciclo/actividad"+tc.query, nil)
			if tc.rol != "" {
				req.Header.Set("Authorization", "Bearer "+registro.Token(secret, "persona", tc.rol))
			}
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != tc.estado || lector.llamadas != tc.llamadas {
				t.Fatalf("estado=%d llamadas=%d", res.StatusCode, lector.llamadas)
			}
		})
	}
}
