package middleware

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// El header X-Sesion-Motivo es lo que le permite al frontend distinguir "tu
// sesión venció" —que no se anuncia, le pasa a todo el mundo— de "te cerraron
// la sesión", que sí hay que explicar. Antes de esto, los cuatro rechazos eran
// un 401 indistinguible salvo por el texto del mensaje.

func motivoDe(t *testing.T, app *fiber.App, token string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", "/protegido", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	return resp.StatusCode, resp.Header.Get(HeaderMotivoSesion)
}

func TestMotivo_TokenExpirado_Expirada(t *testing.T) {
	app := appConProteccion(testSecret)
	tok := tokenValido(t, testSecret, time.Now().Add(-time.Hour))

	status, motivo := motivoDe(t, app, tok)

	if status != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", status)
	}
	if motivo != MotivoExpirada {
		t.Fatalf("un token vencido tiene que marcarse como %q, marcó %q", MotivoExpirada, motivo)
	}
}

// Vencido y falsificado comparten el 401 pero no la situación: el primero es
// el final normal de una sesión y el segundo no debería pasar nunca.
func TestMotivo_FirmaInvalida_NoEsExpirada(t *testing.T) {
	app := appConProteccion(testSecret)
	tok := tokenValido(t, []byte("secreto-incorrecto-pero-largo"), time.Now().Add(time.Hour))

	status, motivo := motivoDe(t, app, tok)

	if status != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", status)
	}
	if motivo != MotivoInvalida {
		t.Fatalf("esperaba %q, obtuve %q", MotivoInvalida, motivo)
	}
}

func TestMotivo_CuentaDadaDeBaja_Revocada(t *testing.T) {
	aut := Autenticacion{
		Secret: testSecret,
		Vigente: func(context.Context, string) (EstadoDeCuenta, error) {
			return EstadoDeCuenta{Vigente: false}, nil
		},
	}
	app := appConAutenticacion(aut)
	tok := tokenValido(t, testSecret, time.Now().Add(time.Hour))

	status, motivo := motivoDe(t, app, tok)

	if status != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", status)
	}
	if motivo != MotivoRevocada {
		t.Fatalf("esperaba %q, obtuve %q", MotivoRevocada, motivo)
	}
}

func TestMotivo_VersionDeSesionVieja_PasswordCambiada(t *testing.T) {
	aut := Autenticacion{
		Secret: testSecret,
		Vigente: func(context.Context, string) (EstadoDeCuenta, error) {
			// La cuenta está bien; lo que cambió es la contraseña, y con ella
			// la versión de sesión (RF-01.11).
			return EstadoDeCuenta{Vigente: true, Rol: "ADMIN", VersionSesion: 7}, nil
		},
	}
	app := appConAutenticacion(aut)
	tok := tokenValido(t, testSecret, time.Now().Add(time.Hour)) // versión 0

	status, motivo := motivoDe(t, app, tok)

	if status != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", status)
	}
	if motivo != MotivoPasswordCambiada {
		t.Fatalf("esperaba %q, obtuve %q", MotivoPasswordCambiada, motivo)
	}
}

// Una respuesta exitosa no lleva el header: si lo llevara, el cliente tendría
// que mirar el status igual y el header dejaría de significar algo.
func TestMotivo_RequestExitoso_SinHeader(t *testing.T) {
	app := appConProteccion(testSecret)
	tok := tokenValido(t, testSecret, time.Now().Add(time.Hour))

	status, motivo := motivoDe(t, app, tok)

	if status != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", status)
	}
	if motivo != "" {
		t.Fatalf("una respuesta exitosa no debería traer motivo, trajo %q", motivo)
	}
}
