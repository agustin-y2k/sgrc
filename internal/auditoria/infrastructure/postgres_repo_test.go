//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ramiro/sgrc/internal/auditoria/application"
	"github.com/ramiro/sgrc/internal/auditoria/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
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

func crearUsuario(t *testing.T, pool *pgxpool.Pool, nombre, apellido string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO usuario (id, nombre, apellido, email, password_hash, rol, estado)
		VALUES ($1, $2, $3, $4, 'hash', 'ADMIN', 'APROBADA')`,
		id, nombre, apellido, id+"@escuela.test")
	if err != nil {
		t.Fatalf("insertando usuario: %v", err)
	}
	return id
}

// anotar escribe una entrada directo por SQL, no vía shared/audit: este paquete
// LEE, y atarlo al escritor haría que un test de lectura falle por un cambio en
// la escritura.
func anotar(t *testing.T, pool *pgxpool.Pool, usuarioID, accion, entidad string, entidadID *string, cuando time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO audit_log (usuario_id, accion, entidad, entidad_id, detalle, ip_origen, creado_en)
		VALUES ($1, $2, $3, $4, '{"cuantos": 3}'::jsonb, '10.0.0.7', $5)`,
		usuarioID, accion, entidad, entidadID, cuando)
	if err != nil {
		t.Fatalf("anotando en el registro: %v", err)
	}
}

var (
	ayer = time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	hoy  = time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC)
)

func TestListar_OrdenaDeLoMasNuevoALoMasViejo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	admin := crearUsuario(t, pool, "Marta", "Fernández")

	anotar(t, pool, admin, "CURSO_ELIMINADO", "curso", nil, ayer)
	anotar(t, pool, admin, "MATERIA_ELIMINADA", "materia", nil, hoy)

	entradas, total, err := repo.Listar(context.Background(), domain.Filtro{}, paginacion.PorDefecto())
	if err != nil {
		t.Fatalf("listando: %v", err)
	}
	if total != 2 {
		t.Fatalf("esperaba 2, obtuve %d", total)
	}
	if entradas[0].Accion != "MATERIA_ELIMINADA" {
		t.Errorf("lo más nuevo va primero; llegó %q", entradas[0].Accion)
	}
	if entradas[0].ActorNombre != "Marta Fernández" {
		t.Errorf("el nombre del actor tenía que resolverse, llegó %q", entradas[0].ActorNombre)
	}
	if entradas[0].IPOrigen != "10.0.0.7" {
		t.Errorf("la IP llegó como %q", entradas[0].IPOrigen)
	}
	if len(entradas[0].Detalle) == 0 {
		t.Error("el detalle JSON tenía que viajar")
	}
}

// Lo que hizo una cuenta SOBREVIVE a su eliminación (RF-01.9): `audit_log` no
// tiene clave foránea a propósito. Con un INNER JOIN, eliminar una cuenta
// borraría de la vista todo lo que hizo — que es exactamente lo que el registro
// existe para impedir.
func TestListar_LaEntradaSobreviveALaCuentaEliminada(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	admin := crearUsuario(t, pool, "Marta", "Fernández")

	anotar(t, pool, admin, "EQUIPO_DADO_DE_BAJA", "equipo", nil, hoy)

	if _, err := pool.Exec(ctx, `DELETE FROM usuario WHERE id = $1`, admin); err != nil {
		t.Fatalf("eliminando la cuenta: %v", err)
	}

	entradas, total, err := repo.Listar(ctx, domain.Filtro{}, paginacion.PorDefecto())
	if err != nil {
		t.Fatalf("listando: %v", err)
	}
	if total != 1 {
		t.Fatalf("la entrada tenía que sobrevivir a la cuenta; total=%d", total)
	}
	if entradas[0].ActorNombre != "" {
		t.Errorf("sin cuenta no hay nombre; llegó %q", entradas[0].ActorNombre)
	}
	if entradas[0].UsuarioID != admin {
		t.Errorf("el identificador del actor tiene que quedar: llegó %q", entradas[0].UsuarioID)
	}
}

