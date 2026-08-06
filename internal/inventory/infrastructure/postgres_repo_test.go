//go:build integration

package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/testdb"
)

func levantarPostgresDeTest(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	esquema, err := testdb.SQLDeMigraciones("../../../migrations")
	if err != nil {
		t.Fatalf("no se pudo leer el esquema: %v", err)
	}

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

	if _, err := pool.Exec(ctx, esquema); err != nil {
		t.Fatalf("no se pudo aplicar la migración: %v", err)
	}

	return pool
}

func crearCarroDeTest(t *testing.T, repo *PostgresRepo, nombre string) *domain.Carro {
	t.Helper()
	c, err := domain.NuevoCarro(NuevoID(), nombre, "")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCarro(context.Background(), c); err != nil {
		t.Fatalf("no se pudo crear el carro de prueba: %v", err)
	}
	return c
}

func TestPostgresRepo_CrearCarroYBuscarPorID_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)

	c := crearCarroDeTest(t, repo, "Carro 1")

	encontrado, err := repo.BuscarCarroPorID(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if encontrado.Nombre != "Carro 1" {
		t.Errorf("nombre incorrecto: %s", encontrado.Nombre)
	}
}

func TestPostgresRepo_CrearPC_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc, err := domain.NuevaPC(NuevoID(), carro.ID, 27, 123456789, true, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	pc.CPU = "i5"
	pc.SoftwareInstalado = "AutoCAD 2026"

	if err := repo.CrearPC(ctx, pc); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	encontrada, err := repo.BuscarPCPorID(ctx, pc.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if encontrada.Identificador != 27 || encontrada.CPU != "i5" || encontrada.SoftwareInstalado != "AutoCAD 2026" {
		t.Errorf("PC encontrada no coincide: %+v", encontrada)
	}
}

func TestPostgresRepo_IdentificadorRepetidoEnMismoCarro_Error(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc1, _ := domain.NuevaPC(NuevoID(), carro.ID, 27, 111, false, time.Now())
	pc2, _ := domain.NuevaPC(NuevoID(), carro.ID, 27, 222, false, time.Now()) // mismo identificador, mismo carro

	if err := repo.CrearPC(ctx, pc1); err != nil {
		t.Fatalf("la primera no debería fallar: %v", err)
	}
	err := repo.CrearPC(ctx, pc2)
	if err != application.ErrIdentificadorDuplicado {
		t.Fatalf("esperaba ErrIdentificadorDuplicado, obtuve %v", err)
	}
}

func TestPostgresRepo_MismoIdentificadorOtroCarro_OK(t *testing.T) {
	// Confirma contra Postgres real la regla de negocio: el identificador
	// puede repetirse entre carros distintos.
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro1 := crearCarroDeTest(t, repo, "Carro 1")
	carro2 := crearCarroDeTest(t, repo, "Carro 2")
	pc1, _ := domain.NuevaPC(NuevoID(), carro1.ID, 27, 111, false, time.Now())
	pc2, _ := domain.NuevaPC(NuevoID(), carro2.ID, 27, 222, false, time.Now())

	if err := repo.CrearPC(ctx, pc1); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.CrearPC(ctx, pc2); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
}

func TestPostgresRepo_NumeroSerieDuplicado_Error(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro1 := crearCarroDeTest(t, repo, "Carro 1")
	carro2 := crearCarroDeTest(t, repo, "Carro 2")
	pc1, _ := domain.NuevaPC(NuevoID(), carro1.ID, 1, 999888777, false, time.Now())
	pc2, _ := domain.NuevaPC(NuevoID(), carro2.ID, 1, 999888777, false, time.Now()) // mismo numero_serie, distinto carro

	if err := repo.CrearPC(ctx, pc1); err != nil {
		t.Fatalf("la primera no debería fallar: %v", err)
	}
	err := repo.CrearPC(ctx, pc2)
	if err == nil {
		t.Fatal("esperaba un error por número de serie duplicado (constraint global)")
	}
}

func TestPostgresRepo_GuardarPC_ActualizaEstado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc, _ := domain.NuevaPC(NuevoID(), carro.ID, 1, 1, false, time.Now())
	if err := repo.CrearPC(ctx, pc); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if err := pc.CambiarEstado(domain.EstadoEnMantenimiento); err != nil {
		t.Fatalf("transición de dominio inválida: %v", err)
	}
	if err := repo.GuardarPC(ctx, pc); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recargada, err := repo.BuscarPCPorID(ctx, pc.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if recargada.Estado != domain.EstadoEnMantenimiento {
		t.Errorf("el estado no se persistió: %s", recargada.Estado)
	}
}

func TestPostgresRepo_ListarPCsPorCarro(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc1, _ := domain.NuevaPC(NuevoID(), carro.ID, 1, 111, false, time.Now())
	pc2, _ := domain.NuevaPC(NuevoID(), carro.ID, 2, 222, false, time.Now())
	repo.CrearPC(ctx, pc1)
	repo.CrearPC(ctx, pc2)

	pcs, err := repo.ListarPCsPorCarro(ctx, carro.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(pcs) != 2 {
		t.Fatalf("esperaba 2 PCs, obtuve %d", len(pcs))
	}
}

func TestPostgresRepo_Incidencia_CrearYListar(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc, _ := domain.NuevaPC(NuevoID(), carro.ID, 1, 1, false, time.Now())
	if err := repo.CrearPC(ctx, pc); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	inc, err := domain.NuevaIncidencia(NuevoID(), pc.ID, "", "No enciende", domain.GravedadGrave, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearIncidencia(ctx, inc); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	incidencias, err := repo.ListarIncidenciasPorPC(ctx, pc.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(incidencias) != 1 || incidencias[0].Descripcion != "No enciende" {
		t.Fatalf("incidencias incorrectas: %+v", incidencias)
	}
}

func TestPostgresRepo_GuardarIncidencia_MarcarEnviadaDGE(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc, _ := domain.NuevaPC(NuevoID(), carro.ID, 1, 1, false, time.Now())
	repo.CrearPC(ctx, pc)

	inc, _ := domain.NuevaIncidencia(NuevoID(), pc.ID, "", "Falla", domain.GravedadGrave, time.Now().UTC().Truncate(time.Microsecond))
	if err := repo.CrearIncidencia(ctx, inc); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	inc.MarcarEnviadaDGE(time.Now().UTC().Truncate(time.Microsecond))
	if err := repo.GuardarIncidencia(ctx, inc); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recargada, err := repo.BuscarIncidenciaPorID(ctx, inc.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !recargada.EnviadoDGE || recargada.Estado != domain.IncidenciaEnviadaDGE {
		t.Errorf("no se persistió el envío a DGE: %+v", recargada)
	}
}

// Un ID sin formato UUID tiene que mapear a application.ErrIDInvalido
// (400), nunca a un 500 crudo de Postgres: es un error del cliente.
func TestPostgresRepo_IDConFormatoInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	casos := []struct {
		nombre string
		fn     func() error
	}{
		{"BuscarCarroPorID", func() error { _, err := repo.BuscarCarroPorID(ctx, "CARRO_ID"); return err }},
		{"BuscarPCPorID", func() error { _, err := repo.BuscarPCPorID(ctx, "PC_ID"); return err }},
		{"BuscarIncidenciaPorID", func() error { _, err := repo.BuscarIncidenciaPorID(ctx, "INCIDENCIA_ID"); return err }},
		{"ListarPCsPorCarro", func() error { _, err := repo.ListarPCsPorCarro(ctx, "CARRO_ID"); return err }},
		{"ListarIncidenciasPorPC", func() error { _, err := repo.ListarIncidenciasPorPC(ctx, "PC_ID"); return err }},
		{"CrearPC_CarroInvalido", func() error {
			pc, _ := domain.NuevaPC(NuevoID(), "CARRO_ID", 1, 1, false, time.Now())
			return repo.CrearPC(ctx, pc)
		}},
	}

	for _, c := range casos {
		err := c.fn()
		if err != application.ErrIDInvalido {
			t.Errorf("%s: esperaba application.ErrIDInvalido, obtuve %v", c.nombre, err)
		}
	}
}
