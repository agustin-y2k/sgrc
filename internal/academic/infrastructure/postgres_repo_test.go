//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
	"github.com/ramiro/sgrc/internal/shared/testdb"
)

func levantarPostgresDeTest(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	contenedor, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("sgrc_test"),
		postgres.WithUsername("sgrc_test"),
		postgres.WithPassword("sgrc_test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("no se pudo levantar el contenedor de Postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := contenedor.Terminate(context.Background()); err != nil {
			t.Logf("advertencia: no se pudo terminar el contenedor limpiamente: %v", err)
		}
	})

	connStr, err := contenedor.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("no se pudo obtener el connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("no se pudo conectar al pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := testdb.AplicarEsquema(ctx, connStr); err != nil {
		t.Fatalf("no se pudo aplicar el esquema: %v", err)
	}

	return pool
}

// insertarUsuarioDeTest inserta directo por SQL (no vía internal/auth, a
// propósito) — academic no debe depender de auth ni siquiera en los tests.
func insertarUsuarioDeTest(t *testing.T, pool *pgxpool.Pool, estado string) string {
	t.Helper()
	id := uuidNuevo()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO usuario (id, nombre, apellido, email, password_hash, rol, estado)
		VALUES ($1, 'Test', 'Docente', $2, 'hash-de-prueba', 'DOCENTE', $3)
	`, id, id+"@escuela.edu.ar", estado)
	if err != nil {
		t.Fatalf("no se pudo insertar usuario de prueba: %v", err)
	}
	return id
}

func TestPostgresRepo_CrearCicloYBuscarActivo_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	c, err := domain.NuevoCicloLectivo(uuidNuevo(), 2026)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCiclo(ctx, c); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	activo, err := repo.BuscarCicloActivo(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if activo.ID != c.ID {
		t.Errorf("el ciclo activo encontrado no coincide: %+v", activo)
	}
}

func TestPostgresRepo_DosCiclosActivos_ConstraintDeLaBase(t *testing.T) {
	// Confirma que el índice único parcial (idx_ciclo_lectivo_activo_unico)
	// protege esto a nivel de base, no solo en application.Service — última
	// línea de defensa ante una condición de carrera.
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	c1, _ := domain.NuevoCicloLectivo(uuidNuevo(), 2025)
	c2, _ := domain.NuevoCicloLectivo(uuidNuevo(), 2026)

	if err := repo.CrearCiclo(ctx, c1); err != nil {
		t.Fatalf("el primer ciclo activo no debería fallar: %v", err)
	}

	err := repo.CrearCiclo(ctx, c2) // también Activo=true, sin pasar por el chequeo de application
	if err == nil {
		t.Fatal("esperaba que la base rechace un segundo ciclo activo")
	}
}

func TestPostgresRepo_ArchivarCiclo_MarcaCicloCursosYMaterias(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	ciclo, _ := domain.NuevoCicloLectivo(uuidNuevo(), 2026)
	if err := repo.CrearCiclo(ctx, ciclo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	curso, _ := domain.NuevoCurso(uuidNuevo(), ciclo.ID, "1°A")
	if err := repo.CrearCurso(ctx, curso); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	materia, _ := domain.NuevaMateria(uuidNuevo(), curso.ID, "Matemáticas")
	if err := repo.CrearMateria(ctx, materia); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if err := repo.ArchivarCiclo(ctx, ciclo.ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	cicloRecargado, err := repo.BuscarCicloPorID(ctx, ciclo.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !cicloRecargado.Archivado || cicloRecargado.Activo {
		t.Errorf("el ciclo debería quedar archivado y no activo: %+v", cicloRecargado)
	}

	cursoRecargado, err := repo.BuscarCursoPorID(ctx, curso.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !cursoRecargado.Archivado {
		t.Error("el curso debería quedar archivado")
	}

	materiaRecargada, err := repo.BuscarMateriaPorID(ctx, materia.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !materiaRecargada.Archivado {
		t.Error("la materia debería quedar archivada")
	}
}

func TestPostgresRepo_ArchivarCiclo_DosVeces_ErrorNoEncontrado(t *testing.T) {
	// El WHERE archivado=false en el UPDATE hace que la segunda llamada no
	// afecte ninguna fila — se traduce a ErrCicloNoEncontrado, no a un "éxito"
	// silencioso que re-archive algo ya archivado.
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	ciclo, _ := domain.NuevoCicloLectivo(uuidNuevo(), 2026)
	if err := repo.CrearCiclo(ctx, ciclo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.ArchivarCiclo(ctx, ciclo.ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	err := repo.ArchivarCiclo(ctx, ciclo.ID)
	if err != application.ErrCicloNoEncontrado {
		t.Fatalf("esperaba ErrCicloNoEncontrado en el segundo archivado, obtuve %v", err)
	}
}

func TestPostgresRepo_ClonarCicloA_ClonaCursosYMaterias(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	origen, _ := domain.NuevoCicloLectivo(uuidNuevo(), 2025)
	if err := repo.CrearCiclo(ctx, origen); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	curso1, _ := domain.NuevoCurso(uuidNuevo(), origen.ID, "1°A")
	curso2, _ := domain.NuevoCurso(uuidNuevo(), origen.ID, "2°B")
	for _, c := range []*domain.Curso{curso1, curso2} {
		if err := repo.CrearCurso(ctx, c); err != nil {
			t.Fatalf("no debería fallar creando curso: %v", err)
		}
	}

	m1, _ := domain.NuevaMateria(uuidNuevo(), curso1.ID, "Matemáticas")
	m2, _ := domain.NuevaMateria(uuidNuevo(), curso1.ID, "Lengua")
	m3, _ := domain.NuevaMateria(uuidNuevo(), curso2.ID, "Historia")
	for _, m := range []*domain.Materia{m1, m2, m3} {
		if err := repo.CrearMateria(ctx, m); err != nil {
			t.Fatalf("no debería fallar creando materia: %v", err)
		}
	}

	// Archivar antes de clonar (no es un requisito técnico del método,
	// pero refleja el flujo real de application.ArchivarYClonar).
	if err := repo.ArchivarCiclo(ctx, origen.ID); err != nil {
		t.Fatalf("no debería fallar archivando: %v", err)
	}

	nuevoCiclo, _ := domain.NuevoCicloLectivo(uuidNuevo(), 2026)
	cursosClonados, materiasClonadas, err := repo.ClonarCicloA(ctx, origen.ID, nuevoCiclo)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if cursosClonados != 2 {
		t.Errorf("esperaba 2 cursos clonados, obtuve %d", cursosClonados)
	}
	if materiasClonadas != 3 {
		t.Errorf("esperaba 3 materias clonadas, obtuve %d", materiasClonadas)
	}

	cursosNuevoCiclo, err := repo.ListarCursosPorCiclo(ctx, nuevoCiclo.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(cursosNuevoCiclo) != 2 {
		t.Fatalf("esperaba 2 cursos en el ciclo nuevo, obtuve %d", len(cursosNuevoCiclo))
	}
	for _, c := range cursosNuevoCiclo {
		if c.Archivado {
			t.Errorf("un curso recién clonado no debería venir archivado: %+v", c)
		}
	}
}

func TestPostgresRepo_Curso_NombreDuplicadoEnMismoCiclo_Error(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	ciclo, _ := domain.NuevoCicloLectivo(uuidNuevo(), 2026)
	if err := repo.CrearCiclo(ctx, ciclo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	c1, _ := domain.NuevoCurso(uuidNuevo(), ciclo.ID, "1°A")
	c2, _ := domain.NuevoCurso(uuidNuevo(), ciclo.ID, "1°A") // mismo nombre, mismo ciclo

	if err := repo.CrearCurso(ctx, c1); err != nil {
		t.Fatalf("el primero no debería fallar: %v", err)
	}
	err := repo.CrearCurso(ctx, c2)
	if err != application.ErrCursoNombreDuplicado {
		t.Fatalf("esperaba ErrCursoNombreDuplicado, obtuve %v", err)
	}
}

func TestPostgresRepo_EliminarCurso_CascadeAMaterias(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	ciclo, _ := domain.NuevoCicloLectivo(uuidNuevo(), 2026)
	repo.CrearCiclo(ctx, ciclo)
	curso, _ := domain.NuevoCurso(uuidNuevo(), ciclo.ID, "1°A")
	repo.CrearCurso(ctx, curso)
	materia, _ := domain.NuevaMateria(uuidNuevo(), curso.ID, "Matemáticas")
	repo.CrearMateria(ctx, materia)

	if err := repo.EliminarCurso(ctx, curso.ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	_, err := repo.BuscarMateriaPorID(ctx, materia.ID)
	if err != application.ErrMateriaNoEncontrada {
		t.Fatalf("la materia debería haberse eliminado en cascada, obtuve %v", err)
	}
}

func TestPostgresRepo_AsignarYRemoverDocenteMateria_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	ciclo, _ := domain.NuevoCicloLectivo(uuidNuevo(), 2026)
	repo.CrearCiclo(ctx, ciclo)
	curso, _ := domain.NuevoCurso(uuidNuevo(), ciclo.ID, "1°A")
	repo.CrearCurso(ctx, curso)
	materia, _ := domain.NuevaMateria(uuidNuevo(), curso.ID, "Matemáticas")
	repo.CrearMateria(ctx, materia)

	usuarioID := insertarUsuarioDeTest(t, pool, "APROBADA")

	dm := domain.NuevoDocenteMateria(uuidNuevo(), usuarioID, materia.ID, domain.RolTitular)
	if err := repo.AsignarDocente(ctx, dm); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	docentes, err := repo.ListarDocentesDeMateria(ctx, materia.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(docentes) != 1 || docentes[0].UsuarioID != usuarioID {
		t.Fatalf("docentes incorrectos: %+v", docentes)
	}

	if err := repo.RemoverDocenteMateria(ctx, dm.ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	docentesTrasRemover, err := repo.ListarDocentesDeMateria(ctx, materia.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(docentesTrasRemover) != 0 {
		t.Errorf("esperaba 0 docentes tras remover, obtuve %d", len(docentesTrasRemover))
	}
}

func TestPostgresRepo_GuardarDocenteMateria_CambiaSoloElRol(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	ciclo, _ := domain.NuevoCicloLectivo(uuidNuevo(), 2026)
	repo.CrearCiclo(ctx, ciclo)
	curso, _ := domain.NuevoCurso(uuidNuevo(), ciclo.ID, "1°A")
	repo.CrearCurso(ctx, curso)
	materia, _ := domain.NuevaMateria(uuidNuevo(), curso.ID, "Matemáticas")
	repo.CrearMateria(ctx, materia)

	usuarioID := insertarUsuarioDeTest(t, pool, "APROBADA")
	dm := domain.NuevoDocenteMateria(uuidNuevo(), usuarioID, materia.ID, domain.RolTitular)
	if err := repo.AsignarDocente(ctx, dm); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	dm.Rol = domain.RolSuplente
	if err := repo.GuardarDocenteMateria(ctx, dm); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	guardado, err := repo.BuscarDocenteMateria(ctx, dm.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if guardado.Rol != domain.RolSuplente {
		t.Errorf("rol = %s, esperaba SUPLENTE", guardado.Rol)
	}
	// El vínculo sigue siendo el mismo: cambiar el rol no puede ser una baja
	// disfrazada (ver Service.CambiarRolDocente).
	if guardado.UsuarioID != usuarioID || guardado.MateriaID != materia.ID {
		t.Errorf("el vínculo cambió: %+v", guardado)
	}
}

func TestPostgresRepo_GuardarDocenteMateria_NoExiste_Error(t *testing.T) {
	repo := NewPostgresRepo(levantarPostgresDeTest(t))

	err := repo.GuardarDocenteMateria(context.Background(),
		domain.NuevoDocenteMateria(uuidNuevo(), uuidNuevo(), uuidNuevo(), domain.RolSuplente))

	if !errors.Is(err, application.ErrDocenteMateriaNoEncontrado) {
		t.Fatalf("esperaba ErrDocenteMateriaNoEncontrado, obtuve %v", err)
	}
}

func TestValidadorUsuarioPostgres_Aprobado_True(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	usuarioID := insertarUsuarioDeTest(t, pool, "APROBADA")
	validador := NewValidadorUsuarioPostgres(pool)

	valido, err := validador.ExisteYAprobado(context.Background(), usuarioID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !valido {
		t.Error("un usuario APROBADA debería ser válido")
	}
}

func TestValidadorUsuarioPostgres_Pendiente_False(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	usuarioID := insertarUsuarioDeTest(t, pool, "PENDIENTE")
	validador := NewValidadorUsuarioPostgres(pool)

	valido, err := validador.ExisteYAprobado(context.Background(), usuarioID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if valido {
		t.Error("un usuario PENDIENTE no debería ser válido para asignar")
	}
}

func TestValidadorUsuarioPostgres_NoExiste_FalseSinError(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorUsuarioPostgres(pool)

	valido, err := validador.ExisteYAprobado(context.Background(), uuidNuevo())

	if err != nil {
		t.Fatalf("un usuario inexistente no debería ser un error, solo false: %v", err)
	}
	if valido {
		t.Error("un usuario inexistente nunca debería ser válido")
	}
}

// TestPostgresRepo_IDConFormatoInvalido_ErrorControlado reproduce el bug real
// encontrado probando el servidor a mano: pegar un ID que no tiene formato
// UUID (ej.
func TestPostgresRepo_IDConFormatoInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	casos := []struct {
		nombre string
		fn     func() error
	}{
		{"BuscarCicloPorID", func() error { _, err := repo.BuscarCicloPorID(ctx, "CICLO_ID"); return err }},
		{"BuscarCursoPorID", func() error { _, err := repo.BuscarCursoPorID(ctx, "CURSO_ID"); return err }},
		{"BuscarMateriaPorID", func() error { _, err := repo.BuscarMateriaPorID(ctx, "MATERIA_ID"); return err }},
		{"ListarCursosPorCiclo", func() error { _, err := repo.ListarCursosPorCiclo(ctx, "CICLO_ID"); return err }},
		{"ListarMateriasPorCurso", func() error { _, err := repo.ListarMateriasPorCurso(ctx, "CURSO_ID"); return err }},
		{"EliminarCurso", func() error { return repo.EliminarCurso(ctx, "CURSO_ID") }},
		{"EliminarMateria", func() error { return repo.EliminarMateria(ctx, "MATERIA_ID") }},
		{"CrearCurso_CicloInvalido", func() error {
			c, _ := domain.NuevoCurso(uuidNuevo(), "CICLO_ID", "1°A")
			return repo.CrearCurso(ctx, c)
		}},
		{"CrearMateria_CursoInvalido", func() error {
			m, _ := domain.NuevaMateria(uuidNuevo(), "CURSO_ID", "Matemáticas")
			return repo.CrearMateria(ctx, m)
		}},
	}

	for _, c := range casos {
		err := c.fn()
		if err != application.ErrIDInvalido {
			t.Errorf("%s: esperaba application.ErrIDInvalido, obtuve %v", c.nombre, err)
		}
	}
}

// ── RF-04.1: materias reservables ──────────────────────────────────────

func TestListarMateriasReservables_FiltraArchivadasYPorDocente(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	docente := insertarUsuarioDeTest(t, pool, "APROBADA")
	otroDocente := insertarUsuarioDeTest(t, pool, "APROBADA")

	cicloID := uuidNuevo()
	if _, err := pool.Exec(ctx, `INSERT INTO ciclo_lectivo (id, anio, activo) VALUES ($1, 2026, true)`, cicloID); err != nil {
		t.Fatal(err)
	}
	cursoID := uuidNuevo()
	if _, err := pool.Exec(ctx, `INSERT INTO curso (id, ciclo_lectivo_id, nombre) VALUES ($1, $2, '1°A')`, cursoID, cicloID); err != nil {
		t.Fatal(err)
	}

	// Materia del docente, materia de otro, y una archivada.
	mia, ajena, archivada := uuidNuevo(), uuidNuevo(), uuidNuevo()
	for id, nombre := range map[string]string{mia: "Matemáticas", ajena: "Lengua", archivada: "Historia"} {
		if _, err := pool.Exec(ctx, `INSERT INTO materia (id, curso_id, nombre) VALUES ($1, $2, $3)`, id, cursoID, nombre); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE materia SET archivado = true WHERE id = $1`, archivada); err != nil {
		t.Fatal(err)
	}
	for materiaID, usuarioID := range map[string]string{mia: docente, ajena: otroDocente, archivada: docente} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO docente_materia (id, usuario_id, materia_id, rol) VALUES ($1, $2, $3, 'TITULAR')`,
			uuidNuevo(), usuarioID, materiaID); err != nil {
			t.Fatal(err)
		}
	}

	nombres := func(ms []application.MateriaReservable) []string {
		out := make([]string, len(ms))
		for i, m := range ms {
			out[i] = m.MateriaNombre
		}
		return out
	}

	// Docente: solo la suya y no archivada.
	delDocente, err := repo.ListarMateriasReservables(ctx, &docente)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if got := nombres(delDocente); len(got) != 1 || got[0] != "Matemáticas" {
		t.Errorf("un docente solo debe ver sus materias no archivadas, obtuve %v", got)
	}
	if delDocente[0].CursoNombre != "1°A" || delDocente[0].CicloAnio != 2026 {
		t.Errorf("debe traer curso y año resueltos: %+v", delDocente[0])
	}

	// Admin: todas las no archivadas, de cualquier docente.
	todas, err := repo.ListarMateriasReservables(ctx, nil)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if got := nombres(todas); len(got) != 2 {
		t.Errorf("un Admin debe ver las 2 materias no archivadas, obtuve %v", got)
	}
}
