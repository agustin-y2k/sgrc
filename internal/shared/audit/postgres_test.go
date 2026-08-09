//go:build integration

package audit

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ramiro/sgrc/internal/shared/testdb"
)

func levantarPostgresDeTest(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	// Por el helper compartido y no leyendo un archivo por nombre: este
	// harness apuntaba a `001_init.sql` y se rompió cuando el esquema pasó a
	// estar en un solo archivo. Nombrar el archivo hace que un test quede
	// construyendo un esquema viejo sin que nada lo avise — el modo de falla
	// que testdb existe para evitar.
	sqlEsquema, err := testdb.SQLDeMigraciones("../../../migrations")
	if err != nil {
		t.Fatalf("no se pudo leer el esquema: %v", err)
	}

	contenedor, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("sgrc_test"),
		postgres.WithUsername("sgrc_test"),
		postgres.WithPassword("sgrc_test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("no se pudo levantar el contenedor de Postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := contenedor.Terminate(context.Background()); err != nil {
			t.Logf("advertencia: no se pudo terminar el contenedor limpiamente: %v", err)
		}
	})

	connStr, err := contenedor.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("no se pudo obtener el connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("no se pudo conectar al pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, sqlEsquema); err != nil {
		t.Fatalf("no se pudo aplicar la migración: %v", err)
	}

	return pool
}

func crearUsuarioDeTest(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO usuario (id, nombre, apellido, email, password_hash, rol, estado)
		VALUES ($1, 'Ada', 'Lovelace', $2, 'hash-de-prueba', 'ADMIN', 'APROBADA')
	`, id, id+"@escuela.edu.ar")
	if err != nil {
		t.Fatalf("no se pudo crear usuario de prueba: %v", err)
	}
	return id
}

func TestPostgresAuditor_Registrar_ConDetalleYEntidadID_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	auditor := NewPostgresAuditor(pool)
	adminID := crearUsuarioDeTest(t, pool)
	entidadID := uuid.NewString()

	err := auditor.Registrar(context.Background(), Entrada{
		UsuarioID: adminID,
		Accion:    CuentaBaja,
		Entidad:   "usuario",
		EntidadID: &entidadID,
		Detalle:   map[string]any{"motivo": "prueba"},
		IPOrigen:  "203.0.113.7",
	})
	if err != nil {
		t.Fatalf("error inesperado registrando auditoría: %v", err)
	}

	var accion, entidad, ipTexto string
	var detalleTexto string
	row := pool.QueryRow(context.Background(), `
		SELECT accion, entidad, detalle::text, host(ip_origen)
		FROM audit_log WHERE usuario_id = $1 AND entidad_id = $2
	`, adminID, entidadID)
	if err := row.Scan(&accion, &entidad, &detalleTexto, &ipTexto); err != nil {
		t.Fatalf("no se pudo leer la fila insertada: %v", err)
	}

	if accion != CuentaBaja || entidad != "usuario" {
		t.Fatalf("fila insertada con datos inesperados: accion=%q entidad=%q", accion, entidad)
	}
	if ipTexto != "203.0.113.7" {
		t.Fatalf("ip_origen inesperada: %q", ipTexto)
	}
}

func TestPostgresAuditor_Registrar_SinDetalleNiIP_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	auditor := NewPostgresAuditor(pool)
	adminID := crearUsuarioDeTest(t, pool)

	err := auditor.Registrar(context.Background(), Entrada{
		UsuarioID: adminID,
		Accion:    AdminCreado,
		Entidad:   "usuario",
	})
	if err != nil {
		t.Fatalf("error inesperado con detalle/IP vacíos: %v", err)
	}

	var count int
	row := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log WHERE usuario_id = $1 AND accion = $2 AND entidad_id IS NULL
	`, adminID, AdminCreado)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("no se pudo contar filas: %v", err)
	}
	if count != 1 {
		t.Fatalf("esperaba 1 fila sin entidad_id, encontré %d", count)
	}
}
