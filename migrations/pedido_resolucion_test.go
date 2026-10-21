//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/shared/testdb"
	"github.com/pressly/goose/v3"
)

func TestPedidoResolucion_MigracionConDatosYRollback(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)
	if err := testdb.AplicarHasta(ctx, dsn, 18); err != nil {
		t.Fatal(err)
	}
	ejecutar(ctx, t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Ada','Docente','ada@test.local','hash','DOCENTE','APROBADA')`, idDocente)
	ejecutar(ctx, t, pool, `INSERT INTO ciclo_lectivo(id,anio) VALUES($1,2026)`, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division) VALUES($1,$2,1,'A')`, idCurso, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,'Robótica')`, idMateria, idCurso)
	const pendiente = "aaaaaaaa-1111-1111-1111-111111111111"
	const aprobada = "aaaaaaaa-2222-2222-2222-222222222222"
	const rechazada = "aaaaaaaa-3333-3333-3333-333333333333"
	const existente = "aaaaaaaa-4444-4444-4444-444444444444"
	ejecutar(ctx, t, pool, `INSERT INTO pedido_de_materia(id,usuario_id,materia_solicitada,motivo,estado,resuelto_en) VALUES($1,$4,'Robótica','Cargo nuevo','PENDIENTE',NULL),($2,$4,'Robótica','Cargo nuevo','APROBADO',now()),($3,$4,'Robótica','Cargo nuevo','RECHAZADO',now())`, pendiente, aprobada, rechazada, idDocente)
	ejecutar(ctx, t, pool, `INSERT INTO pedido_de_materia(id,usuario_id,materia_id,motivo,estado,resuelto_en) VALUES($1,$2,$3,'Segundo turno','APROBADO',now())`, existente, idDocente, idMateria)
	var antes, despues string
	if err := pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(p) ORDER BY id)::text FROM pedido_de_materia p`).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	if err := testdb.AplicarHasta(ctx, dsn, 19); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(p)-'materia_creada_id' ORDER BY id)::text FROM pedido_de_materia p`).Scan(&despues); err != nil {
		t.Fatal(err)
	}
	if antes != despues {
		t.Fatal("la actualización cambió los pedidos existentes")
	}
	var vinculados int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM pedido_de_materia WHERE materia_creada_id IS NOT NULL`).Scan(&vinculados); err != nil || vinculados != 0 {
		t.Fatalf("se inventaron vínculos históricos: %d %v", vinculados, err)
	}
	for _, id := range []string{pendiente, rechazada, existente} {
		if _, err := pool.Exec(ctx, `UPDATE pedido_de_materia SET materia_creada_id=$2 WHERE id=$1`, id, idMateria); err == nil {
			t.Fatalf("se permitió un vínculo incompatible: %s", id)
		}
	}
	ejecutar(ctx, t, pool, `UPDATE pedido_de_materia SET materia_creada_id=$2 WHERE id=$1`, aprobada, idMateria)
	// Eliminar la materia conserva la solicitud de creación y su resolución.
	ejecutar(ctx, t, pool, `DELETE FROM materia WHERE id=$1`, idMateria)
	var estado, texto string
	var vinculo *string
	if err := pool.QueryRow(ctx, `SELECT estado,materia_solicitada,materia_creada_id FROM pedido_de_materia WHERE id=$1`, aprobada).Scan(&estado, &texto, &vinculo); err != nil || estado != "APROBADO" || texto != "Robótica" || vinculo != nil {
		t.Fatalf("se perdió la constancia al eliminar la materia: %s %s %v", estado, texto, err)
	}
	if err := pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(p)-'materia_creada_id' ORDER BY id)::text FROM pedido_de_materia p`).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.DownContext(ctx, db, "."); err != nil {
		t.Fatalf("bajando 019: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(p) ORDER BY id)::text FROM pedido_de_materia p`).Scan(&despues); err != nil {
		t.Fatal(err)
	}
	if antes != despues {
		t.Fatal("la bajada perdió datos de la solicitud")
	}
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatalf("reaplicando 019: %v", err)
	}
}
