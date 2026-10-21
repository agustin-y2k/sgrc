//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/shared/testdb"
	"github.com/pressly/goose/v3"
)

func TestBajaUsuario_MigracionLegadaYRollbackProtegido(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)
	if err := testdb.AplicarHasta(ctx, dsn, 19); err != nil {
		t.Fatal(err)
	}
	ejecutar(ctx, t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Ada','Docente','ada@test.local','hash','ADMIN','BAJA')`, idDocente)
	ejecutar(ctx, t, pool, `INSERT INTO ciclo_lectivo(id,anio) VALUES($1,2026)`, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division) VALUES($1,$2,1,'A')`, idCurso, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,'Robótica')`, idMateria, idCurso)
	ejecutar(ctx, t, pool, `INSERT INTO docente_materia(usuario_id,materia_id,rol) VALUES($1,$2,'TITULAR')`, idDocente, idMateria)
	var antes, despues string
	const datos = `SELECT jsonb_build_object('usuario',(SELECT to_jsonb(u) FROM usuario u WHERE id=$1),'asignaciones',(SELECT jsonb_agg(to_jsonb(dm) ORDER BY id) FROM docente_materia dm WHERE usuario_id=$1))::text`
	if err := pool.QueryRow(ctx, datos, idDocente).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	if err := testdb.AplicarHasta(ctx, dsn, 20); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, datos, idDocente).Scan(&despues); err != nil || antes != despues {
		t.Fatalf("cambió datos existentes: %v", err)
	}
	var procesos, materias int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM baja_usuario WHERE usuario_id=$1 AND completada_en IS NULL),(SELECT count(*) FROM baja_usuario_materia WHERE usuario_id=$1 AND materia_id=$2 AND completada_en IS NULL)`, idDocente, idMateria).Scan(&procesos, &materias); err != nil || procesos != 1 || materias != 1 {
		t.Fatalf("progreso legado incorrecto: %d/%d %v", procesos, materias, err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownContext(ctx, db, "."); err == nil {
		t.Fatal("descartó una baja pendiente")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM baja_usuario_materia WHERE usuario_id=$1`, idDocente).Scan(&materias); err != nil || materias != 1 {
		t.Fatalf("rollback perdió el pendiente: %d %v", materias, err)
	}
	ejecutar(ctx, t, pool, `UPDATE baja_usuario_materia SET completada_en=now() WHERE usuario_id=$1`, idDocente)
	ejecutar(ctx, t, pool, `DELETE FROM docente_materia WHERE usuario_id=$1`, idDocente)
	ejecutar(ctx, t, pool, `UPDATE baja_usuario SET completada_en=now() WHERE usuario_id=$1`, idDocente)
	if err := goose.DownContext(ctx, db, "."); err != nil {
		t.Fatalf("bajando 020 completada: %v", err)
	}
	var tabla *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('baja_usuario')::text`).Scan(&tabla); err != nil || tabla != nil {
		t.Fatalf("no eliminó la tabla: %v %v", tabla, err)
	}
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatalf("reaplicando 020: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM baja_usuario`).Scan(&procesos); err != nil || procesos != 0 {
		t.Fatalf("inventó tareas sin asignaciones: %d %v", procesos, err)
	}
}
