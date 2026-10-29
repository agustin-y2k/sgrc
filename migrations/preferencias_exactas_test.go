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

func TestPreferenciasExactas_MigracionPreservaMarcasYEvitaAmpliarlasAlRevertir(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)
	if err := testdb.AplicarHasta(ctx, dsn, 28); err != nil {
		t.Fatal(err)
	}
	ejecutar(ctx, t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES ($1,'Notebook','Computadora')`, idEquipo)
	marca := uuid.NewString()
	ejecutar(ctx, t, pool, `INSERT INTO equipo_preferencia(id,equipo_id,materia_nombre,anio,prioridad) VALUES ($1,$2,'Matemática',3,2)`, marca, idEquipo)
	var antes, despues string
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(ep)::text FROM equipo_preferencia ep WHERE id=$1`, marca).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	if err := testdb.AplicarHasta(ctx, dsn, 29); err != nil {
		t.Fatal(err)
	}
	var exacta bool
	if err := pool.QueryRow(ctx, `SELECT (to_jsonb(ep)-'curso_exacto')::text,curso_exacto FROM equipo_preferencia ep WHERE id=$1`, marca).Scan(&despues, &exacta); err != nil {
		t.Fatal(err)
	}
	if antes != despues || exacta {
		t.Fatalf("se alteró una marca anterior: %s / %s, exacta=%v", antes, despues, exacta)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownContext(ctx, db, "."); err != nil {
		t.Fatalf("rollback sin nuevas marcas: %v", err)
	}
	if err := testdb.AplicarHasta(ctx, dsn, 29); err != nil {
		t.Fatal(err)
	}
	nueva := uuid.NewString()
	ejecutar(ctx, t, pool, `INSERT INTO equipo_preferencia(id,equipo_id,materia_nombre,anio,prioridad,curso_exacto) VALUES ($1,$2,'Matemática',3,1,true)`, nueva, idEquipo)
	if err := goose.DownContext(ctx, db, "."); err == nil {
		t.Fatal("revertir ampliaría el alcance de un curso concreto a todas las modalidades")
	}
	if err := pool.QueryRow(ctx, `SELECT curso_exacto FROM equipo_preferencia WHERE id=$1`, nueva).Scan(&exacta); err != nil || !exacta {
		t.Fatalf("rollback fallido alteró la marca: %v, %v", exacta, err)
	}
}
