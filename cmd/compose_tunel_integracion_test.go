//go:build integration

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCompose_TunelOpcionalYReinicio(t *testing.T) {
	for _, tc := range []struct {
		nombre            string
		local, desarrollo bool
		perfiles          []string
	}{
		{"produccion", false, false, nil},
		{"produccion-tunel", false, false, []string{"tunel"}},
		{"produccion-tableros", false, false, []string{"observabilidad"}},
		{"produccion-completo", false, false, []string{"observabilidad", "tunel"}},
		{"todos-los-perfiles", false, false, []string{"*"}},
		{"desarrollo", false, true, nil},
		{"desarrollo-tunel", false, true, []string{"tunel"}},
		{"local", true, false, nil},
		{"local-tunel", true, false, []string{"tunel"}},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			config, _ := resolverComposeAislado(t, tc.local, tc.desarrollo, true, tc.perfiles...)
			esperados := []string{"frontend", "postgres", "sgrc-app"}
			if tc.desarrollo {
				esperados = append(esperados, "seed-datos")
			}
			for _, perfil := range tc.perfiles {
				switch perfil {
				case "tunel":
					esperados = append(esperados, "cloudflared")
				case "observabilidad":
					esperados = append(esperados, "prometheus", "grafana", "dozzle")
				case "*":
					esperados = append(esperados, "cloudflared", "prometheus", "grafana", "dozzle")
				}
			}
			var actuales []string
			for nombre, servicio := range config.Services {
				actuales = append(actuales, nombre)
				if nombre != "seed-datos" && servicio.Restart != "unless-stopped" {
					t.Errorf("%s no tiene reinicio automático salvo detención manual", nombre)
				}
				if nombre == "cloudflared" && !reflect.DeepEqual(servicio.Profiles, []string{"tunel"}) {
					t.Error("el túnel no conserva el perfil tunel en todos los entornos")
				}
			}
			sort.Strings(esperados)
			sort.Strings(actuales)
			if !reflect.DeepEqual(actuales, esperados) {
				t.Errorf("servicios activos: %v; esperados: %v", actuales, esperados)
			}
		})
	}
}

func TestMake_SeleccionDeTunelSinEjecutarDocker(t *testing.T) {
	contenido, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	carpeta := t.TempDir()
	if err := os.WriteFile(filepath.Join(carpeta, "Makefile"), contenido, 0600); err != nil {
		t.Fatal(err)
	}
	// Un ejecutable señuelo vuelve observable cualquier ejecución accidental.
	bin := filepath.Join(carpeta, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	marca := filepath.Join(carpeta, "docker-ejecutado")
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\ntouch '"+marca+"'\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		target, contiene string
		sinTunel         bool
	}{
		{"run-prod", "docker compose up --build", true},
		{"run-prod-tunel", "docker compose --profile tunel up --build", false},
		{"levantar", "docker compose up -d --build postgres sgrc-app frontend", true},
		{"levantar TABLEROS=1", "docker compose up -d --build postgres sgrc-app frontend prometheus grafana dozzle", true},
		{"stop", "docker compose --profile '*' stop", true},
		{"down", "docker compose --profile '*' down", true},
		{"restart", "docker compose --profile '*' restart", true},
		{"logs", "docker compose --profile '*' logs -f", true},
		{"ps", "docker compose --profile '*' ps", true},
		{"dev-down", "--profile '*' down", true},
		{"local-stop", "--profile '*' stop", true},
		{"local-ps", "--profile '*' ps", true},
	} {
		t.Run(tc.target, func(t *testing.T) {
			args := append([]string{"-n", "--no-print-directory"}, strings.Fields(tc.target)...)
			cmd := exec.Command("make", args...)
			cmd.Dir = carpeta
			cmd.Env = []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "HOME=" + carpeta}
			salida, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("falló el ensayo de Make: %v", err)
			}
			if !strings.Contains(string(salida), tc.contiene) {
				t.Errorf("el comando no contiene %q", tc.contiene)
			}
			if tc.sinTunel && strings.Contains(string(salida), "--profile tunel") {
				t.Error("activa el túnel sin solicitarlo")
			}
			if strings.HasPrefix(tc.target, "levantar") && !strings.Contains(string(salida), "docker network connect") {
				t.Error("se perdió la reconexión del túnel externo")
			}
			if _, err := os.Stat(marca); !os.IsNotExist(err) {
				t.Fatal("se ejecutó Docker durante el ensayo")
			}
		})
	}
}
