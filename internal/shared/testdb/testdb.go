//go:build integration

// Package testdb da a los tests de integración de cada paquete una única
// forma de construir el esquema: la misma que usa el binario al arrancar.
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/ramiro/sgrc/migrations"
)

// loggerSilencioso descarta el "OK 001_esquema_inicial.sql" que goose imprime
// por cada migración aplicada.
type loggerSilencioso struct{}

func (loggerSilencioso) Printf(string, ...any) {}

func (loggerSilencioso) Fatalf(formato string, v ...any) { log.Fatalf(formato, v...) }

// AplicarEsquema deja la base del contenedor en la última versión del
// esquema. dsn es el connection string que devuelve testcontainers.
func AplicarEsquema(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("abriendo la conexión de test: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(loggerSilencioso{})
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("configurando goose: %w", err)
	}

	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("aplicando el esquema: %w", err)
	}
	return nil
}
