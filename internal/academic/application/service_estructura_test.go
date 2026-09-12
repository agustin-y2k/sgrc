package application

import (
	"context"
	"errors"
	"testing"

	"github.com/ramiro/sgrc/internal/academic/domain"
)

// cicloDePrueba deja un ciclo activo y devuelve su ID.
func cicloDePrueba(r *fakeRepo, id string, anio int) string {
	r.ciclos[id] = &domain.CicloLectivo{ID: id, Anio: anio, Activo: true}
	return id
}

// ── ImportarEstructura ──────────────────────────────────────────────────

func TestImportarEstructura_CreaCursosYMaterias(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	svc := servicioSimple(repo)

	res, err := svc.ImportarEstructura(context.Background(), ciclo, []CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Lengua", "Matemática"}},
		{Anio: 1, Division: "2", Materias: []string{"Lengua", "Matemática"}},
	})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	if res.CursosCreados != 2 || res.MateriasCreadas != 4 {
		t.Errorf("esperaba 2 cursos y 4 materias creadas, obtuve %d y %d",
			res.CursosCreados, res.MateriasCreadas)
	}
	if res.CursosExistentes != 0 || res.MateriasExistentes != 0 {
		t.Errorf("sobre un ciclo vacío no debería haber nada existente, obtuve %d y %d",
			res.CursosExistentes, res.MateriasExistentes)
	}
}

// Importar dos veces el mismo archivo tiene que ser inofensivo: es lo que hace
// que se pueda reintentar sin mirar antes qué entró.
func TestImportarEstructura_EsIdempotente(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	svc := servicioSimple(repo)
	archivo := []CursoConMaterias{{Anio: 1, Division: "1", Materias: []string{"Lengua"}}}

	if _, err := svc.ImportarEstructura(context.Background(), ciclo, archivo); err != nil {
		t.Fatalf("primera importación: %v", err)
	}
	res, err := svc.ImportarEstructura(context.Background(), ciclo, archivo)
	if err != nil {
		t.Fatalf("segunda importación: %v", err)
	}

	if res.CursosCreados != 0 || res.MateriasCreadas != 0 {
		t.Errorf("la segunda pasada no debería crear nada, creó %d cursos y %d materias",
			res.CursosCreados, res.MateriasCreadas)
	}
	if res.CursosExistentes != 1 || res.MateriasExistentes != 1 {
		t.Errorf("esperaba 1 curso y 1 materia ya existentes, obtuve %d y %d",
			res.CursosExistentes, res.MateriasExistentes)
	}
	if len(repo.materias) != 1 {
		t.Errorf("esperaba una sola materia en la base, hay %d", len(repo.materias))
	}
}

// Una planilla escrita a mano trae la misma materia con y sin acento sin que
// nadie lo note. La base compara por nombre normalizado, así que si la
// importación compara por el nombre exacto el curso termina con las dos.
func TestImportarEstructura_NoDuplicaPorAcentoNiMayusculas(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	svc := servicioSimple(repo)

	res, err := svc.ImportarEstructura(context.Background(), ciclo, []CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Matemática", "MATEMATICA", "matematica"}},
	})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if res.MateriasCreadas != 1 {
		t.Errorf("esperaba una sola materia creada, se crearon %d", res.MateriasCreadas)
	}
	if len(repo.materias) != 1 {
		t.Errorf("esperaba una sola materia en la base, hay %d", len(repo.materias))
	}
}

// Dos renglones para el mismo curso es cómo se ve una planilla donde alguien
// partió la lista de materias en dos filas. Se funden, no se rechaza el archivo.
func TestImportarEstructura_FundeFilasDelMismoCurso(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	svc := servicioSimple(repo)

	res, err := svc.ImportarEstructura(context.Background(), ciclo, []CursoConMaterias{
		{Anio: 4, Division: "1", Modalidad: "Electromecánica", Materias: []string{"Física"}},
		{Anio: 4, Division: " 1", Modalidad: "electromecanica", Materias: []string{"Química"}},
	})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if res.CursosCreados != 1 {
		t.Errorf("esperaba un solo curso creado, se crearon %d", res.CursosCreados)
	}
	if res.MateriasCreadas != 2 {
		t.Errorf("esperaba las dos materias de las dos filas, se crearon %d", res.MateriasCreadas)
	}
}

