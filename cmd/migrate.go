package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	// El driver de database/sql que necesita goose. El resto de la
	// aplicación habla con Postgres por pgxpool, que es otra API del mismo
	// pgx: este import registra la variante compatible con `sql.Open` y no
	// agrega una dependencia nueva.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/ramiro/sgrc/migrations"
)

// El esquema lo aplica el propio binario al arrancar, con goose.
//
// Antes lo hacía Postgres con los scripts de docker-entrypoint-initdb.d, que
// corren UNA sola vez: cuando el volumen se crea vacío. Sobre una base que ya
// existía no corría nada y no avisaba nada, así que una actualización dejaba
// el binario nuevo hablando con el esquema viejo. Eso se ve como un sistema
// que arranca perfecto y falla con 500 en la primera consulta que toca una
// columna que no está — un error que no menciona ni el esquema ni la
// migración que faltaba. Costó una base entera.
//
// Con goose, cada migración aplicada queda anotada en `goose_db_version`:
// arrancar dos veces no reaplica nada, y una base vieja se pone al día sola.

// timeoutMigraciones acota el arranque: si la base no responde o alguien
// dejó una transacción abierta sobre las tablas, el contenedor tiene que
// fallar con un mensaje claro en vez de quedarse colgado para siempre en un
// estado donde el healthcheck tampoco dice nada útil.
const timeoutMigraciones = 2 * time.Minute

// aplicarMigraciones deja la base en la última versión del esquema. Es
// idempotente: si ya está al día, no hace nada.
//
// No toma un lock: el sistema corre como un único proceso (ver ADR 001), así
// que no hay dos instancias compitiendo por migrar. Si algún día se corre
// más de una réplica, esto necesita el locking de sesión de goose antes que
// cualquier otra cosa.
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
// para poder mirar y forzar el estado del esquema sin levantar la
// aplicación. Mismo recurso que `sgrc-app healthcheck`: en una imagen
// `FROM scratch` el único ejecutable disponible es este binario, así que los
// comandos de operación tienen que vivir acá adentro.
//
// `down` NO está, a propósito: revertir el esquema inicial borra las tablas
// y con ellas los datos. Existe en el archivo de migración porque tiene que
// existir, pero no se llega por accidente desde un comando corto — el mismo
// criterio con el que `docker compose down -v` no tiene atajo en el Makefile.
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
