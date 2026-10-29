package http

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/agustin-y2k/sgrc/internal/configuracion/application"
	"github.com/agustin-y2k/sgrc/internal/configuracion/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/authtest"
)

type repoHTTP struct{ guardadas int }

func (r *repoHTTP) Leer(context.Context) (domain.Operacion, error) {
	return domain.Operacion{RetiroGraciaMinutos: 40, RecordatorioAntelacionMinutos: 60, CierreGraciaMinutos: 60, CierreHoraSinJornada: 23, LicenciasHoraAviso: 8, Version: 1}, nil
}
func (r *repoHTTP) Guardar(_ context.Context, o domain.Operacion, actor, ip string) (domain.Operacion, error) {
	r.guardadas++
	return o, nil
}
func TestConfiguracion_SoloAdminYCuerpoCompleto(t *testing.T) {
	for _, tc := range []struct {
		nombre, rol, metodo, cuerpo string
		estado                      int
	}{
		{"sin-sesion", "", "GET", "", 401}, {"docente-lee", "DOCENTE", "GET", "", 403}, {"docente-edita", "DOCENTE", "PUT", `{}`, 403},
		{"admin-lee", "ADMIN", "GET", "", 200},
		{"falta-un-campo", "ADMIN", "PUT", `{"retiroGraciaMinutos":40,"recordatorioAntelacionMinutos":60,"cierreGraciaMinutos":60,"cierreHoraSinJornada":23,"version":1}`, 400},
		{"plazo-invalido", "ADMIN", "PUT", `{"retiroGraciaMinutos":0,"recordatorioAntelacionMinutos":60,"cierreGraciaMinutos":60,"cierreHoraSinJornada":23,"licenciasHoraAviso":8,"version":1}`, 400},
		{"cero-explicito-valido", "ADMIN", "PUT", `{"retiroGraciaMinutos":10,"recordatorioAntelacionMinutos":60,"cierreGraciaMinutos":0,"cierreHoraSinJornada":0,"licenciasHoraAviso":0,"version":1}`, 200},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			r := &repoHTTP{}
			registro := authtest.Nuevo()
			secret := []byte("secreto-de-prueba-de-configuracion-largo")
			app := fiber.New()
			RegisterRoutes(app, application.NewService(r), registro.Autenticacion(secret))
			req := httptest.NewRequest(tc.metodo, "/api/configuracion/operacion", strings.NewReader(tc.cuerpo))
			req.Header.Set("Content-Type", "application/json")
			if tc.rol != "" {
				req.Header.Set("Authorization", "Bearer "+registro.Token(secret, "cuenta", tc.rol))
			}
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != tc.estado {
				t.Fatalf("estado=%d esperado=%d", res.StatusCode, tc.estado)
			}
			if tc.estado != 200 && r.guardadas != 0 {
				t.Fatal("se escribió pese al rechazo")
			}
		})
	}
}
