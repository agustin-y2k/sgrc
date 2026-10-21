//go:build integration

package main

import (
	"context"
	"fmt"
	"testing"

	academicdomain "github.com/agustin-y2k/sgrc/internal/academic/domain"
	academicinfra "github.com/agustin-y2k/sgrc/internal/academic/infrastructure"
	"github.com/google/uuid"
)

func TestAsignacionesMultiples_SoloElUltimoCancelaLaMateriaDelCurso(t *testing.T) {
	pool := baseVacia(t)
	e := prepararBajaReal(t, pool, false)
	ctx := context.Background()
	svc := servicioAsignacionesReal(pool, e, academicinfra.NewValidadorUsuarioPostgres(pool))
	materia := e.materias[0]
	vinculos := []string{vinculoReal(t, pool, e.usuario, materia)}
	for i, rol := range []academicdomain.RolDocente{academicdomain.RolTitular, academicdomain.RolSuplente, academicdomain.RolSuplente} {
		id := uuid.NewString()
		ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Docente','Prueba',$2,'hash','DOCENTE','APROBADA')`, id, fmt.Sprintf("asignado%d@revision.test", i))
		dm, err := svc.AsignarDocente(ctx, materia, id, rol)
		if err != nil {
			t.Fatal(err)
		}
		vinculos = append(vinculos, dm.ID)
	}
	comprobarCantidad(t, pool, 2, `SELECT count(*) FROM docente_materia WHERE materia_id=$1 AND rol='TITULAR'`, materia)
	comprobarCantidad(t, pool, 2, `SELECT count(*) FROM docente_materia WHERE materia_id=$1 AND rol='SUPLENTE'`, materia)

	// El mismo nombre en otra división y en otro año no comparte docentes ni
	// cancelaciones. Sus reservas deben sobrevivir incluso a la última baja.
	var ajenas []string
	for _, curso := range []struct {
		anio     int
		division string
	}{{1, "B"}, {2, "A"}} {
		cursoID, materiaID, equipo, grupo, reserva := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		ejecutarCierreSQL(t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division) SELECT $1,c.ciclo_lectivo_id,$2,$3 FROM materia m JOIN curso c ON c.id=m.curso_id WHERE m.id=$4`, cursoID, curso.anio, curso.division, materia)
		ejecutarCierreSQL(t, pool, `INSERT INTO materia(id,curso_id,nombre) SELECT $1,$2,nombre FROM materia WHERE id=$3`, materiaID, cursoID, materia)
		if _, err := svc.AsignarDocente(ctx, materiaID, e.usuario, academicdomain.RolTitular); err != nil {
			t.Fatal(err)
		}
		ejecutarCierreSQL(t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,$2,'Computadora')`, equipo, "Notebook "+reserva)
		ejecutarCierreSQL(t, pool, `INSERT INTO reserva_grupo(id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,'Docente Prueba','2026-05-04','08:00','09:00')`, grupo, materiaID, e.usuario)
		ejecutarCierreSQL(t, pool, `INSERT INTO reserva(id,reserva_grupo_id,equipo_id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,$4,$5,'Docente Prueba','2026-05-04','08:00','09:00')`, reserva, grupo, equipo, materiaID, e.usuario)
		ajenas = append(ajenas, reserva)
	}
	for i, id := range vinculos {
		n, err := svc.RemoverDocenteMateria(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		esperadas, estado := 0, "CONFIRMADA"
		if i == len(vinculos)-1 {
			esperadas, estado = 1, "CANCELADA"
		}
		if n != esperadas || e.reservaAvisos.Load() != int32(esperadas) {
			t.Fatalf("baja %d: canceladas=%d avisos=%d, se esperaban %d", i, n, e.reservaAvisos.Load(), esperadas)
		}
		comprobarCantidad(t, pool, len(vinculos)-i-1, `SELECT count(*) FROM docente_materia WHERE materia_id=$1`, materia)
		comprobarCantidad(t, pool, 1, `SELECT count(*) FROM reserva WHERE id=$1 AND estado=$2`, e.reservas[0], estado)
		for _, ajena := range append(ajenas, e.reservas[1]) {
			comprobarCantidad(t, pool, 1, `SELECT count(*) FROM reserva WHERE id=$1 AND estado='CONFIRMADA'`, ajena)
		}
	}
}
