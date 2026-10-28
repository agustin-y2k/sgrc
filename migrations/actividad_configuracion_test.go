//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/agustin-y2k/sgrc/internal/shared/testdb"
)

func TestActividadConfiguracion_ActualizacionPreservaLegado(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)
	if err := testdb.AplicarHasta(ctx, dsn, 34); err != nil {
		t.Fatal(err)
	}
	ejecutar(ctx, t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,'PC prueba','Computadora')`, idEquipo)
	ejecutar(ctx, t, pool, `INSERT INTO ciclo_lectivo(id,anio) VALUES($1,2026)`, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division) VALUES($1,$2,1,'1')`, idCurso, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,'Materia')`, idMateria, idCurso)
	ejecutar(ctx, t, pool, `INSERT INTO reserva_grupo(id,materia_id,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,'Docente','2026-05-04','08:00','09:00')`, idGrupo, idMateria)
	ejecutar(ctx, t, pool, `INSERT INTO reserva(id,reserva_grupo_id,equipo_id,materia_id,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,$4,'Docente','2026-05-04','08:00','09:00')`, idReserva, idGrupo, idEquipo, idMateria)
	vinculado, desvinculado := uuid.NewString(), uuid.NewString()
	ejecutar(ctx, t, pool, `INSERT INTO prestamo(id,equipo_id,reserva_id,entregado_a_nombre,entregado_en,devuelto_en) VALUES($1,$2,$3,'Docente','2026-05-04 08:00Z','2026-05-04 09:00Z'),($4,$2,NULL,'Docente','2026-05-05 08:00Z','2026-05-05 09:00Z')`, vinculado, idEquipo, idReserva, desvinculado)
	ejecutar(ctx, t, pool, `INSERT INTO snapshot_ciclo(ciclo_id,anio) VALUES($1,2026)`, idCiclo)
	var antes, despues string
	const custodia = `SELECT jsonb_agg(to_jsonb(p)-'origen' ORDER BY id)::text FROM prestamo p`
	if err := pool.QueryRow(ctx, custodia).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, custodia).Scan(&despues); err != nil || antes != despues {
		t.Fatal("se modificó la custodia durante la actualización")
	}
	for _, tc := range []struct{ id, origen string }{{vinculado, "CON_RESERVA"}, {desvinculado, "NO_DETERMINADO"}} {
		var origen string
		if err := pool.QueryRow(ctx, `SELECT origen FROM prestamo WHERE id=$1`, tc.id).Scan(&origen); err != nil || origen != tc.origen {
			t.Fatalf("origen=%s %v", origen, err)
		}
	}
	nuevo := uuid.NewString()
	ejecutar(ctx, t, pool, `INSERT INTO prestamo(id,equipo_id,reserva_id,entregado_a_nombre,devuelto_en) VALUES($1,$2,$3,'Docente',now())`, nuevo, idEquipo, idReserva)
	var origenNuevo string
	if err := pool.QueryRow(ctx, `SELECT origen FROM prestamo WHERE id=$1`, nuevo).Scan(&origenNuevo); err != nil || origenNuevo != "CON_RESERVA" {
		t.Fatal("no conservó la procedencia explícita de una inserción")
	}
	if _, err := pool.Exec(ctx, `UPDATE prestamo SET origen='SIN_RESERVA' WHERE id=$1`, nuevo); err == nil {
		t.Fatal("se pudo reescribir el origen de una entrega")
	}
	var falta bool
	if err := pool.QueryRow(ctx, `SELECT actividad IS NULL FROM snapshot_ciclo WHERE ciclo_id=$1`, idCiclo).Scan(&falta); err != nil || !falta {
		t.Fatal("recalculó el histórico anterior")
	}
}
