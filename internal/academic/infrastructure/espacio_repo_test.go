//go:build integration

package infrastructure

import (
	"context"
	"testing"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
)

// Los espacios (RF-02.13) contra una base real.
//
// Lo que se prueba acá y no en los tests con fake es justamente lo que sólo
// Postgres puede contestar: el CHECK que exige un contenedor y sólo uno, la
// unicidad del nombre sin tildes, y —lo más importante— que una materia de un
// espacio NO desaparezca de las consultas que antes hacían `JOIN curso`.

func crearCicloDePrueba(t *testing.T, repo *PostgresRepo, anio int) *domain.CicloLectivo {
	t.Helper()
	c, err := domain.NuevoCicloLectivo(uuidNuevo(), anio)
	if err != nil {
		t.Fatalf("armando ciclo: %v", err)
	}
	if err := repo.CrearCiclo(context.Background(), c); err != nil {
		t.Fatalf("creando ciclo: %v", err)
	}
	return c
}

func nuevoEspacioDePrueba(t *testing.T, repo *PostgresRepo, cicloID, nombre string) *domain.Espacio {
	t.Helper()
	e, err := domain.NuevoEspacio(uuidNuevo(), cicloID, nombre)
	if err != nil {
		t.Fatalf("armando espacio: %v", err)
	}
	if err := repo.CrearEspacio(context.Background(), e); err != nil {
		t.Fatalf("creando espacio: %v", err)
	}
	return e
}

func TestEspacio_SeCreaYSeLista(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := crearCicloDePrueba(t, repo, 2026)

	nuevoEspacioDePrueba(t, repo, ciclo.ID, "Biblioteca")
	nuevoEspacioDePrueba(t, repo, ciclo.ID, "Dirección")

	espacios, err := repo.ListarEspaciosPorCiclo(ctx, ciclo.ID)
	if err != nil {
		t.Fatalf("listando: %v", err)
	}
	if len(espacios) != 2 {
		t.Fatalf("esperaba 2 espacios, obtuve %d", len(espacios))
	}
}

// Misma regla que el resto del sistema (migración 011).
func TestEspacio_NombreRepetidoSinTildes(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ciclo := crearCicloDePrueba(t, repo, 2026)

	nuevoEspacioDePrueba(t, repo, ciclo.ID, "Dirección")

	otro, _ := domain.NuevoEspacio(uuidNuevo(), ciclo.ID, "direccion")
	err := repo.CrearEspacio(context.Background(), otro)
	if err != application.ErrNombreEspacioDuplicado {
		t.Fatalf("esperaba nombre duplicado, obtuve %v", err)
	}
}

// El CHECK de la migración 017: una materia cuelga de UNO de los dos.
func TestMateria_NoPuedeColgarDeLosDosNiDeNinguno(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`INSERT INTO materia (id, curso_id, espacio_id, nombre) VALUES (gen_random_uuid(), NULL, NULL, 'Suelta')`)
	if err == nil {
		t.Error("una materia sin contenedor entró en la base")
	}
}

// **El test que justifica la vista.** Una materia de espacio tiene que
// aparecer en las consultas que antes hacían `JOIN curso`: si alguna vuelve a
// ese JOIN, la descarta en silencio y esto se pone en rojo.
func TestMateriaDeEspacio_EsReservableYNoSePierde(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := crearCicloDePrueba(t, repo, 2026)

	espacio := nuevoEspacioDePrueba(t, repo, ciclo.ID, "Biblioteca")

	// No se carga ninguna materia a mano: estos lugares no dictan materias, y
	// el alta del espacio ya dejó la fila que hace posible reservar para él.
	materias, err := repo.ListarMateriasPorEspacio(ctx, espacio.ID)
	if err != nil {
		t.Fatalf("listando materias del espacio: %v", err)
	}
	if len(materias) != 1 || materias[0].Nombre != "Biblioteca" {
		t.Fatalf("el alta del espacio tenía que dejar su materia; quedó %+v", materias)
	}
	m := materias[0]

	reservables, err := repo.ListarMateriasReservables(ctx, nil)
	if err != nil {
		t.Fatalf("listando reservables: %v", err)
	}
	var encontrada *application.MateriaReservable
	for i := range reservables {
		if reservables[i].MateriaID == m.ID {
			encontrada = &reservables[i]
		}
	}
	if encontrada == nil {
		t.Fatal("la materia de la Biblioteca no aparece entre las reservables — alguna consulta volvió al JOIN curso")
	}
	// El contenedor viaja con su nombre, para que la pantalla pueda decir de
	// dónde es: «Apoyo escolar (Biblioteca)».
	if encontrada.CursoNombre != "Biblioteca" {
		t.Errorf("esperaba que el contenedor se llamara Biblioteca; dijo %q", encontrada.CursoNombre)
	}
}

// El clonado de fin de año tiene que llevarse los espacios: si no, la
// Biblioteca desaparece cada 31 de diciembre y nadie se entera.
func TestClonado_SeLlevaLosEspaciosConSusMaterias(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	viejo := crearCicloDePrueba(t, repo, 2026)

	nuevoEspacioDePrueba(t, repo, viejo.ID, "Biblioteca")

	// Se archiva primero: sólo puede haber un ciclo activo, y el clonado de
	// verdad (ArchivarYClonar) hace las dos cosas en ese orden.
	if err := repo.ArchivarCiclo(ctx, viejo.ID); err != nil {
		t.Fatalf("archivando el ciclo viejo: %v", err)
	}

	nuevo, err := domain.NuevoCicloLectivo(uuidNuevo(), 2027)
	if err != nil {
		t.Fatalf("armando ciclo nuevo: %v", err)
	}
	if _, _, err := repo.ClonarCicloA(ctx, viejo.ID, nuevo); err != nil {
		t.Fatalf("clonando: %v", err)
	}

	espacios, err := repo.ListarEspaciosPorCiclo(ctx, nuevo.ID)
	if err != nil {
		t.Fatalf("listando espacios del ciclo nuevo: %v", err)
	}
	if len(espacios) != 1 || espacios[0].Nombre != "Biblioteca" {
		t.Fatalf("el clonado perdió los espacios: %+v", espacios)
	}

	materias, err := repo.ListarMateriasPorEspacio(ctx, espacios[0].ID)
	if err != nil {
		t.Fatalf("listando materias del espacio clonado: %v", err)
	}
	if len(materias) != 1 || materias[0].Nombre != "Biblioteca" {
		t.Fatalf("el clonado perdió las materias del espacio: %+v", materias)
	}
}

// Renombrar el lugar tiene que renombrar también aquello para lo que se
// reserva. Si no, Académico diría «Biblioteca central» y la pantalla de
// reservar seguiría ofreciendo «Biblioteca», sin nada que lo explique.
func TestEspacio_RenombrarArrastraLoQueSeReserva(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ciclo := crearCicloDePrueba(t, repo, 2026)

	e := nuevoEspacioDePrueba(t, repo, ciclo.ID, "Biblioteca")
	if err := e.Renombrar("Biblioteca central"); err != nil {
		t.Fatalf("renombrando: %v", err)
	}
	if err := repo.GuardarEspacio(ctx, e); err != nil {
		t.Fatalf("guardando: %v", err)
	}

	materias, err := repo.ListarMateriasPorEspacio(ctx, e.ID)
	if err != nil {
		t.Fatalf("listando: %v", err)
	}
	if len(materias) != 1 || materias[0].Nombre != "Biblioteca central" {
		t.Fatalf("la materia no siguió al nombre del lugar: %+v", materias)
	}
}
