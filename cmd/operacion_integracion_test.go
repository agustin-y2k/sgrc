//go:build integration

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/agustin-y2k/sgrc/internal/shared/secretos"
	"github.com/agustin-y2k/sgrc/internal/shared/testdb"
)

// La base destino empieza vacía, sin migraciones. Se restaura con pg_dump y
// psql reales, igual que en operación, y nunca usa DATABASE_URL del entorno.
func postgresOperacional(t *testing.T, ctx context.Context) (*postgres.PostgresContainer, *pgxpool.Pool, string) {
	t.Helper()
	c, err := postgres.Run(ctx, "postgres:16-alpine", postgres.WithDatabase("sgrc_test"), postgres.WithUsername("sgrc_test"), postgres.WithPassword("sgrc_test"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Terminate(context.Background()); err != nil {
			t.Logf("limpieza: %v", err)
		}
	})
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return c, pool, dsn
}

func ejecutarOperacional(t *testing.T, ctx context.Context, c *postgres.PostgresContainer, comando ...string) {
	t.Helper()
	codigo, salida, err := c.Exec(ctx, comando)
	if err != nil {
		t.Fatal(err)
	}
	datos, err := io.ReadAll(salida)
	if err != nil || codigo != 0 {
		t.Fatalf("comando %v: salida=%d, error=%v, detalle=%s", comando, codigo, err, datos)
	}
}

func huellasBase(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string][32]byte {
	t.Helper()
	filas, err := pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`)
	if err != nil {
		t.Fatal(err)
	}
	var tablas []string
	for filas.Next() {
		var tabla string
		if err := filas.Scan(&tabla); err != nil {
			t.Fatal(err)
		}
		tablas = append(tablas, tabla)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		t.Fatal(err)
	}
	resultado := make(map[string][32]byte)
	for _, tabla := range tablas {
		var datos string
		query := `SELECT coalesce(jsonb_agg(to_jsonb(f) ORDER BY to_jsonb(f)::text),'[]'::jsonb)::text FROM ` + pgx.Identifier{"public", tabla}.Sanitize() + ` f`
		if err := pool.QueryRow(ctx, query).Scan(&datos); err != nil {
			t.Fatal(err)
		}
		resultado[tabla] = sha256.Sum256([]byte(datos))
	}
	for nombre, query := range map[string]string{
		"_indices":  `SELECT coalesce(jsonb_agg(indexdef ORDER BY indexname),'[]'::jsonb)::text FROM pg_indexes WHERE schemaname='public'`,
		"_triggers": `SELECT coalesce(jsonb_agg(pg_get_triggerdef(t.oid) ORDER BY c.relname,t.tgname),'[]'::jsonb)::text FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND NOT t.tgisinternal`,
	} {
		var datos string
		if err := pool.QueryRow(ctx, query).Scan(&datos); err != nil {
			t.Fatal(err)
		}
		resultado[nombre] = sha256.Sum256([]byte(datos))
	}
	resultado["_restricciones"] = huellaRestricciones(t, ctx, pool)
	return resultado
}

func huellaRestricciones(t *testing.T, ctx context.Context, pool *pgxpool.Pool) [32]byte {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	filas, err := tx.Query(ctx, `SELECT c.relname,k.conname,pg_get_constraintdef(k.oid),k.convalidated,k.connoinherit FROM pg_constraint k JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' ORDER BY c.relname,k.conname`)
	if err != nil {
		t.Fatal(err)
	}
	type restriccion struct {
		Tabla, Nombre, Definicion string
		Validada, NoHeredada      bool
	}
	var restricciones []restriccion
	for filas.Next() {
		var r restriccion
		if err := filas.Scan(&r.Tabla, &r.Nombre, &r.Definicion, &r.Validada, &r.NoHeredada); err != nil {
			t.Fatal(err)
		}
		restricciones = append(restricciones, r)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		t.Fatal(err)
	}
	for i := range restricciones {
		r := &restricciones[i]
		if !strings.HasPrefix(r.Definicion, "CHECK ") {
			continue
		}
		// pg_dump puede pasar el cast varchar[]→text[] a cada literal del
		// ARRAY. Volver a parsear AMBAS expresiones con PostgreSQL evita una
		// diferencia textual falsa, conservando íntegra la condición CHECK.
		expresion := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(r.Definicion, "CHECK "), " NOT VALID"), " NO INHERIT")
		if _, err := tx.Exec(ctx, `CREATE TEMP VIEW sgrc_check_canonico AS SELECT `+expresion+` AS valido FROM `+pgx.Identifier{"public", r.Tabla}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(ctx, `SELECT pg_get_viewdef('pg_temp.sgrc_check_canonico'::regclass)`).Scan(&r.Definicion); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `DROP VIEW pg_temp.sgrc_check_canonico`); err != nil {
			t.Fatal(err)
		}
	}
	datos, err := json.Marshal(restricciones)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(datos)
}

