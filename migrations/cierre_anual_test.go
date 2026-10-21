//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/shared/testdb"
	"github.com/pressly/goose/v3"
)

func TestCierreAnual_MigracionConDatosYRollback(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)
	if err := testdb.AplicarHasta(ctx, dsn, 17); err != nil {
		t.Fatal(err)
	}
	ejecutar(ctx, t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Ada','Docente','ada@test.local','hash','DOCENTE','APROBADA')`, idDocente)
	ejecutar(ctx, t, pool, `INSERT INTO ciclo_lectivo(id,anio,archivado) VALUES($1,2026,true)`, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division) VALUES($1,$2,1,'A')`, idCurso, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,'Matemática')`, idMateria, idCurso)
	ejecutar(ctx, t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,'Notebook','Computadora')`, idEquipo)
	ejecutar(ctx, t, pool, `INSERT INTO reserva_grupo(id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,'Ada Docente','2026-05-04','08:00','09:00')`, idGrupo, idMateria, idDocente)
	ejecutar(ctx, t, pool, `INSERT INTO reserva(id,reserva_grupo_id,equipo_id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,$4,$5,'Ada Docente','2026-05-04','08:00','09:00')`, idReserva, idGrupo, idEquipo, idMateria, idDocente)
	// Un cierre legado puede conservar un resumen más completo que sus reservas.
	// Los homónimos sin cuenta se mantienen como filas distintas.
	ejecutar(ctx, t, pool, `INSERT INTO historico_uso_docente(anio,usuario_id,nombre_docente_snapshot,cantidad_reservas,minutos_totales) VALUES(2026,$1,'Ada Docente',50,3000),(2026,NULL,'Alex Igual',2,120),(2026,NULL,'Alex Igual',3,180)`, idDocente)
	ejecutar(ctx, t, pool, `INSERT INTO historico_uso_equipo(anio,equipo_id,etiqueta_snapshot,minutos_reservados,cantidad_reservas) VALUES(2026,$1,'Notebook',3300,55)`, idEquipo)
	var antes string
	if err := pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(h) ORDER BY id)::text FROM historico_uso_docente h`).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	if err := testdb.AplicarHasta(ctx, dsn, 18); err != nil {
		t.Fatal(err)
	}
	var despues, identidad string
	var legado bool
	if err := pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(h)-'docente_id_historico' ORDER BY id)::text FROM historico_uso_docente h`).Scan(&despues); err != nil {
		t.Fatal(err)
	}
	if antes != despues {
		t.Fatalf("la migración modificó el histórico legado: %s / %s", antes, despues)
	}
	if err := pool.QueryRow(ctx, `SELECT legado FROM snapshot_ciclo WHERE ciclo_id=$1 AND anio=2026`, idCiclo).Scan(&legado); err != nil || !legado {
		t.Fatalf("histórico anterior sin sello: %v / %v", legado, err)
	}
	if err := pool.QueryRow(ctx, `SELECT docente_id_historico FROM reserva WHERE id=$1`, idReserva).Scan(&identidad); err != nil || identidad != idDocente {
		t.Fatalf("identidad no migrada: %s %v", identidad, err)
	}
	var personas int
	if err := pool.QueryRow(ctx, `SELECT COUNT(DISTINCT docente_id_historico) FROM historico_uso_docente`).Scan(&personas); err != nil || personas != 3 {
		t.Fatalf("se fusionaron personas: %d %v", personas, err)
	}
	// ON DELETE SET NULL sigue funcionando sobre reservas de un año sellado.
	ejecutar(ctx, t, pool, `DELETE FROM usuario WHERE id=$1`, idDocente)
	if err := pool.QueryRow(ctx, `SELECT docente_id_historico FROM reserva WHERE id=$1`, idReserva).Scan(&identidad); err != nil || identidad != idDocente {
		t.Fatalf("se perdió identidad al eliminar cuenta: %s %v", identidad, err)
	}
	var cantidad, minutos int
	if err := pool.QueryRow(ctx, `SELECT SUM(cantidad_reservas),SUM(minutos_totales) FROM historico_uso_docente`).Scan(&cantidad, &minutos); err != nil || cantidad != 55 || minutos != 3300 {
		t.Fatalf("se perdió uso: %d / %d / %v", cantidad, minutos, err)
	}
	// La bajada restaura el contrato anterior y conserva todas las filas.
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownContext(ctx, db, "."); err != nil {
		t.Fatalf("bajando 018: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT SUM(cantidad_reservas),SUM(minutos_totales) FROM historico_uso_docente`).Scan(&cantidad, &minutos); err != nil || cantidad != 55 || minutos != 3300 {
		t.Fatalf("la bajada perdió datos: %d / %d / %v", cantidad, minutos, err)
	}
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatalf("reaplicando 018: %v", err)
	}
}
