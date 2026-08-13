//go:build integration

// Package testdb da a los tests de integración de cada paquete una única
// forma de construir el esquema: la misma que usa el binario al arrancar.
//
// Eso último es el punto. Antes esto leía los .sql del directorio y los
// ejecutaba concatenados, que era parecido a lo que hacía producción pero no
// idéntico — y un test que construye su base de una forma distinta a la real
// deja de avisar justo cuando el esquema cambia. Hoy los dos caminos son la
// misma llamada a goose sobre los mismos archivos embebidos.
//
// De paso desapareció la ruta relativa ("../../../migrations") que cada
// harness tenía que acertar. Ya pasó una vez que un harness nombrara un
// archivo que había dejado de existir; falló ruidosamente, que fue lo bueno,
// pero ahora no hay ruta que equivocar.
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

// loggerSilencioso descarta el "OK 001_esquema_inicial.sql" que goose
// imprime por cada migración aplicada. Cada test de integración levanta su
// propio contenedor y migra de cero, así que esas líneas se multiplican por
// cientos y tapan lo único que importa mirar, que es qué test falló.
//
// Fatalf no se silencia: si goose decide que algo es fatal, tiene que verse.
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