// Los nombres llegan de celdas partidas por comas: «Lengua, Matemática» deja un
// espacio adelante del segundo. Sin recortar, la base guarda « Matemática».
func TestImportarEstructura_RecortaLosNombres(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	svc := servicioSimple(repo)

	if _, err := svc.ImportarEstructura(context.Background(), ciclo, []CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{" Matemática ", "  ", ""}},
	}); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	if len(repo.materias) != 1 {
		t.Fatalf("las celdas vacías deberían saltearse; hay %d materias", len(repo.materias))
	}
	for _, m := range repo.materias {
		if m.Nombre != "Matemática" {
			t.Errorf("esperaba el nombre recortado, obtuve %q", m.Nombre)
		}
	}
}

func TestImportarEstructura_CicloArchivado(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["ciclo-1"] = &domain.CicloLectivo{ID: "ciclo-1", Anio: 2025, Archivado: true}
	svc := servicioSimple(repo)

	_, err := svc.ImportarEstructura(context.Background(), "ciclo-1", []CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Lengua"}},
	})
	if !errors.Is(err, ErrCicloArchivado) {
		t.Errorf("esperaba ErrCicloArchivado, obtuve: %v", err)
	}
}

func TestImportarEstructura_SinCursos(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	svc := servicioSimple(repo)

	if _, err := svc.ImportarEstructura(context.Background(), ciclo, nil); !errors.Is(err, ErrSinCursosParaImportar) {
		t.Errorf("esperaba ErrSinCursosParaImportar, obtuve: %v", err)
	}
}

// El número de fila es lo único que permite encontrar el renglón malo en una
// planilla de cien: sin él el mensaje describe un problema que hay que salir a
// buscar.
func TestImportarEstructura_ErrorNombraLaFila(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	svc := servicioSimple(repo)

	_, err := svc.ImportarEstructura(context.Background(), ciclo, []CursoConMaterias{
		{Anio: 1, Division: "1", Materias: []string{"Lengua"}},
		{Anio: 99, Division: "1", Materias: []string{"Lengua"}},
	})
	if !errors.Is(err, domain.ErrAnioCursoInvalido) {
		t.Fatalf("esperaba ErrAnioCursoInvalido, obtuve: %v", err)
	}
	if msg := err.Error(); msg[:7] != "fila 2:" {
		t.Errorf("el error debería empezar nombrando la fila, obtuve: %q", msg)
	}
	// Y no tiene que haber escrito nada: la validación corre entera antes de
	// que la importación toque la base.
	if len(repo.cursos) != 0 {
		t.Errorf("un archivo inválido no debería crear cursos, se crearon %d", len(repo.cursos))
	}
}

// ── CopiarMaterias ──────────────────────────────────────────────────────

// cursoConMaterias deja un curso con sus materias y devuelve su ID.
func cursoConMaterias(r *fakeRepo, id, cicloID string, anio int, division string, materias ...string) string {
	r.cursos[id] = &domain.Curso{
		ID: id, CicloLectivoID: cicloID, Anio: anio, Division: division,
		Nombre: domain.ComponerNombre(anio, division),
	}
	for i, nombre := range materias {
		materiaID := id + "-m" + string(rune('1'+i))
		r.materias[materiaID] = &domain.Materia{ID: materiaID, CursoID: id, Nombre: nombre}
	}
	return id
}

func TestCopiarMaterias_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	origen := cursoConMaterias(repo, "curso-1", ciclo, 1, "1", "Lengua", "Matemática")
	destino1 := cursoConMaterias(repo, "curso-2", ciclo, 1, "2")
	destino2 := cursoConMaterias(repo, "curso-3", ciclo, 1, "3")
	svc := servicioSimple(repo)

	res, err := svc.CopiarMaterias(context.Background(), origen, []string{destino1, destino2})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if res.MateriasCreadas != 4 || res.CursosDestino != 2 {
		t.Errorf("esperaba 4 materias en 2 cursos, obtuve %d en %d",
			res.MateriasCreadas, res.CursosDestino)
	}
}

// Lo que el destino ya tiene no se toca ni se cuenta como creado: la copia
// completa lo que falta, no reemplaza lo que hay.
func TestCopiarMaterias_SalteaLasQueYaEstan(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	origen := cursoConMaterias(repo, "curso-1", ciclo, 1, "1", "Lengua", "Matemática")
	destino := cursoConMaterias(repo, "curso-2", ciclo, 1, "2", "Lengua")
	svc := servicioSimple(repo)

	res, err := svc.CopiarMaterias(context.Background(), origen, []string{destino})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if res.MateriasCreadas != 1 || res.MateriasExistentes != 1 {
		t.Errorf("esperaba 1 creada y 1 existente, obtuve %d y %d",
			res.MateriasCreadas, res.MateriasExistentes)
	}
}

