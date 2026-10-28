//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/shared/testdb"
	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func TestContextoPrestamos_MigracionReconstruyeSoloVinculosExistentes(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)
	if err := testdb.AplicarHasta(ctx, dsn, 30); err != nil {
		t.Fatal(err)
	}
	ejecutar(ctx, t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,'Computadora','Computadora')`, idEquipo)
	ejecutar(ctx, t, pool, `INSERT INTO ciclo_lectivo(id,anio) VALUES($1,2026)`, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division,modalidad) VALUES($1,$2,3,'2','Construcción')`, idCurso, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,'Dibujo')`, idMateria, idCurso)
	ejecutar(ctx, t, pool, `INSERT INTO reserva_grupo(id,materia_id,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,'Docente','2026-05-04','08:00','09:00')`, idGrupo, idMateria)
	ejecutar(ctx, t, pool, `INSERT INTO reserva(id,reserva_grupo_id,equipo_id,materia_id,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,$4,'Docente','2026-05-04','08:00','09:00')`, idReserva, idGrupo, idEquipo, idMateria)
	abierto, devuelto, sinReserva := uuid.NewString(), uuid.NewString(), uuid.NewString()
	ejecutar(ctx, t, pool, `INSERT INTO prestamo(id,equipo_id,reserva_id,entregado_a_nombre,entregado_en,devuelto_en,destino)
 VALUES($1,$2,$3,'Docente','2026-05-04 08:00Z','2026-05-04 08:15Z','Destino escrito'),($4,$2,$3,'Docente','2026-05-04 08:30Z',NULL,NULL),($5,$2,NULL,'Otra persona','2026-05-03 08:00Z','2026-05-03 09:00Z','Dibujo de 3°2')`, devuelto, idEquipo, idReserva, abierto, sinReserva)
	const datos = `SELECT jsonb_agg(to_jsonb(p)-ARRAY['destino','origen'] ORDER BY id)::text FROM prestamo p`
	var antes, despues string
	if err := pool.QueryRow(ctx, datos).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, datos).Scan(&despues); err != nil || antes != despues {
		t.Fatal("la migración modificó la custodia o sus fechas")
	}
	var cantidad int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM prestamo_contexto_academico WHERE reconstruido AND materia_nombre='Dibujo' AND curso_anio=3 AND curso_division='2' AND modalidad='Construcción' AND ciclo_anio=2026 AND reserva_id_original=$1`, idReserva).Scan(&cantidad); err != nil || cantidad != 2 {
		t.Fatalf("reconstrucción: %d %v", cantidad, err)
	}
	for _, tc := range []struct{ id, destino string }{{abierto, "3°2 · Construcción"}, {devuelto, "Destino escrito"}, {sinReserva, "Dibujo de 3°2"}} {
		var destino string
		if err := pool.QueryRow(ctx, `SELECT destino FROM prestamo WHERE id=$1`, tc.id).Scan(&destino); err != nil || destino != tc.destino {
			t.Fatalf("destino modificado: %s %v", destino, err)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM prestamo_contexto_academico WHERE prestamo_id=$1`, sinReserva).Scan(&cantidad); err != nil || cantidad != 0 {
		t.Fatal("inventó un contexto a partir del texto libre")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownToContext(ctx, db, ".", 30); err == nil {
		t.Fatal("el rollback eliminaría contextos históricos")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM prestamo_contexto_academico`).Scan(&cantidad); err != nil || cantidad != 2 {
		t.Fatal("el rollback fallido alteró los contextos")
	}
	// Sin datos históricos, el cambio de esquema sigue siendo reversible.
	ejecutar(ctx, t, pool, `DELETE FROM prestamo_contexto_academico`)
	if err := goose.DownToContext(ctx, db, ".", 30); err != nil {
		t.Fatal(err)
	}
	var tabla *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('prestamo_contexto_academico')::text`).Scan(&tabla); err != nil || tabla != nil {
		t.Fatal("la tabla vacía no se pudo revertir")
	}
}
