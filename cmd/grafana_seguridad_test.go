package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestGrafana_ArranqueExigeClavePropia(t *testing.T) {
	azar := make([]byte, 24)
	if _, err := rand.Read(azar); err != nil {
		t.Fatal(err)
	}
	privada := hex.EncodeToString(azar)
	for _, caso := range []struct {
		nombre, clave string
		codigo        int
	}{
		{"vacia", "", 1},
		{"admin_por_defecto", "admin", 1},
		{"plantilla", "cambiar_por_una_contrasena_larga", 1},
		{"fallback_publico", "sin-configurar-ver-make-observabilidad", 1},
		{"corta", "corta", 1},
		{"propia", privada, 23},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Setenv("GF_SECURITY_ADMIN_PASSWORD", caso.clave)
			// El proceso sólo se ejecuta cuando la validación deja pasar. Su
			// código distingue un exec efectivo de salir sin iniciar Grafana.
			cmd := exec.Command("sh", "../observabilidad/grafana/arrancar-seguro.sh", "sh", "-c", "exit 23")
			salida, err := cmd.CombinedOutput()
			var fallo *exec.ExitError
			if !errors.As(err, &fallo) || fallo.ExitCode() != caso.codigo {
				t.Fatalf("arranque: se esperaba código %d; error %v", caso.codigo, err)
			}
			if caso.clave != "" && strings.Contains(string(salida), caso.clave) {
				t.Fatal("el arranque imprime la contraseña")
			}
		})
	}
}
