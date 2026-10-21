package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Ejecuta el target REAL con un docker simulado: una salida parcial de pg_dump
// no debe destruir la última copia buena. No usa Docker ni una base real.
func TestBackup_ConservaCopiaAnteriorAnteFalloOVaciado(t *testing.T) {
	makeBin, err := exec.LookPath("make")
	if err != nil {
		t.Skip("requiere make para probar el target de operación")
	}
	makefile, err := filepath.Abs("../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	for _, caso := range []struct {
		modo  string
		falla bool
	}{{"parcial", true}, {"vacio", true}, {"correcto", false}} {
		t.Run(caso.modo, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			for nombre, script := range map[string]string{
				"date":   "#!/bin/sh\nprintf '2030-01-01\\n'\n",
				"docker": "#!/bin/sh\ncase \"$SGRC_BACKUP_MODO\" in\nparcial) printf 'SQL incompleto\\n'; exit 1;;\nvacio) exit 0;;\ncorrecto) printf 'SQL completo\\n';;\nesac\n",
			} {
				if err := os.WriteFile(filepath.Join(bin, nombre), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
			}
			datos := filepath.Join(dir, "datos con espacios")
			if err := os.Mkdir(datos, 0700); err != nil {
				t.Fatal(err)
			}
			archivo := filepath.Join(datos, "backup-sgrc-2030-01-01.sql")
			const anterior = "copia anterior válida\n"
			if err := os.WriteFile(archivo, []byte(anterior), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(makeBin, "-f", makefile, "backup", "DIR_BACKUPS="+datos)
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "SGRC_BACKUP_MODO="+caso.modo)
			salida, err := cmd.CombinedOutput()
			if (err != nil) != caso.falla {
				t.Fatalf("fallo=%v esperado=%v salida=%s", err, caso.falla, salida)
			}
			contenido, err := os.ReadFile(archivo)
			if err != nil {
				t.Fatal(err)
			}
			esperado := anterior
			if !caso.falla {
				esperado = "SQL completo\n"
			}
			if string(contenido) != esperado {
				t.Fatalf("backup=%q esperado=%q", contenido, esperado)
			}
			info, err := os.Stat(archivo)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0600 {
				t.Fatalf("permisos=%o", info.Mode().Perm())
			}
			entradas, err := os.ReadDir(datos)
			if err != nil {
				t.Fatal(err)
			}
			if len(entradas) != 1 {
				t.Fatalf("temporales sin limpiar: %v", entradas)
			}
			if caso.falla && strings.Contains(string(salida), "Backup en ") {
				t.Fatal("el fallo anunció éxito")
			}
		})
	}
}
