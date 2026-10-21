//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/shared/testdb"
	"github.com/pressly/goose/v3"
)

func TestVersionesReservas_MigracionPreservaDatosYRollback(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)
	if err := testdb.AplicarHasta(ctx, dsn, 20); err != nil {
		t.Fatal(err)
	}
	ejecutar(ctx, t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Ada','Docente','ada@test.local','hash','DOCENTE','APROBADA')`, idDocente)
	ejecutar(ctx, t, pool, `INSERT INTO ciclo_lectivo(id,anio) VALUES($1,2026)`, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division) VALUES($1,$2,1,'A')`, idCurso, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,'Robótica')`, idMateria, idCurso)
	ejecutar(ctx, t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,'Notebook','Computadora')`, idEquipo)
	ejecutar(ctx, t, pool, `INSERT INTO reserva_grupo(id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,'Ada','2026-05-04','08:00','09:00')`, idGrupo, idMateria, idDocente)
	ejecutar(ctx, t, pool, `INSERT INTO reserva(id,reserva_grupo_id,equipo_id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin,estado,motivo_cancelacion,cancelada_en) VALUES($1,$2,$3,$4,$5,'Ada','2026-05-04','08:00','09:00','CANCELADA','Legada',now())`, idReserva, idGrupo, idEquipo, idMateria, idDocente)
	const datos = `SELECT jsonb_build_object('reserva',(SELECT to_jsonb(r)-'version' FROM reserva r WHERE id=$1),'grupo',(SELECT to_jsonb(g)-'version' FROM reserva_grupo g WHERE id=$2))::text`
	var antes, despues string
	if err := pool.QueryRow(ctx, datos, idReserva, idGrupo).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	if err := testdb.AplicarHasta(ctx, dsn, 21); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, datos, idReserva, idGrupo).Scan(&despues); err != nil || antes != despues {
		t.Fatalf("la migración cambió datos: %v", err)
	}
	var vr, vg int64
	if err := pool.QueryRow(ctx, `SELECT (SELECT version FROM reserva WHERE id=$1),(SELECT version FROM reserva_grupo WHERE id=$2)`, idReserva, idGrupo).Scan(&vr, &vg); err != nil || vr != 0 || vg != 0 {
		t.Fatalf("versiones legadas incorrectas: %d/%d %v", vr, vg, err)
	}
	for _, tabla := range []string{"reserva", "reserva_grupo"} {
		if _, err := pool.Exec(ctx, `UPDATE `+tabla+` SET version=-1`); err == nil {
			t.Fatalf("versión negativa en %s", tabla)
		}
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, datos, idReserva, idGrupo).Scan(&despues); err != nil || antes != despues {
		t.Fatalf("la bajada perdió datos: %v", err)
	}
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatal(err)
	}
}
