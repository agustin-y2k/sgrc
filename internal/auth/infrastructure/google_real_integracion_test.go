//go:build integration

package infrastructure

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Usa un ID token Google reciente, entregado por archivo, y el adaptador de
// producción con su endpoint JWKS normal. No imprime token ni identidad.
func TestGoogleReal_IDToken(t *testing.T) {
	if os.Getenv("SGRC_TEST_GOOGLE_REAL") != "1" {
		t.Skip("activar SGRC_TEST_GOOGLE_REAL=1 para verificar un token real")
	}
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	archivo := strings.TrimSpace(os.Getenv("SGRC_TEST_GOOGLE_TOKEN_FILE"))
	if clientID == "" || archivo == "" {
		t.Fatal("configurar GOOGLE_CLIENT_ID y SGRC_TEST_GOOGLE_TOKEN_FILE")
	}
	datos, err := os.ReadFile(archivo)
	if err != nil {
		t.Fatal("no se pudo leer el archivo del ID token")
	}
	if len(datos) == 0 || len(datos) > 64*1024 {
		t.Fatal("el archivo del ID token está vacío o excede 64 KiB")
	}
	verificador := NewVerificadorGoogle(clientID, DominiosPermitidos(os.Getenv("GOOGLE_DOMINIOS_PERMITIDOS")), time.Now)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	identidad, err := verificador.Verificar(ctx, strings.TrimSpace(string(datos)))
	if err != nil {
		t.Fatal("Google no pudo validar el ID token: revisar conectividad, firma, client ID, dominio y expiración")
	}
	if identidad.Sub == "" || identidad.Email == "" || !identidad.EmailVerificado {
		t.Fatal("el ID token no contiene una identidad con email verificado")
	}
	t.Log("Google verificó firma, audiencia, emisor, expiración, dominio e identidad. Esta prueba no registra ni modifica cuentas del SGRC.")
}
