//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/shared/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestAnioLectivo_MigracionProtegeNuevasReservasYConservaLegadas(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)
	if err := testdb.AplicarHasta(ctx, dsn, 33); err != nil {
		t.Fatal(err)
	}
	ejecutar(ctx, t, pool, `INSERT INTO ciclo_lectivo(id,anio) VALUES($1,2026)`, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division) VALUES($1,$2,3,'2')`, idCurso, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,'Dibujo')`, idMateria, idCurso)
	ejecutar(ctx, t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,'Computadora','Computadora')`, idEquipo)
	ejecutar(ctx, t, pool, `INSERT INTO reserva_grupo(id,materia_id,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,'Docente','2027-01-04','08:00','09:00')`, idGrupo, idMateria)
	ejecutar(ctx, t, pool, `INSERT INTO reserva(id,reserva_grupo_id,equipo_id,materia_id,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,$4,'2027-01-04','08:00','09:00')`, idReserva, idGrupo, idEquipo, idMateria)
	reglaLegada := uuid.NewString()
	ejecutar(ctx, t, pool, `INSERT INTO regla_recurrencia(id,materia_id,dia_semana,hora_inicio,hora_fin,fecha_inicio,fecha_fin) VALUES($1,$2,'LUNES','08:00','09:00','2026-12-28','2027-01-04')`, reglaLegada, idMateria)
	const datos = `SELECT jsonb_build_object('reserva',(SELECT to_jsonb(r) FROM reserva r WHERE id=$1),'grupo',(SELECT to_jsonb(g) FROM reserva_grupo g WHERE id=$2),'regla',(SELECT to_jsonb(rr) FROM regla_recurrencia rr WHERE id=$3))::text`
	var antes, despues string
	if err := pool.QueryRow(ctx, datos, idReserva, idGrupo, reglaLegada).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, datos, idReserva, idGrupo, reglaLegada).Scan(&despues); err != nil || antes != despues {
		t.Fatalf("alteró el histórico: %v", err)
	}
	ejecutar(ctx, t, pool, `UPDATE reserva SET estado='CANCELADA',motivo_cancelacion='Fuera del ciclo' WHERE id=$1`, idReserva)
	comprobarRechazo := func(q string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, q, args...)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "23514" || pgerr.ConstraintName != "reserva_anio_lectivo" {
			t.Fatalf("se esperaba la restricción anual: %v", err)
		}
	}
	for _, rango := range []struct{ fecha, inicio, fin string }{{"2027-01-01", "08:00", "09:00"}, {"2025-12-31", "23:00", "00:00"}, {"2026-12-31", "23:00", "01:00"}} {
		comprobarRechazo(`INSERT INTO reserva_grupo(id,materia_id,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,'Docente',$3,$4,$5)`, uuid.NewString(), idMateria, rango.fecha, rango.inicio, rango.fin)
		comprobarRechazo(`INSERT INTO reserva(id,reserva_grupo_id,equipo_id,materia_id,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.NewString(), idGrupo, idEquipo, idMateria, rango.fecha, rango.inicio, rango.fin)
	}
	valido := uuid.NewString()
	ejecutar(ctx, t, pool, `INSERT INTO reserva_grupo(id,materia_id,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,'Docente','2026-12-31','23:00','00:00')`, valido, idMateria)
	comprobarRechazo(`UPDATE reserva_grupo SET fecha='2027-01-01' WHERE id=$1`, valido)
	comprobarRechazo(`INSERT INTO regla_recurrencia(id,materia_id,dia_semana,hora_inicio,hora_fin,fecha_inicio,fecha_fin) VALUES($1,$2,'LUNES','08:00','09:00','2026-12-28','2027-01-01')`, uuid.NewString(), idMateria)
	comprobarRechazo(`INSERT INTO regla_recurrencia(id,materia_id,dia_semana,hora_inicio,hora_fin,fecha_inicio,fecha_fin) VALUES($1,$2,'JUEVES','23:00','01:00','2026-12-24','2026-12-31')`, uuid.NewString(), idMateria)
	ejecutar(ctx, t, pool, `INSERT INTO regla_recurrencia(id,materia_id,dia_semana,hora_inicio,hora_fin,fecha_inicio,fecha_fin) VALUES($1,$2,'MIERCOLES','23:00','01:00','2026-12-23','2026-12-31')`, uuid.NewString(), idMateria)
}
