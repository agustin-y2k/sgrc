package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	// El driver de database/sql que necesita goose.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/ramiro/sgrc/migrations"
)

// El esquema lo aplica el propio binario al arrancar, con goose.

// timeoutMigraciones acota el arranque: si la base no responde o alguien dejó
// una transacción abierta sobre las tablas, el contenedor tiene que fallar
// con un mensaje claro en vez de quedarse colgado para siempre en un estado
// donde el healthcheck tampoco dice nada útil.
const timeoutMigraciones = 2 * time.Minute

// aplicarMigraciones deja la base en la última versión del esquema.
func aplicarMigraciones(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("abriendo la conexión para migrar: %w", err)
	}
	defer db.Close()

	if err := configurarGoose(); err != nil {
		return err
	}

	ctx, cancelar := context.WithTimeout(ctx, timeoutMigraciones)
	defer cancelar()

	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("aplicando migraciones: %w", err)
	}
	return nil
}

// configurarGoose lo apunta al esquema embebido en el binario y unifica su
// salida con la del resto del arranque, para que las líneas de la migración
// no aparezcan con otro formato en medio del log.
func configurarGoose() error {
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(log.Default())
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("configurando goose: %w", err)
	}
	return nil
}

func esInvocacionDeMigrate(args []string) bool {
	return len(args) > 1 && args[1] == "migrate"
}

// ejecutarMigrate atiende `sgrc-app migrate status` y `sgrc-app migrate up`,
// para poder mirar y forzar el estado del esquema sin levantar la aplicación.
func ejecutarMigrate(args []string, dsn string) int {
	accion := "status"
	if len(args) > 2 {
		accion = args[2]
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no se pudo abrir la conexión: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := configurarGoose(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	ctx, cancelar := context.WithTimeout(context.Background(), timeoutMigraciones)
	defer cancelar()

	switch accion {
	case "status":
		err = goose.StatusContext(ctx, db, ".")
	case "up":
		err = goose.UpContext(ctx, db, ".")
	default:
		fmt.Fprintf(os.Stderr, "acción desconocida: %q. Uso: sgrc-app migrate [status|up]\n", accion)
		return 2
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	return 0
}