func TestCopiarMaterias_DestinoRepetidoCuentaUnaVez(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	origen := cursoConMaterias(repo, "curso-1", ciclo, 1, "1", "Lengua")
	destino := cursoConMaterias(repo, "curso-2", ciclo, 1, "2")
	svc := servicioSimple(repo)

	res, err := svc.CopiarMaterias(context.Background(), origen, []string{destino, destino})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if res.CursosDestino != 1 {
		t.Errorf("esperaba un solo destino, obtuve %d", res.CursosDestino)
	}
}

func TestCopiarMaterias_AlMismoCurso(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	origen := cursoConMaterias(repo, "curso-1", ciclo, 1, "1", "Lengua")
	svc := servicioSimple(repo)

	if _, err := svc.CopiarMaterias(context.Background(), origen, []string{origen}); !errors.Is(err, ErrCopiaAlMismoCurso) {
		t.Errorf("esperaba ErrCopiaAlMismoCurso, obtuve: %v", err)
	}
}

func TestCopiarMaterias_EntreCiclos(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	otro := cicloDePrueba(repo, "ciclo-2", 2027)
	origen := cursoConMaterias(repo, "curso-1", ciclo, 1, "1", "Lengua")
	ajeno := cursoConMaterias(repo, "curso-2", otro, 1, "1")
	svc := servicioSimple(repo)

	if _, err := svc.CopiarMaterias(context.Background(), origen, []string{ajeno}); !errors.Is(err, ErrCopiaEntreCiclos) {
		t.Errorf("esperaba ErrCopiaEntreCiclos, obtuve: %v", err)
	}
}

func TestCopiarMaterias_OrigenSinMaterias(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	origen := cursoConMaterias(repo, "curso-1", ciclo, 1, "1")
	destino := cursoConMaterias(repo, "curso-2", ciclo, 1, "2")
	svc := servicioSimple(repo)

	if _, err := svc.CopiarMaterias(context.Background(), origen, []string{destino}); !errors.Is(err, ErrCursoOrigenSinMat) {
		t.Errorf("esperaba ErrCursoOrigenSinMat, obtuve: %v", err)
	}
}

func TestCopiarMaterias_CicloArchivado(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["ciclo-1"] = &domain.CicloLectivo{ID: "ciclo-1", Anio: 2025, Archivado: true}
	origen := cursoConMaterias(repo, "curso-1", "ciclo-1", 1, "1", "Lengua")
	destino := cursoConMaterias(repo, "curso-2", "ciclo-1", 1, "2")
	svc := servicioSimple(repo)

	if _, err := svc.CopiarMaterias(context.Background(), origen, []string{destino}); !errors.Is(err, ErrCicloArchivado) {
		t.Errorf("esperaba ErrCicloArchivado, obtuve: %v", err)
	}
}

// ── ExportarEstructura ──────────────────────────────────────────────────

// Lo que sale del ciclo tiene que poder volver a entrar sin traducción: es lo
// que hace que la descarga sirva para armar el año siguiente y no sólo para
// mirar.
func TestExportarEstructura_VuelveAEntrarIgual(t *testing.T) {
	repo := nuevoFakeRepo()
	ciclo := cicloDePrueba(repo, "ciclo-1", 2026)
	cursoConMaterias(repo, "curso-1", ciclo, 1, "1", "Lengua", "Matemática")
	cursoConMaterias(repo, "curso-2", ciclo, 1, "2", "Lengua")
	svc := servicioSimple(repo)

	estructura, err := svc.ExportarEstructura(context.Background(), ciclo)
	if err != nil {
		t.Fatalf("exportando: %v", err)
	}
	if len(estructura) != 2 {
		t.Fatalf("esperaba 2 cursos, obtuve %d", len(estructura))
	}

	destino := cicloDePrueba(repo, "ciclo-2", 2027)
	res, err := svc.ImportarEstructura(context.Background(), destino, estructura)
	if err != nil {
		t.Fatalf("importando lo exportado: %v", err)
	}
	if res.CursosCreados != 2 || res.MateriasCreadas != 3 {
		t.Errorf("esperaba 2 cursos y 3 materias en el ciclo nuevo, obtuve %d y %d",
			res.CursosCreados, res.MateriasCreadas)
	}
}
