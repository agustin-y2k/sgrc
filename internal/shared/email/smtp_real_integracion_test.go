//go:build integration

package email

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// El envío externo requiere una activación explícita y una casilla de prueba.
// Nunca se envía correo al correr la suite de integración habitual.
func TestSMTPReal_EnvioAutorizado(t *testing.T) {
	if os.Getenv("SGRC_TEST_SMTP_REAL") != "1" {
		t.Skip("activar SGRC_TEST_SMTP_REAL=1 para enviar un correo real")
	}
	para := strings.TrimSpace(os.Getenv("SGRC_TEST_SMTP_TO"))
	if para == "" {
		t.Fatal("falta SGRC_TEST_SMTP_TO: usar una casilla propia autorizada")
	}
	enviador, err := DesdeEntorno(os.Getenv, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := enviador.(*EnviadorSMTP); !ok {
		t.Fatal("SMTP_HOST está vacío: un envío deshabilitado no valida SMTP real")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := enviador.Enviar(ctx, para, "SGRC — prueba local de correo", "Prueba del backend SGRC con SMTP y STARTTLS.\nSi recibiste este mensaje, confirmá también la recepción y revisá la carpeta de spam."); err != nil {
		t.Fatal(err)
	}
	t.Log("El servidor SMTP aceptó el mensaje. Confirmar la recepción en la casilla: la aceptación SMTP no garantiza entrega final.")
}
