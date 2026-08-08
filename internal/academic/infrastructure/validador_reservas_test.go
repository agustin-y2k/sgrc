//go:build integration

package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
)

// crearMateriaConCursoDeTest arma ciclo→curso→materia mínimos para estos
// tests — academic/infrastructure no importa reservation, así que una
// reserva_grupo se inserta acá directo por SQL (mismos campos que
// reservation usaría, sin depender de su domain).
func crearMateriaConCursoDeTest(t *testing.T, pool *pgxpool.Pool) (cursoID, materiaID string) {
	t.Helper()
	ctx := context.Background()

	ciclo, err := domain.NuevoCicloLectivo(NuevoID(), 2026)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo := NewPostgresRepo(pool)
	if err := repo.CrearCiclo(ctx, ciclo); err != nil {
		t.Fatalf("no se pudo crear ciclo de prueba: %v", err)
	}

	curso, err := domain.NuevoCurso(NuevoID(), ciclo.ID, "1°A")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCurso(ctx, curso); err != nil {
		t.Fatalf("no se pudo crear curso de prueba: %v", err)
	}

	materia, err := domain.NuevaMateria(NuevoID(), curso.ID, "Matemáticas")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearMateria(ctx, materia); err != nil {
		t.Fatalf("no se pudo crear materia de prueba: %v", err)
	}

	return curso.ID, materia.ID
}

func insertarReservaGrupoDeTest(t *testing.T, pool *pgxpool.Pool, materiaID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO reserva_grupo (id, materia_id, nombre_docente_snapshot, fecha, hora_inicio, hora_fin, estado)
		VALUES ($1, $2, 'Ada Lovelace', $3, '08:00', '09:00', 'CONFIRMADA')
	`, NuevoID(), materiaID, time.Now().AddDate(0, 0, 7))
	if err != nil {
		t.Fatalf("no se pudo insertar reserva_grupo de prueba: %v", err)
	}
}

func TestValidadorReservasPostgres_TieneReservasMateria_True(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	_, materiaID := crearMateriaConCursoDeTest(t, pool)
	insertarReservaGrupoDeTest(t, pool, materiaID)

	validador := NewValidadorReservasPostgres(pool)
	tiene, err := validador.TieneReservasMateria(context.Background(), materiaID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !tiene {
		t.Error("esperaba TieneReservasMateria=true")
	}
}

func TestValidadorReservasPostgres_TieneReservasMateria_False(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	_, materiaID := crearMateriaConCursoDeTest(t, pool)

	validador := NewValidadorReservasPostgres(pool)
	tiene, err := validador.TieneReservasMateria(context.Background(), materiaID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if tiene {
		t.Error("esperaba TieneReservasMateria=false, sin ninguna reserva_grupo")
	}
}

func TestValidadorReservasPostgres_TieneReservasCurso_True(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	cursoID, materiaID := crearMateriaConCursoDeTest(t, pool)
	insertarReservaGrupoDeTest(t, pool, materiaID)

	validador := NewValidadorReservasPostgres(pool)
	tiene, err := validador.TieneReservasCurso(context.Background(), cursoID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !tiene {
		t.Error("esperaba TieneReservasCurso=true (vía su materia)")
	}
}

func TestValidadorReservasPostgres_TieneReservasCurso_False(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	cursoID, _ := crearMateriaConCursoDeTest(t, pool)

	validador := NewValidadorReservasPostgres(pool)
	tiene, err := validador.TieneReservasCurso(context.Background(), cursoID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if tiene {
		t.Error("esperaba TieneReservasCurso=false")
	}
}

func TestValidadorReservasPostgres_IDInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorReservasPostgres(pool)

	_, err1 := validador.TieneReservasCurso(context.Background(), "CURSO_ID")
	_, err2 := validador.TieneReservasMateria(context.Background(), "MATERIA_ID")

	if err1 != application.ErrIDInvalido {
		t.Errorf("TieneReservasCurso: esperaba application.ErrIDInvalido, obtuve %v", err1)
	}
	if err2 != application.ErrIDInvalido {
		t.Errorf("TieneReservasMateria: esperaba application.ErrIDInvalido, obtuve %v", err2)
	}
}

// El archivado borra tres cosas y esta comprobación es la que decide si el
// reintento puede terminar lo que faltó. Con las reglas afuera, un fallo
// justo entre borrar los grupos y borrar las reglas dejaba filas que el
// reintento ya no veía.
func TestValidadorReservasPostgres_TieneReservasDeCiclo_VeLasReglasHuerfanas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	v := NewValidadorReservasPostgres(pool)
	ctx := context.Background()

	_, materiaID := crearMateriaConCursoDeTest(t, pool)
	cicloID := cicloDeLaMateria(t, pool, materiaID)

	// Como queda tras un borrado que murió a la mitad: sin grupos, con la
	// regla todavía ahí.
	insertarReglaRecurrenciaDeTest(t, pool, materiaID)

	tiene, err := v.TieneReservasDeCiclo(ctx, cicloID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !tiene {
		t.Error("una regla de recurrencia sin borrar es limpieza pendiente")
	}
}

func TestValidadorReservasPostgres_TieneReservasDeCiclo_LimpioDaFalse(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	v := NewValidadorReservasPostgres(pool)
	ctx := context.Background()

	_, materiaID := crearMateriaConCursoDeTest(t, pool)
	cicloID := cicloDeLaMateria(t, pool, materiaID)

	tiene, err := v.TieneReservasDeCiclo(ctx, cicloID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if tiene {
		t.Error("sin nada colgado no hay limpieza pendiente")
	}
}

func cicloDeLaMateria(t *testing.T, pool *pgxpool.Pool, materiaID string) string {
	t.Helper()
	var cicloID string
	err := pool.QueryRow(context.Background(), `
		SELECT c.ciclo_lectivo_id FROM materia m
		JOIN curso c ON c.id = m.curso_id
		WHERE m.id = $1
	`, materiaID).Scan(&cicloID)
	if err != nil {
		t.Fatalf("no se pudo resolver el ciclo de la materia: %v", err)
	}
	return cicloID
}

func insertarReglaRecurrenciaDeTest(t *testing.T, pool *pgxpool.Pool, materiaID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO regla_recurrencia (id, materia_id, dia_semana, hora_inicio, hora_fin, fecha_inicio, fecha_fin)
		VALUES ($1, $2, 'LUNES', '08:00', '09:00', $3, $4)
	`, NuevoID(), materiaID, time.Now(), time.Now().AddDate(0, 0, 30))
	if err != nil {
		t.Fatalf("no se pudo insertar regla_recurrencia de prueba: %v", err)
	}
}
