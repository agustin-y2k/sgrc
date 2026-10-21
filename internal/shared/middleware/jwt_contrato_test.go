package middleware

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

func TestJWTAuth_ExigeHS256YExpiracionEnAmbasRutas(t *testing.T) {
	for _, temporal := range []bool{false, true} {
		for _, caso := range []struct {
			nombre string
			metodo jwt.SigningMethod
			exp    *jwt.NumericDate
			estado int
		}{
			{"hs256-vigente", jwt.SigningMethodHS256, jwt.NewNumericDate(time.Now().Add(time.Hour)), 200},
			{"hs384", jwt.SigningMethodHS384, jwt.NewNumericDate(time.Now().Add(time.Hour)), 401},
			{"hs512", jwt.SigningMethodHS512, jwt.NewNumericDate(time.Now().Add(time.Hour)), 401},
			{"sin-expiracion", jwt.SigningMethodHS256, nil, 401},
			{"expirado", jwt.SigningMethodHS256, jwt.NewNumericDate(time.Now().Add(-time.Hour)), 401},
		} {
			ruta := "normal/"
			if temporal {
				ruta = "password-temporal/"
			}
			t.Run(ruta+caso.nombre, func(t *testing.T) {
				consultas := 0
				aut := Autenticacion{Secret: testSecret, Vigente: func(context.Context, string) (EstadoDeCuenta, error) {
					consultas++
					return EstadoDeCuenta{Vigente: true, Rol: "DOCENTE"}, nil
				}}
				app := fiber.New()
				proteger := aut.Requerida()
				if temporal {
					proteger = aut.RequeridaPermitiendoPasswordVencida()
				}
				app.Get("/protegido", proteger, func(c *fiber.Ctx) error { return c.SendStatus(200) })
				claims := Claims{UserID: "user-1", DebeCambiarPassword: temporal, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: caso.exp}}
				token, err := jwt.NewWithClaims(caso.metodo, claims).SignedString(testSecret)
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest("GET", "/protegido", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				resp, err := app.Test(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != caso.estado {
					t.Fatalf("estado=%d esperado=%d", resp.StatusCode, caso.estado)
				}
				if caso.estado == 401 && consultas != 0 {
					t.Fatal("se consultó la cuenta de un token inválido")
				}
				motivo := MotivoInvalida
				if caso.nombre == "expirado" {
					motivo = MotivoExpirada
				}
				if caso.estado == 401 && resp.Header.Get(HeaderMotivoSesion) != motivo {
					t.Fatalf("motivo incorrecto: %s", resp.Header.Get(HeaderMotivoSesion))
				}
			})
		}
	}
}
