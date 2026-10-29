//go:build integration

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var variablesBackendCompose = strings.Fields(`POSTGRES_HOST POSTGRES_PORT POSTGRES_DB POSTGRES_USER POSTGRES_PASSWORD
 JWT_SECRET JWT_ACCESS_TTL JWT_REMEMBER_TTL OUTBOX_SECRET CUENTAS_SECRET SEED_ADMIN_EMAIL SEED_ADMIN_PASSWORD
 FRONTEND_ORIGIN TRUSTED_PROXIES APP_PORT APP_TIMEZONE GOOGLE_CLIENT_ID GOOGLE_DOMINIOS_PERMITIDOS
 SMTP_HOST SMTP_PORT SMTP_USER SMTP_PASSWORD SMTP_FROM SMTP_FROM_NAME LICENCIAS_HORA_AVISO
 LICENCIAS_INTERVALO_MINUTOS RETIRO_GRACIA_MINUTOS CIERRE_JORNADA PING_URL_RESERVAS_VENCIDAS
 PING_URL_BARRIDO_ENTREGAS PING_URL_AVISO_LICENCIAS RETIRO_AVISO_MINUTOS DEVOLUCION_DEMORA_MINUTOS`)

type servicioComposeAislado struct {
	Environment map[string]string `json:"environment"`
	EnvFile     []json.RawMessage `json:"env_file"`
	Profiles    []string          `json:"profiles"`
	Restart     string            `json:"restart"`
}
type composeAislado struct {
	Services map[string]servicioComposeAislado `json:"services"`
}

