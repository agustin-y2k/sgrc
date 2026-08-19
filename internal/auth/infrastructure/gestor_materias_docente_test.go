//go:build integration

package infrastructure

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// crearMateriaDeTest arma ciclo→curso→materia mínimos por SQL directo —
// auth/infrastructure no importa academic, así que no puede usar su domain
// para esto.
var contadorAnioDeTest int32

func crearMateriaDeTest(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	anio := int(atomic.AddInt32(&contadorAnioDeTest, 1)) + 3000
	cicloID := NuevoID()
	cursoID := NuevoID()
	materiaID := NuevoID()

	// activo=false a propósito: este fixture puede crearse varias veces por test
	// (una por materia independiente), y solo puede haber UN ciclo activo a la
	// vez en toda la tabla (idx_ciclo_lectivo_activo_unico, RF-02.1) — estos
	// tests no necesitan que el ciclo esté activo para nada, así que evitamos
	// esa constraint directamente.
	if _, err := pool.Exec(ctx, `INSERT INTO ciclo_lectivo (id, anio, activo) VALUES ($1, $2, false)`, cicloID, anio); err != nil {
		t.Fatalf("no se pudo crear ciclo de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO curso (id, ciclo_lectivo_id, nombre) VALUES ($1, $2, '1°A')`, cursoID, cicloID); err != nil {
		t.Fatalf("no se pudo crear curso de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO materia (id, curso_id, nombre) VALUES ($1, $2, 'Matemáticas')`, materiaID, cursoID); err != nil {
		t.Fatalf("no se pudo crear materia de prueba: %v", err)
	}
	return materiaID
}

func asignarDocenteDeTest(t *testing.T, pool *pgxpool.Pool, usuarioID, materiaID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO docente_materia (id, usuario_id, materia_id, rol) VALUES ($1, $2, $3, 'TITULAR')`,
		NuevoID(), usuarioID, materiaID)
	if err != nil {
		t.Fatalf("no se pudo asignar docente de prueba: %v", err)
	}
}

func crearUsuarioDeTestGestor(t *testing.T, pool *pgxpool.Pool, estado string) string {
	t.Helper()
	id := NuevoID()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO usuario (id, nombre, apellido, email, password_hash, rol, estado)
		VALUES ($1, 'Ada', 'Lovelace', $2, 'hash-de-prueba', 'DOCENTE', $3)
	`, id, id+"@escuela.edu.ar", estado)
	if err != nil {
		t.Fatalf("no se pudo crear usuario de prueba: %v", err)
	}
	return id
}

func TestGestorMateriasDocentePostgres_MateriasDeDocente(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	docenteID := crearUsuarioDeTestGestor(t, pool, "APROBADA")
	materia1 := crearMateriaDeTest(t, pool)
	materia2 := crearMateriaDeTest(t, pool)
	asignarDocenteDeTest(t, pool, docenteID, materia1)
	asignarDocenteDeTest(t, pool, docenteID, materia2)

	gestor := NewGestorMateriasDocentePostgres(pool)
	materias, err := gestor.MateriasDeDocente(context.Background(), docenteID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(materias) != 2 {
		t.Fatalf("esperaba 2 materias, obtuve %d", len(materias))
	}
}

func TestGestorMateriasDocentePostgres_MateriasDeDocente_SinAsignaciones(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	docenteID := crearUsuarioDeTestGestor(t, pool, "APROBADA")

	gestor := NewGestorMateriasDocentePostgres(pool)
	materias, err := gestor.MateriasDeDocente(context.Background(), docenteID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(materias) != 0 {
		t.Errorf("esperaba 0 materias, obtuve %d", len(materias))
	}
}

func TestGestorMateriasDocentePostgres_QuedaOtroDocenteActivo_True(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	materiaID := crearMateriaDeTest(t, pool)
	docente1 := crearUsuarioDeTestGestor(t, pool, "APROBADA")
	docente2 := crearUsuarioDeTestGestor(t, pool, "APROBADA")
	asignarDocenteDeTest(t, pool, docente1, materiaID)
	asignarDocenteDeTest(t, pool, docente2, materiaID)

	gestor := NewGestorMateriasDocentePostgres(pool)
	quedaOtro, err := gestor.QuedaOtroDocenteActivo(context.Background(), materiaID, docente1)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !quedaOtro {
		t.Error("esperaba quedaOtro=true (docente2 sigue APROBADA en la misma materia)")
	}
}

func TestGestorMateriasDocentePostgres_QuedaOtroDocenteActivo_False_EraElUnico(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	materiaID := crearMateriaDeTest(t, pool)
	docente1 := crearUsuarioDeTestGestor(t, pool, "APROBADA")
	asignarDocenteDeTest(t, pool, docente1, materiaID)

	gestor := NewGestorMateriasDocentePostgres(pool)
	quedaOtro, err := gestor.QuedaOtroDocenteActivo(context.Background(), materiaID, docente1)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if quedaOtro {
		t.Error("esperaba quedaOtro=false, era el único docente")
	}
}

func TestGestorMateriasDocentePostgres_QuedaOtroDocenteActivo_False_OtroNoAprobado(t *testing.T) {
	// Caso importante: si el "otro" docente existe pero no está en estado
	// APROBADA (ej.
	pool := levantarPostgresDeTest(t)
	materiaID := crearMateriaDeTest(t, pool)
	docente1 := crearUsuarioDeTestGestor(t, pool, "APROBADA")
	docente2 := crearUsuarioDeTestGestor(t, pool, "BAJA")
	asignarDocenteDeTest(t, pool, docente1, materiaID)
	asignarDocenteDeTest(t, pool, docente2, materiaID)

	gestor := NewGestorMateriasDocentePostgres(pool)
	quedaOtro, err := gestor.QuedaOtroDocenteActivo(context.Background(), materiaID, docente1)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if quedaOtro {
		t.Error("un docente en BAJA no debería contar como 'otro docente activo'")
	}
}

func TestGestorMateriasDocentePostgres_RemoverAsignacionesDeDocente(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	docenteID := crearUsuarioDeTestGestor(t, pool, "APROBADA")
	materia1 := crearMateriaDeTest(t, pool)
	materia2 := crearMateriaDeTest(t, pool)
	asignarDocenteDeTest(t, pool, docenteID, materia1)
	asignarDocenteDeTest(t, pool, docenteID, materia2)

	gestor := NewGestorMateriasDocentePostgres(pool)
	if err := gestor.RemoverAsignacionesDeDocente(context.Background(), docenteID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	materiasRestantes, err := gestor.MateriasDeDocente(context.Background(), docenteID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(materiasRestantes) != 0 {
		t.Errorf("esperaba 0 asignaciones tras remover, obtuve %d", len(materiasRestantes))
	}
}

func TestGestorMateriasDocentePostgres_IDInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	gestor := NewGestorMateriasDocentePostgres(pool)

	_, err1 := gestor.MateriasDeDocente(context.Background(), "USUARIO_ID")
	_, err2 := gestor.QuedaOtroDocenteActivo(context.Background(), "MATERIA_ID", "USUARIO_ID")
	err3 := gestor.RemoverAsignacionesDeDocente(context.Background(), "USUARIO_ID")

	if err1 == nil || err2 == nil || err3 == nil {
		t.Fatal("esperaba error de ID inválido en los tres casos")
	}
}
