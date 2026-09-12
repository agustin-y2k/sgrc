//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
)

// crearMateriaConCursoDeTest arma ciclo→curso→materia mínimos para estos
// tests — academic/infrastructure no importa reservation, así que una
// reserva_grupo se inserta acá directo por SQL (mismos campos que reservation
// usaría, sin depender de su domain).
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

	curso, err := domain.NuevoCurso(NuevoID(), ciclo.ID, 1, "A", "")
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
// reintento puede terminar lo que faltó.
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

// `regla_recurrencia.materia_id` es NOT NULL con ON DELETE NO ACTION, igual que
// `reserva_grupo.materia_id`. El chequeo previo miraba sólo la segunda, así que
// una materia con una regla y sin grupos pasaba y el DELETE reventaba contra la
// clave foránea: un 500 en vez del 409 que explica qué pasó.
func TestValidadorReservasPostgres_UnaReglaDeRecurrenciaTambienBloquea(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorReservasPostgres(pool)
	cursoID, materiaID := crearMateriaConCursoDeTest(t, pool)

	insertarReglaRecurrenciaDeTest(t, pool, materiaID)

	tieneMateria, err := validador.TieneReservasMateria(context.Background(), materiaID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !tieneMateria {
		t.Error("una materia con una regla de recurrencia no se puede eliminar")
	}

	// Y el curso también: borrarlo arrastra sus materias en cascada, así que
	// cualquier cosa que bloquee a una bloquea al curso entero.
	tieneCurso, err := validador.TieneReservasCurso(context.Background(), cursoID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !tieneCurso {
		t.Error("un curso con una materia con regla de recurrencia no se puede eliminar")
	}
}

// Y que el DELETE de verdad fallaría: lo que justifica el chequeo previo es que
// la base rechaza, y si dejara de rechazar el chequeo pasaría a ser una
// restricción inventada.
func TestValidadorReservasPostgres_LaBaseRechazaBorrarUnaMateriaConRegla(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	_, materiaID := crearMateriaConCursoDeTest(t, pool)

	insertarReglaRecurrenciaDeTest(t, pool, materiaID)

	if err := repo.EliminarMateria(context.Background(), materiaID); err == nil {
		t.Error("la base tenía que rechazar el borrado por la clave foránea de regla_recurrencia")
	}
}

// ── HayBloqueosEnElAnio ─────────────────────────────────────────────────
//
// Es la baranda de corregirle el año a un ciclo, y sólo se puede probar contra
// Postgres: la pregunta no es "¿este ciclo tiene bloqueos?" sino "¿este AÑO
// tiene bloqueos?", porque el año que se quiere estrenar todavía no es de
// ningún ciclo. Un bloqueo no tiene clave foránea al ciclo — se le atribuye a
// uno por EXTRACT(YEAR FROM fecha), y crear uno en un año sin ciclo está
// permitido.

func insertarBloqueoDeTest(t *testing.T, pool *pgxpool.Pool, fecha time.Time) {
	t.Helper()
	var equipoID string
	// La serie va en MAYÚSCULAS porque un CHECK de la tabla lo exige —
	// numero_serie tiene que ser igual a upper(btrim(...))—, que es la misma
	// normalización que hace el dominio al guardarla.
	err := pool.QueryRow(context.Background(), `
		INSERT INTO equipo (id, numero_serie, tipo, nombre, estado, reservable, es_computadora, fecha_alta)
		VALUES ($1, $2, 'PROYECTOR', $3, 'DISPONIBLE', true, false, now())
		RETURNING id
	`, NuevoID(), strings.ToUpper("SERIE-"+NuevoID()[:8]), "Proyector "+NuevoID()[:8]).Scan(&equipoID)
	if err != nil {
		t.Fatalf("no se pudo insertar equipo de prueba: %v", err)
	}

	_, err = pool.Exec(context.Background(), `
		INSERT INTO reserva (id, equipo_id, fecha, hora_inicio, hora_fin, estado, tipo, motivo_bloqueo)
		VALUES ($1, $2, $3, '08:00', '09:00', 'CONFIRMADA', 'BLOQUEO', 'jornada docente')
	`, NuevoID(), equipoID, fecha)
	if err != nil {
		t.Fatalf("no se pudo insertar bloqueo de prueba: %v", err)
	}
}

func TestValidadorReservasPostgres_HayBloqueosEnElAnio(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	ctx := context.Background()
	v := NewValidadorReservasPostgres(pool)

	// Un año sin nada: el ciclo se puede mudar ahí.
	hay, err := v.HayBloqueosEnElAnio(ctx, 2031)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if hay {
		t.Fatal("un año vacío no tiene bloqueos")
	}

	// El bloqueo se crea sin ningún ciclo de 2031 — que es justamente el caso
	// que hace falta cubrir: nada impide cargarlo.
	insertarBloqueoDeTest(t, pool, time.Date(2031, 5, 12, 0, 0, 0, 0, time.UTC))

	hay, err = v.HayBloqueosEnElAnio(ctx, 2031)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !hay {
		t.Error("el bloqueo de 2031 tenía que aparecer")
	}

	// Y no se cuenta en el año de al lado.
	hay, err = v.HayBloqueosEnElAnio(ctx, 2032)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if hay {
		t.Error("un bloqueo de 2031 no es de 2032")
	}
}

// Una reserva NORMAL no es un bloqueo: no cambia de dueño al mover un ciclo de
// año porque cuelga de una materia, que cuelga de un curso, que sí tiene clave
// foránea al ciclo.
func TestValidadorReservasPostgres_HayBloqueosEnElAnio_IgnoraLasNormales(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	ctx := context.Background()
	_, materiaID := crearMateriaConCursoDeTest(t, pool)
	insertarReservaGrupoDeTest(t, pool, materiaID)

	v := NewValidadorReservasPostgres(pool)
	hay, err := v.HayBloqueosEnElAnio(ctx, time.Now().AddDate(0, 0, 7).Year())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if hay {
		t.Error("una reserva de clase no es un bloqueo administrativo")
	}
}

// ── La carrera entre comprobar y borrar ─────────────────────────────────
//
// El servicio pregunta si la materia tiene reservas y después la borra. Entre
// las dos cosas hay una ventana: si alguien crea una reserva justo ahí, el
// DELETE choca contra una foránea. Estos tests saltean la comprobación previa
// —llaman al repositorio directo, que es exactamente lo que hace una carrera
// ganada— y verifican que lo que sale es el error de dominio y no uno crudo.

func TestEliminarMateria_ConReservas_DevuelveElErrorDeDominio(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	ctx := context.Background()
	_, materiaID := crearMateriaConCursoDeTest(t, pool)
	insertarReservaGrupoDeTest(t, pool, materiaID)

	err := NewPostgresRepo(pool).EliminarMateria(ctx, materiaID)

	if !errors.Is(err, application.ErrMateriaConReservas) {
		t.Fatalf("esperaba ErrMateriaConReservas, obtuve %v", err)
	}
}

// Borrar el curso arrastra sus materias en cascada, así que choca contra la
// misma foránea una capa más abajo.
func TestEliminarCurso_ConMateriaReservada_DevuelveElErrorDeDominio(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	ctx := context.Background()
	cursoID, materiaID := crearMateriaConCursoDeTest(t, pool)
	insertarReservaGrupoDeTest(t, pool, materiaID)

	err := NewPostgresRepo(pool).EliminarCurso(ctx, cursoID)

	if !errors.Is(err, application.ErrCursoConReservas) {
		t.Fatalf("esperaba ErrCursoConReservas, obtuve %v", err)
	}
}

// Y sin nada colgando se borran, que es el camino normal: la traducción de
// arriba no puede estar frenando lo que sí se puede borrar.
func TestEliminarMateriaYCurso_SinReservas_Borran(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	ctx := context.Background()
	cursoID, materiaID := crearMateriaConCursoDeTest(t, pool)
	repo := NewPostgresRepo(pool)

	if err := repo.EliminarMateria(ctx, materiaID); err != nil {
		t.Fatalf("una materia sin reservas tenía que borrarse: %v", err)
	}
	if err := repo.EliminarCurso(ctx, cursoID); err != nil {
		t.Fatalf("un curso sin materias tenía que borrarse: %v", err)
	}
}
