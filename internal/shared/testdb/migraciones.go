//go:build integration

// Package testdb da a los tests de integración de cada paquete una única
// forma de construir el esquema.
//
// Antes cada archivo de test leía "migrations/001_init.sql" a mano. Eso
// funcionó mientras hubo una sola migración, pero al agregar la segunda los
// siete harnesses habrían seguido probando contra un esquema viejo — los
// tests pasarían en verde mientras producción corre otra cosa, que es la
// peor forma de fallar. Leer el directorio completo hace que agregar una
// migración no requiera acordarse de tocar nada más.
package testdb

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SQLDeMigraciones devuelve el contenido de todas las migraciones
// concatenadas en orden lexicográfico (001_, 002_, …), que es el mismo
// orden en que las aplica docker-entrypoint-initdb.d en producción.
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
