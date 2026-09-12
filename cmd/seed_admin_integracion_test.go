//go:build integration

// Solo con la tag "integration" (go test -tags=integration ./...), porque
// levanta un Postgres real con testcontainers-go y le aplica TODAS las
// migraciones.
//
// Cubre el único camino que ninguna otra prueba cubría: **arrancar sobre una
// base vacía**. El resto de la suite trabaja sobre bases que ya tienen los
// datos que cada test siembra, y el sistema desplegado tiene su Admin creado
// desde el primer día, así que el sembrado inicial no se volvía a ejecutar en
// ningún lado.
//
// Ahí se escondió un error que dejaba al sistema sin poder instalarse: la
// migración 011 quitó `usuario_email_key UNIQUE (email)` por redundante —el
// índice único sobre lower(email) es más fuerte— y el INSERT del Admin inicial
// seguía diciendo `ON CONFLICT (email)`. Sin árbitro, Postgres responde 42P10,
// el sembrado falla y main.go corta con log.Fatal: el contenedor entra en
// crashloop y nunca llega a servir. Pasó los seis checks de CI y una auditoría
// de seguridad, porque para verlo hay que empezar de cero.
package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ramiro/sgrc/internal/shared/testdb"
)

func baseVacia(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

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

	dsn, err := contenedor.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("no se pudo obtener el connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("no se pudo conectar al pool: %v", err)
	}
	t.Cleanup(pool.Close)

	// Las migraciones REALES, todas. Es lo que hace que este test note que una
	// de ellas se llevó puesta una constraint de la que el código dependía.
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatalf("no se pudo aplicar el esquema: %v", err)
	}
	return pool
}

func entorno(valores map[string]string) func(string) string {
	return func(k string) string { return valores[k] }
}

// Una instalación nueva tiene que poder crear su primer Admin. Si esto falla,
// el sistema no arranca: nadie puede entrar a crear a nadie.
func TestSeedAdmin_SobreBaseVacia(t *testing.T) {
	ctx := context.Background()
	pool := baseVacia(t)

	err := seedAdminSiHaceFalta(ctx, pool, entorno(map[string]string{
		"SEED_ADMIN_EMAIL":    "admin@escuela.edu.ar",
		"SEED_ADMIN_PASSWORD": "una.password.larga.2026",
	}))
	if err != nil {
		t.Fatalf("sembrando el admin inicial sobre una base vacía: %v", err)
	}

	var rol, estado string
	if err := pool.QueryRow(ctx,
		`SELECT rol, estado FROM usuario WHERE lower(email) = 'admin@escuela.edu.ar'`,
	).Scan(&rol, &estado); err != nil {
		t.Fatalf("el admin inicial no quedó creado: %v", err)
	}
	if rol != "ADMIN" || estado != "APROBADA" {
		t.Errorf("el admin inicial quedó como %s/%s; se esperaba ADMIN/APROBADA", rol, estado)
	}
}

// El arranque se repite en cada despliegue y en cada reinicio del contenedor.
// La segunda vez no tiene que crear nada ni pisar la contraseña de nadie: si
// lo hiciera, cada `docker compose restart` resetearía la cuenta del Admin.
func TestSeedAdmin_NoHaceNadaSiYaHayAdmin(t *testing.T) {
	ctx := context.Background()
	pool := baseVacia(t)

	env := entorno(map[string]string{
		"SEED_ADMIN_EMAIL":    "admin@escuela.edu.ar",
		"SEED_ADMIN_PASSWORD": "una.password.larga.2026",
	})
	if err := seedAdminSiHaceFalta(ctx, pool, env); err != nil {
		t.Fatalf("primer sembrado: %v", err)
	}

	var hashOriginal string
	if err := pool.QueryRow(ctx,
		`SELECT password_hash FROM usuario WHERE lower(email) = 'admin@escuela.edu.ar'`,
	).Scan(&hashOriginal); err != nil {
		t.Fatalf("leyendo el hash del admin: %v", err)
	}

	if err := seedAdminSiHaceFalta(ctx, pool, env); err != nil {
		t.Fatalf("segundo sembrado: %v", err)
	}

	var hashDespues string
	var cuantos int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM usuario WHERE rol = 'ADMIN'`,
	).Scan(&cuantos); err != nil {
		t.Fatalf("contando admins: %v", err)
	}
	if cuantos != 1 {
		t.Errorf("quedaron %d admins después de dos arranques; se esperaba 1", cuantos)
	}
	if err := pool.QueryRow(ctx,
		`SELECT password_hash FROM usuario WHERE lower(email) = 'admin@escuela.edu.ar'`,
	).Scan(&hashDespues); err != nil {
		t.Fatalf("releyendo el hash: %v", err)
	}
	if hashDespues != hashOriginal {
		t.Error("el segundo arranque pisó la contraseña del Admin existente")
	}
}

// El .env puede traer "Admin@Escuela.edu.ar" y el índice único es sobre
// lower(email): la cuenta tiene que ser LA MISMA, no una segunda.
func TestSeedAdmin_NoDuplicaPorLaCajaDelCorreo(t *testing.T) {
	ctx := context.Background()
	pool := baseVacia(t)

	if err := seedAdminSiHaceFalta(ctx, pool, entorno(map[string]string{
		"SEED_ADMIN_EMAIL":    "admin@escuela.edu.ar",
		"SEED_ADMIN_PASSWORD": "una.password.larga.2026",
	})); err != nil {
		t.Fatalf("primer sembrado: %v", err)
	}

	// Se le da de baja para que el sembrado vuelva a intentar crear: así se
	// ejerce el camino del ON CONFLICT, que es el que estaba roto.
	if _, err := pool.Exec(ctx,
		`UPDATE usuario SET estado = 'BAJA' WHERE lower(email) = 'admin@escuela.edu.ar'`,
	); err != nil {
		t.Fatalf("dando de baja al admin: %v", err)
	}

	if err := seedAdminSiHaceFalta(ctx, pool, entorno(map[string]string{
		"SEED_ADMIN_EMAIL":    "Admin@Escuela.edu.ar",
		"SEED_ADMIN_PASSWORD": "otra.password.larga.2026",
	})); err != nil {
		t.Fatalf("segundo sembrado con otra caja: %v", err)
	}

	var cuantos int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM usuario WHERE lower(email) = 'admin@escuela.edu.ar'`,
	).Scan(&cuantos); err != nil {
		t.Fatalf("contando: %v", err)
	}
	if cuantos != 1 {
		t.Errorf("quedaron %d cuentas para el mismo correo; se esperaba 1", cuantos)
	}
}