func TestListar_FiltraPorCadaEje(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	marta := crearUsuario(t, pool, "Marta", "Fernández")
	ana := crearUsuario(t, pool, "Ana", "Gómez")
	curso := uuid.NewString()

	anotar(t, pool, marta, "CURSO_ELIMINADO", "curso", &curso, hoy)
	anotar(t, pool, ana, "MATERIA_ELIMINADA", "materia", nil, hoy)
	anotar(t, pool, ana, "CURSO_ELIMINADO", "curso", nil, ayer)

	casos := []struct {
		nombre   string
		filtro   domain.Filtro
		esperado int
	}{
		{"por acción", domain.Filtro{Accion: "CURSO_ELIMINADO"}, 2},
		{"por entidad", domain.Filtro{Entidad: "materia"}, 1},
		{"por la cosa puntual", domain.Filtro{EntidadID: curso}, 1},
		{"por quién lo hizo", domain.Filtro{UsuarioID: ana}, 2},
		{"dos ejes a la vez", domain.Filtro{Accion: "CURSO_ELIMINADO", UsuarioID: ana}, 1},
		{"sin filtro", domain.Filtro{}, 3},
	}
	for _, c := range casos {
		_, total, err := repo.Listar(ctx, c.filtro, paginacion.PorDefecto())
		if err != nil {
			t.Errorf("%s: %v", c.nombre, err)
			continue
		}
		if total != c.esperado {
			t.Errorf("%s: esperaba %d, obtuve %d", c.nombre, c.esperado, total)
		}
	}
}

// El «hasta» es inclusivo del día entero: quien escribe «hasta el 11» espera
// que entre lo que pasó el 11 a las 09:00. Quien arma el filtro convierte esa
// fecha en el límite exclusivo del día siguiente, y esto comprueba que la
// consulta respete ese contrato.
func TestListar_FiltraPorRangoDeFechas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	admin := crearUsuario(t, pool, "Marta", "Fernández")

	anotar(t, pool, admin, "CURSO_ELIMINADO", "curso", nil, ayer)
	anotar(t, pool, admin, "MATERIA_ELIMINADA", "materia", nil, hoy)

	soloHoyDesde := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	finExclusivo := soloHoyDesde.AddDate(0, 0, 1)

	_, total, err := repo.Listar(ctx,
		domain.Filtro{Desde: &soloHoyDesde, Hasta: &finExclusivo}, paginacion.PorDefecto())
	if err != nil {
		t.Fatalf("listando: %v", err)
	}
	if total != 1 {
		t.Errorf("«sólo hoy» tendría que traer 1, trajo %d", total)
	}
}

// Un UUID mal formado vuelve como un 400 que se puede leer, no como el 500 que
// devuelve Postgres.
func TestListar_IDMalFormado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)

	_, _, err := repo.Listar(context.Background(),
		domain.Filtro{EntidadID: "no-soy-un-uuid"}, paginacion.PorDefecto())
	if !errors.Is(err, application.ErrIDInvalido) {
		t.Errorf("esperaba ErrIDInvalido, obtuve: %v", err)
	}
}

func TestOpciones_SoloLoQueDeVerdadOcurrio(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	admin := crearUsuario(t, pool, "Marta", "Fernández")

	anotar(t, pool, admin, "CURSO_ELIMINADO", "curso", nil, hoy)
	anotar(t, pool, admin, "CURSO_ELIMINADO", "curso", nil, ayer)

	acciones, err := repo.AccionesEnUso(ctx)
	if err != nil {
		t.Fatalf("leyendo acciones: %v", err)
	}
	// Una sola, sin repetir, aunque haya dos filas: el selector ofrece opciones,
	// no ocurrencias.
	if len(acciones) != 1 || acciones[0] != "CURSO_ELIMINADO" {
		t.Errorf("esperaba sólo CURSO_ELIMINADO, obtuve %v", acciones)
	}

	entidades, err := repo.EntidadesEnUso(ctx)
	if err != nil {
		t.Fatalf("leyendo entidades: %v", err)
	}
	if len(entidades) != 1 || entidades[0] != "curso" {
		t.Errorf("esperaba sólo «curso», obtuve %v", entidades)
	}
}

// Con el registro vacío los selectores vuelven vacíos, no nulos: la pantalla
// los recorre sin preguntar.
func TestOpciones_RegistroVacio(t *testing.T) {
	repo := NewPostgresRepo(levantarPostgresDeTest(t))

	acciones, err := repo.AccionesEnUso(context.Background())
	if err != nil {
		t.Fatalf("leyendo acciones: %v", err)
	}
	if acciones == nil {
		t.Error("tendría que ser una lista vacía, no nil")
	}
}