func TestOperacionBackend_UpgradeBackupRestoreYReinicio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	origen, pool, dsn := postgresOperacional(t, ctx)
	if err := testdb.AplicarHasta(ctx, dsn, 21); err != nil {
		t.Fatal(err)
	}
	d := prepararCierre(t, pool)
	admin := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Admin','Restore','restore@test.local','hash','ADMIN','APROBADA')`, admin)
	for i := range 2 {
		ejecutarCierreSQL(t, pool, `INSERT INTO codigo_recuperacion(usuario_id,codigo_hash,creado_en,expira_en) VALUES($1,'hash','2026-05-04'::date+$2::integer*interval '1 hour','2026-05-05')`, d.docente, i)
	}
	for range 2 {
		hilo := uuid.NewString()
		ejecutarCierreSQL(t, pool, `INSERT INTO sugerencia(id,usuario_id,tipo,asunto) VALUES($1,$2,'AYUDA','Consulta legada')`, hilo, d.docente)
		ejecutarCierreSQL(t, pool, `INSERT INTO sugerencia_mensaje(sugerencia_id,autor_id,de_admin,texto) VALUES($1,$2,false,'Texto legado')`, hilo, d.docente)
	}
	ejecutarCierreSQL(t, pool, `INSERT INTO notificacion(usuario_id,sobre_usuario_id,mensaje,tipo) VALUES($1,$2,'Aviso legado','SUGERENCIA')`, admin, d.docente)
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM codigo_recuperacion WHERE usado_en IS NULL`)
	comprobarCantidad(t, pool, 2, `SELECT count(*) FROM notificacion WHERE hilo_id IS NOT NULL AND estado='NO_LEIDA'`)
	academico, _ := serviciosCierre(pool, "")
	nuevoAnio := 2027
	if _, err := academico.ArchivarYClonar(ctx, d.ciclo, &nuevoAnio); err != nil {
		t.Fatal(err)
	}
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM cierre_ciclo`)
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM snapshot_ciclo`)
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM historico_uso_equipo`)
	bus := busDurableReal(t, pool)
	hilo, err := sugerenciasDurables(pool, bus).Escribir(ctx, d.docente, "AYUDA", "Pendiente a restaurar", "Prueba del outbox", "", "")
	if err != nil {
		t.Fatal(err)
	}
	clave, err := secretos.Nuevo("clave-estable-de-cuentas-para-restauracion")
	if err != nil {
		t.Fatal(err)
	}
	cifrado, err := clave.Cifrar("password-sintetica-del-equipo")
	if err != nil {
		t.Fatal(err)
	}
	ejecutarCierreSQL(t, pool, `INSERT INTO equipo_cuenta(equipo_id,usuario,clase,privilegio,tiene_password,password_cifrada) VALUES($1,'Alumno','Local','COMUN',true,$2)`, d.equipo, cifrado)
	foto, err := os.ReadFile("../internal/auth/domain/testdata/foto-lossless.webp")
	if err != nil {
		t.Fatal(err)
	}
	ejecutarCierreSQL(t, pool, `INSERT INTO foto_de_perfil(usuario_id,contenido,tipo) VALUES($1,$2,'image/webp')`, d.docente, foto)
	huella := huellasBase(t, ctx, pool)
	ejecutarOperacional(t, ctx, origen, "sh", "-ec", `umask 077; pg_dump -U sgrc_test -d sgrc_test --no-owner --no-privileges > /tmp/respaldo.sql`)
	lector, err := origen.CopyFileFromContainer(ctx, "/tmp/respaldo.sql")
	if err != nil {
		t.Fatal(err)
	}
	backup, err := io.ReadAll(lector)
	_ = lector.Close()
	if err != nil || len(backup) == 0 {
		t.Fatalf("backup vacío: %v", err)
	}
	destino, restaurada, dsnRestaurada := postgresOperacional(t, ctx)
	if err := destino.CopyToContainer(ctx, backup, "/tmp/respaldo.sql", 0600); err != nil {
		t.Fatal(err)
	}
	ejecutarOperacional(t, ctx, destino, "psql", "-X", "-v", "ON_ERROR_STOP=1", "--single-transaction", "-U", "sgrc_test", "-d", "sgrc_test", "-f", "/tmp/respaldo.sql")
	if obtenida := huellasBase(t, ctx, restaurada); !reflect.DeepEqual(huella, obtenida) {
		for tabla, h := range huella {
			if obtenida[tabla] != h {
				t.Errorf("restauración cambió %s", tabla)
			}
		}
		t.Fatal("las huellas de datos/esquema difieren")
	}
	// El arranque vuelve a aplicar migraciones: una restauración actualizada
	// debe permanecer idéntica y mantener el resultado idempotente del cierre.
	if err := testdb.AplicarEsquema(ctx, dsnRestaurada); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(huella, huellasBase(t, ctx, restaurada)) {
		t.Fatal("el reinicio cambió la base restaurada")
	}
	academico, _ = serviciosCierre(restaurada, "")
	if _, err := academico.ArchivarYClonar(ctx, d.ciclo, &nuevoAnio); err != nil {
		t.Fatal(err)
	}
	comprobarCantidad(t, restaurada, 1, `SELECT count(*) FROM cierre_ciclo`)
	comprobarCantidad(t, restaurada, 2, `SELECT count(*) FROM ciclo_lectivo`)
	var guardado string
	if err := restaurada.QueryRow(ctx, `SELECT password_cifrada FROM equipo_cuenta WHERE equipo_id=$1`, d.equipo).Scan(&guardado); err != nil {
		t.Fatal(err)
	}
	if texto, err := clave.Descifrar(guardado); err != nil || texto != "password-sintetica-del-equipo" {
		t.Fatalf("cuenta restaurada no descifra: %v", err)
	}
	reiniciado := busDurableReal(t, restaurada)
	if n, err := reiniciado.Despachar(ctx, 50); err != nil || n != 1 {
		t.Fatalf("evento restaurado: entregados=%d error=%v", n, err)
	}
	if n, err := reiniciado.Despachar(ctx, 50); err != nil || n != 0 {
		t.Fatalf("evento duplicado: entregados=%d error=%v", n, err)
	}
	comprobarCantidad(t, restaurada, 1, `SELECT count(*) FROM notificacion WHERE hilo_id=$1 AND usuario_id=$2 AND estado='NO_LEIDA'`, hilo.ID, admin)
	comprobarCantidad(t, restaurada, 0, `SELECT count(*) FROM evento_pendiente WHERE procesado_en IS NULL`)
	// Verifica que una secuencia restaurada siga avanzando y que los triggers
	// vuelvan a auditar las escrituras posteriores al restore.
	if _, err := sugerenciasDurables(restaurada, reiniciado).Escribir(ctx, d.docente, "AYUDA", "Después del restore", "Texto", "", ""); err != nil {
		t.Fatal(err)
	}
	var antes int
	if err := restaurada.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entidad='equipo' AND entidad_id=$1`, d.equipo).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	ejecutarCierreSQL(t, restaurada, `UPDATE equipo SET nombre='Nombre posterior al restore' WHERE id=$1`, d.equipo)
	comprobarCantidad(t, restaurada, antes+1, `SELECT count(*) FROM audit_log WHERE entidad='equipo' AND entidad_id=$1`, d.equipo)
	t.Logf("upgrade 021→026 y restore PostgreSQL 16: %d tablas/estructuras comparadas; histórico, foto, cuenta cifrada y outbox verificados", len(huella))
}