// La configuración se resuelve fuera del repositorio y con un entorno de proceso
// vacío salvo PATH y HOME temporal. Nunca se leen .env privados ni se arranca Docker.
func resolverComposeAislado(t *testing.T, local, desarrollo, opcionalesVacias bool, perfiles ...string) (composeAislado, map[string]string) {
	t.Helper()
	raiz, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	carpeta := t.TempDir()
	for _, archivo := range []string{"docker-compose.yml", "docker-compose.dev.yml", "docker-compose.local.example.yml"} {
		datos, err := os.ReadFile(filepath.Join(raiz, archivo))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(carpeta, archivo), datos, 0600); err != nil {
			t.Fatal(err)
		}
	}
	variables := append(append([]string{}, variablesBackendCompose...), "TUNNEL_TOKEN", "GRAFANA_PASSWORD", "DOCENTE_EMAIL", "DOCENTE_PASSWORD", "ANIO", "CANTIDAD_EQUIPOS", "CANARIO_DE_AISLAMIENTO")
	var elegidas map[string]string
	for _, nombre := range []string{".env", ".env.local"} {
		prefijo := "produccion"
		if nombre == ".env.local" {
			prefijo = "local"
		}
		valores := map[string]string{}
		var lineas []string
		for _, clave := range variables {
			valor := prefijo + "_prueba_" + clave
			if opcionalesVacias && (clave == "OUTBOX_SECRET" || clave == "CUENTAS_SECRET" || strings.HasPrefix(clave, "SMTP_") || strings.HasPrefix(clave, "GOOGLE_") || strings.HasPrefix(clave, "PING_URL_") || clave == "TUNNEL_TOKEN") {
				valor = ""
			}
			if clave == "SMTP_PASSWORD" && !opcionalesVacias {
				valor = prefijo + " $literal # con espacios "
			}
			valores[clave] = valor
			lineas = append(lineas, clave+"='"+valor+"'")
		}
		if err := os.WriteFile(filepath.Join(carpeta, nombre), []byte(strings.Join(lineas, "\n")+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if (local && nombre == ".env.local") || (!local && nombre == ".env") {
			elegidas = valores
		}
	}
	entorno := ".env"
	if local {
		entorno = ".env.local"
	}
	args := []string{"compose", "-p", "prueba-aislamiento", "--project-directory", carpeta, "--env-file", entorno, "-f", "docker-compose.yml"}
	if desarrollo {
		args = append(args, "-f", "docker-compose.dev.yml")
	}
	if local {
		args = append(args, "-f", "docker-compose.local.example.yml")
	}
	for _, perfil := range perfiles {
		args = append(args, "--profile", perfil)
	}
	args = append(args, "config", "--format", "json")
	cmd := exec.Command("docker", args...)
	cmd.Dir = carpeta
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + carpeta}
	datos, err := cmd.Output()
	if err != nil {
		t.Fatalf("no se pudo resolver el Compose aislado: %v (salida omitida para proteger valores)", err)
	}
	var config composeAislado
	if err := json.Unmarshal(datos, &config); err != nil {
		t.Fatal("Compose devolvió una configuración JSON inválida")
	}
	return config, elegidas
}
func TestCompose_AislaVariablesPorServicio(t *testing.T) {
	for _, tc := range []struct {
		nombre                   string
		local, desarrollo, tunel bool
	}{
		{"produccion", false, false, false}, {"produccion-con-tunel", false, false, true}, {"desarrollo", false, true, false}, {"desarrollo-con-tunel", false, true, true}, {"local-sin-tunel", true, false, false}, {"local-con-tunel", true, false, true},
	} {
		for _, vacias := range []bool{false, true} {
			nombre := tc.nombre + "-completo"
			if vacias {
				nombre = tc.nombre + "-opcionales-vacias"
			}
			t.Run(nombre, func(t *testing.T) {
				perfiles := []string{"observabilidad"}
				if tc.tunel {
					perfiles = append(perfiles, "tunel")
				}
				config, valores := resolverComposeAislado(t, tc.local, tc.desarrollo, vacias, perfiles...)
				if !tc.tunel {
					if _, ok := config.Services["cloudflared"]; ok {
						t.Fatal("el túnel se habilitó sin solicitar su perfil")
					}
				}
				esperadas := map[string]map[string]string{
					"cloudflared": {"TUNNEL_TOKEN": valores["TUNNEL_TOKEN"]},
					"seed-datos":  {"API": "http://sgrc-app:8080", "ADMIN_EMAIL": valores["SEED_ADMIN_EMAIL"], "ADMIN_PASSWORD": valores["SEED_ADMIN_PASSWORD"], "DOCENTE_EMAIL": valores["DOCENTE_EMAIL"], "DOCENTE_PASSWORD": valores["DOCENTE_PASSWORD"], "ANIO": valores["ANIO"], "CANTIDAD_EQUIPOS": valores["CANTIDAD_EQUIPOS"]},
					"sgrc-app":    {"GOMEMLIMIT": "800MiB"}, "postgres": {"POSTGRES_DB": valores["POSTGRES_DB"], "POSTGRES_USER": valores["POSTGRES_USER"], "POSTGRES_PASSWORD": valores["POSTGRES_PASSWORD"]},
					"frontend": {}, "prometheus": {}, "grafana": {"GF_SECURITY_ADMIN_PASSWORD": valores["GRAFANA_PASSWORD"], "GF_USERS_ALLOW_SIGN_UP": "false", "GF_AUTH_ANONYMOUS_ENABLED": "false"},
					"dozzle": {"DOZZLE_FILTER": "label=sgrc.logs=si", "DOZZLE_NO_ANALYTICS": "true", "DOZZLE_RELEASE_CHECK_MODE": "manual", "DOZZLE_IMAGE_CHECK_MODE": "off"},
				}
				for _, clave := range variablesBackendCompose {
					esperadas["sgrc-app"][clave] = valores[clave]
				}
				if tc.local {
					esperadas["dozzle"]["DOZZLE_FILTER"] = "label=com.docker.compose.project=sgrc-local"
				}
				for _, nombre := range []string{"cloudflared", "seed-datos", "sgrc-app", "postgres", "frontend", "prometheus", "grafana", "dozzle"} {
					servicio, ok := config.Services[nombre]
					if !ok {
						if (nombre == "cloudflared" && !tc.tunel) || (nombre == "seed-datos" && !tc.desarrollo) {
							continue
						}
						t.Fatalf("falta el servicio %s", nombre)
					}
					t.Run(nombre, func(t *testing.T) {
						claves := make([]string, 0, len(servicio.Environment))
						for clave := range servicio.Environment {
							claves = append(claves, clave)
						}
						sort.Strings(claves)
						for _, clave := range claves {
							if _, ok := esperadas[nombre][clave]; !ok {
								t.Errorf("%s recibe una variable ajena: %s", nombre, clave)
							}
						}
						for clave, esperado := range esperadas[nombre] {
							// La salida canónica de config escapa $ como $$ para poder reutilizarse como Compose.
							if actual, ok := servicio.Environment[clave]; !ok || strings.ReplaceAll(actual, "$$", "$") != esperado {
								t.Errorf("%s no conserva el valor configurado de %s (valores omitidos)", nombre, clave)
							}
						}
						if len(servicio.EnvFile) > 0 {
							t.Error("el servicio conserva un archivo de entorno completo")
						}
					})
				}
			})
		}
	}
}
