//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"

	"github.com/agustin-y2k/sgrc/internal/shared/testdb"
)

func TestHallazgosPendientes_ActualizacionConDatosYAuditoriaReversible(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)
	if err := testdb.AplicarHasta(ctx, dsn, 21); err != nil {
		t.Fatal(err)
	}
	ejecutar(ctx, t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Admin','Uno','admin@test.local','hash','ADMIN','APROBADA'),($2,'Ada','Docente','ada@test.local','hash','DOCENTE','APROBADA')`, idAdmin, idDocente)
	codigos := []string{uuid.NewString(), uuid.NewString()}
	for i, id := range codigos {
		ejecutar(ctx, t, pool, `INSERT INTO codigo_recuperacion(id,usuario_id,codigo_hash,creado_en,expira_en) VALUES($1,$2,'hash','2026-05-04'::date + $3::integer * interval '1 hour','2026-05-05')`, id, idDocente, i)
	}
	for range 2 {
		ejecutar(ctx, t, pool, `INSERT INTO pedido_de_materia(id,usuario_id,materia_solicitada,motivo) VALUES($1,$2,$3,'Motivo')`, uuid.NewString(), idDocente, uuid.NewString())
		hilo := uuid.NewString()
		ejecutar(ctx, t, pool, `INSERT INTO sugerencia(id,usuario_id,tipo,asunto) VALUES($1,$2,'AYUDA','Consulta')`, hilo, idDocente)
		ejecutar(ctx, t, pool, `INSERT INTO sugerencia_mensaje(id,sugerencia_id,autor_id,de_admin,texto) VALUES($1,$2,$3,false,'Texto')`, uuid.NewString(), hilo, idDocente)
	}
	legacy := []string{uuid.NewString(), uuid.NewString()}
	for i, tipo := range []string{"PEDIDO_DE_MATERIA", "SUGERENCIA"} {
		ejecutar(ctx, t, pool, `INSERT INTO notificacion(id,usuario_id,sobre_usuario_id,mensaje,tipo) VALUES($1,$2,$3,'Aviso legado',$4)`, legacy[i], idAdmin, idDocente, tipo)
	}
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	contar := func(esperados int, query string, args ...any) {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, query, args...).Scan(&n); err != nil || n != esperados {
			t.Fatalf("cantidad=%d, esperada=%d: %v; %s", n, esperados, err, query)
		}
	}
	contar(1, `SELECT count(*) FROM codigo_recuperacion WHERE usado_en IS NULL AND id=$1`, codigos[1])
	contar(1, `SELECT count(*) FROM codigo_recuperacion WHERE usado_en IS NOT NULL AND id=$1`, codigos[0])
	contar(2, `SELECT count(*) FROM notificacion WHERE id=ANY($1::uuid[]) AND estado='LEIDA'`, legacy)
	contar(2, `SELECT count(*) FROM notificacion WHERE pedido_id IS NOT NULL AND estado='NO_LEIDA'`)
	contar(2, `SELECT count(*) FROM notificacion WHERE hilo_id IS NOT NULL AND estado='NO_LEIDA'`)
	if _, err := pool.Exec(ctx, `INSERT INTO codigo_recuperacion(usuario_id,codigo_hash,expira_en) VALUES($1,'otro',now()+interval '1 hour')`, idDocente); err == nil {
		t.Fatal("permite otro código sin usar")
	}
	ejecutar(ctx, t, pool, `UPDATE usuario SET nombre='Ada nueva' WHERE id=$1`, idDocente)
	contar(1, `SELECT count(*) FROM audit_log WHERE accion='CAMBIO_TRANSACCIONAL' AND entidad_id=$1`, idDocente)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownToContext(ctx, db, ".", 24); err != nil {
		t.Fatal(err)
	}
	contar(1, `SELECT count(*) FROM audit_log WHERE accion='CAMBIO_TRANSACCIONAL' AND entidad_id=$1`, idDocente)
	ejecutar(ctx, t, pool, `UPDATE usuario SET nombre='Ada anterior' WHERE id=$1`, idDocente)
	contar(1, `SELECT count(*) FROM audit_log WHERE accion='CAMBIO_TRANSACCIONAL' AND entidad_id=$1`, idDocente)
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	ejecutar(ctx, t, pool, `UPDATE usuario SET nombre='Ada final' WHERE id=$1`, idDocente)
	contar(2, `SELECT count(*) FROM audit_log WHERE accion='CAMBIO_TRANSACCIONAL' AND entidad_id=$1`, idDocente)
}
