//go:build integration

// Package testdb da a los tests de integración de cada paquete una única
// forma de construir el esquema.
//
// Lee el directorio completo en vez de nombrar los archivos, para que un
// cambio de esquema no requiera acordarse de tocar los ocho harnesses de
// integración. Si alguno quedara apuntando a un archivo viejo, sus tests
// pasarían en verde mientras producción corre otra cosa — la peor forma de
// fallar. (Pasó: un harness nombraba `001_init.sql` y sobrevivió hasta que
// ese archivo dejó de existir; ahí falló ruidosamente, que es lo bueno de no
// tener alternativa silenciosa.)
package testdb

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SQLDeMigraciones devuelve el contenido de `migrations/` concatenado en
// orden lexicográfico, que es el mismo orden en que lo aplica
// docker-entrypoint-initdb.d en producción. Hoy es un solo archivo con el
// esquema completo; el orden importa el día que haya más de uno.
//
// dirRelativo es la ruta al directorio migrations/ desde el paquete que
// llama (típicamente "../../../migrations").
func SQLDeMigraciones(dirRelativo string) (string, error) {
	entradas, err := os.ReadDir(dirRelativo)
	if err != nil {
		return "", fmt.Errorf("leyendo el directorio de migraciones: %w", err)
	}

	var nombres []string
	for _, e := range entradas {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			nombres = append(nombres, e.Name())
		}
	}
	if len(nombres) == 0 {
		return "", fmt.Errorf("no se encontró ninguna migración en %s", dirRelativo)
	}
	sort.Strings(nombres)

	var sb strings.Builder
	for _, n := range nombres {
		contenido, err := os.ReadFile(filepath.Join(dirRelativo, n))
		if err != nil {
			return "", fmt.Errorf("leyendo %s: %w", n, err)
		}
		sb.Write(contenido)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
