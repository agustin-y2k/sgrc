//go:build integration

package migrations_test

import (
	"context"
	"github.com/agustin-y2k/sgrc/internal/shared/testdb"
	"github.com/google/uuid"
	"testing"
)

func TestEspaciosYBloqueos_MigracionesConservanIdentidadesYReparanFaltantes(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)
	if err := testdb.AplicarHasta(ctx, dsn, 31); err != nil {
		t.Fatal(err)
	}
	ciclo, espacio, roto, interna, adicional := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	ejecutar(ctx, t, pool, `INSERT INTO ciclo_lectivo(id,anio) VALUES($1,2026)`, ciclo)
	ejecutar(ctx, t, pool, `INSERT INTO espacio(id,ciclo_lectivo_id,nombre) VALUES($1,$3,'Biblioteca'),($2,$3,'Dirección')`, espacio, roto, ciclo)
	ejecutar(ctx, t, pool, `INSERT INTO materia(id,espacio_id,nombre) VALUES($1,$3,'biblioteca'),($2,$3,'Apoyo escolar')`, interna, adicional, espacio)
	equipos := []string{uuid.NewString(), uuid.NewString()}
	reservas := []string{uuid.NewString(), uuid.NewString()}
	for i, equipo := range equipos {
		ejecutar(ctx, t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,$2,'Computadora')`, equipo, "Computadora "+equipo)
		ejecutar(ctx, t, pool, `INSERT INTO reserva(id,equipo_id,fecha,hora_inicio,hora_fin,tipo,motivo_bloqueo) VALUES($1,$2,'2026-11-02','08:00','09:00','BLOQUEO',$3)`, reservas[i], equipo, []string{"Mantenimiento", "Evaluación"}[i])
	}
	const datos = `SELECT jsonb_agg(to_jsonb(r) ORDER BY id)::text FROM reserva r`
	var antes, despues string
	if err := pool.QueryRow(ctx, datos).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(r)-'bloqueo_operacion_id' ORDER BY id)::text FROM reserva r`).Scan(&despues); err != nil || antes != despues {
		t.Fatalf("se alteraron reservas históricas: %v", err)
	}
	var marcadas, reparadas int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM materia WHERE id=$1 AND interna_de_espacio AND nombre='Biblioteca'`, interna).Scan(&marcadas); err != nil || marcadas != 1 {
		t.Fatalf("no conservó identidad: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM materia WHERE espacio_id=$1 AND interna_de_espacio AND nombre='Dirección'`, roto).Scan(&reparadas); err != nil || reparadas != 1 {
		t.Fatalf("no reparó faltante: %v", err)
	}
	var nombre string
	var flag bool
	if err := pool.QueryRow(ctx, `SELECT nombre,interna_de_espacio FROM materia WHERE id=$1`, adicional).Scan(&nombre, &flag); err != nil || nombre != "Apoyo escolar" || flag {
		t.Fatal("modificó materia adicional")
	}
	var cantidad int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM reserva WHERE bloqueo_operacion_id=id`).Scan(&cantidad); err != nil || cantidad != 2 {
		t.Fatal("fusionó operaciones históricas")
	}
	if _, err := pool.Exec(ctx, `UPDATE reserva SET bloqueo_operacion_id=$1 WHERE id=$2`, reservas[0], reservas[1]); err == nil {
		t.Fatal("permitió cambiar identidad histórica")
	}
}
