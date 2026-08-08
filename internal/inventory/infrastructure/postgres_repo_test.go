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

func TestPostgresRepo_CrearEquipo_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	equipo, err := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 27, "5CD1234ABC", true, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	equipo.CPU = "i5"
	equipo.SoftwareInstalado = "AutoCAD 2026"

	if err := repo.CrearEquipo(ctx, equipo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	encontrada, err := repo.BuscarEquipoPorID(ctx, equipo.ID)
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
	pc1, _ := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 27, "SERIE-111", false, time.Now())
	pc2, _ := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 27, "SERIE-222", false, time.Now()) // mismo identificador, mismo carro

	if err := repo.CrearEquipo(ctx, pc1); err != nil {
		t.Fatalf("la primera no debería fallar: %v", err)
	}
	err := repo.CrearEquipo(ctx, pc2)
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
	pc1, _ := domain.NuevoEquipoDeCarro(NuevoID(), carro1.ID, 27, "SERIE-111", false, time.Now())
	pc2, _ := domain.NuevoEquipoDeCarro(NuevoID(), carro2.ID, 27, "SERIE-222", false, time.Now())

	if err := repo.CrearEquipo(ctx, pc1); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.CrearEquipo(ctx, pc2); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
}

func TestPostgresRepo_NumeroSerieDuplicado_Error(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro1 := crearCarroDeTest(t, repo, "Carro 1")
	carro2 := crearCarroDeTest(t, repo, "Carro 2")
	pc1, _ := domain.NuevoEquipoDeCarro(NuevoID(), carro1.ID, 1, "SERIE-999888777", false, time.Now())
	pc2, _ := domain.NuevoEquipoDeCarro(NuevoID(), carro2.ID, 1, "SERIE-999888777", false, time.Now()) // mismo numero_serie, distinto carro

	if err := repo.CrearEquipo(ctx, pc1); err != nil {
		t.Fatalf("la primera no debería fallar: %v", err)
	}
	err := repo.CrearEquipo(ctx, pc2)
	if err == nil {
		t.Fatal("esperaba un error por número de serie duplicado (constraint global)")
	}
}

func TestPostgresRepo_GuardarEquipo_ActualizaEstado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	equipo, _ := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 1, "SERIE-UNICA", false, time.Now())
	if err := repo.CrearEquipo(ctx, equipo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if err := equipo.CambiarEstado(domain.EstadoEnMantenimiento); err != nil {
		t.Fatalf("transición de dominio inválida: %v", err)
	}
	if err := repo.GuardarEquipo(ctx, equipo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recargada, err := repo.BuscarEquipoPorID(ctx, equipo.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if recargada.Estado != domain.EstadoEnMantenimiento {
		t.Errorf("el estado no se persistió: %s", recargada.Estado)
	}
}

func TestPostgresRepo_ListarEquiposPorCarro(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc1, _ := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 1, "SERIE-111", false, time.Now())
	pc2, _ := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 2, "SERIE-222", false, time.Now())
	repo.CrearEquipo(ctx, pc1)
	repo.CrearEquipo(ctx, pc2)

	equipos, err := repo.ListarEquiposPorCarro(ctx, carro.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(equipos) != 2 {
		t.Fatalf("esperaba 2 PCs, obtuve %d", len(equipos))
	}
}

func TestPostgresRepo_Incidencia_CrearYListar(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	equipo, _ := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 1, "SERIE-UNICA", false, time.Now())
	if err := repo.CrearEquipo(ctx, equipo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	inc, err := domain.NuevaIncidencia(NuevoID(), equipo.ID, "", "No enciende", domain.GravedadGrave, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearIncidencia(ctx, inc); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	incidencias, err := repo.ListarIncidenciasPorEquipo(ctx, equipo.ID)
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
	equipo, _ := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, 1, "SERIE-UNICA", false, time.Now())
	repo.CrearEquipo(ctx, equipo)

	inc, _ := domain.NuevaIncidencia(NuevoID(), equipo.ID, "", "Falla", domain.GravedadGrave, time.Now().UTC().Truncate(time.Microsecond))
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
		{"BuscarEquipoPorID", func() error { _, err := repo.BuscarEquipoPorID(ctx, "PC_ID"); return err }},
		{"BuscarIncidenciaPorID", func() error { _, err := repo.BuscarIncidenciaPorID(ctx, "INCIDENCIA_ID"); return err }},
		{"ListarEquiposPorCarro", func() error { _, err := repo.ListarEquiposPorCarro(ctx, "CARRO_ID"); return err }},
		{"ListarIncidenciasPorEquipo", func() error { _, err := repo.ListarIncidenciasPorEquipo(ctx, "PC_ID"); return err }},
		{"CrearEquipo_CarroInvalido", func() error {
			equipo, _ := domain.NuevoEquipoDeCarro(NuevoID(), "CARRO_ID", 1, "SERIE-UNICA", false, time.Now())
			return repo.CrearEquipo(ctx, equipo)
		}},
	}

	for _, c := range casos {
		err := c.fn()
		if err != application.ErrIDInvalido {
			t.Errorf("%s: esperaba application.ErrIDInvalido, obtuve %v", c.nombre, err)
		}
	}
}

// ── Equipos sueltos (015) ───────────────────────────────────────────────

func crearEquipoSueltoDeTest(t *testing.T, repo *PostgresRepo, tipo, nombre string, reservable bool) *domain.Equipo {
	t.Helper()
	eq, err := domain.NuevoEquipoSuelto(NuevoID(), tipo, nombre, reservable, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearEquipo(context.Background(), eq); err != nil {
		t.Fatalf("no se pudo crear el equipo de prueba: %v", err)
	}
	return eq
}

// Lo que solo una base de verdad puede confirmar: que las tres columnas que
// la 015 aflojó aceptan NULL y vuelven vacías, no como un cero o un error de
// escaneo. Un proyector sin carro, sin identificador y sin número de serie es
// exactamente la fila que antes era imposible.
func TestPostgresRepo_EquipoSuelto_GuardarYRecuperar(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	eq := crearEquipoSueltoDeTest(t, repo, "PROYECTOR", "Proyector Epson", true)

	vuelto, err := repo.BuscarEquipoPorID(ctx, eq.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if vuelto.CarroID != "" || vuelto.Identificador != 0 || vuelto.NumeroSerie != "" {
		t.Errorf("esperaba carro/identificador/serie vacíos, obtuve %q/%d/%q",
			vuelto.CarroID, vuelto.Identificador, vuelto.NumeroSerie)
	}
	if vuelto.Tipo != "PROYECTOR" || vuelto.Nombre != "Proyector Epson" || !vuelto.Reservable {
		t.Errorf("no volvió como se guardó: %+v", vuelto)
	}
	if vuelto.Etiqueta() != "Proyector Epson" {
		t.Errorf("un equipo suelto se nombra por su nombre, obtuve %q", vuelto.Etiqueta())
	}
}

// Una PC de carro tiene que seguir volviendo igual que siempre: las columnas
// nuevas no pueden ensuciarla ni el escaneo por punteros perder nada.
func TestPostgresRepo_EquipoDeCarro_SigueSiendoEquipo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	equipo := crearEquipoDeCarroDeTest(t, repo, carro.ID, 3, "SERIE-EQ-1")

	vuelto, err := repo.BuscarEquipoPorID(ctx, equipo.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if vuelto.Tipo != domain.TipoPC || vuelto.Nombre != "" || !vuelto.Reservable {
		t.Errorf("esperaba tipo PC, sin nombre y reservable; obtuve %q/%q/%v",
			vuelto.Tipo, vuelto.Nombre, vuelto.Reservable)
	}
	if vuelto.Etiqueta() != "PC 3" {
		t.Errorf("una PC de carro se nombra por su identificador, obtuve %q", vuelto.Etiqueta())
	}
}

// El nombre es lo único que distingue a un equipo suelto en la lista de
// entregas. Dos "Cargador 1" tienen que rebotar como un 409 con mensaje, no
// como un 500 de la base — y sin importar la caja.
func TestPostgresRepo_EquipoSuelto_NombreDuplicado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	crearEquipoSueltoDeTest(t, repo, "CARGADOR", "Cargador 1", false)

	otro, _ := domain.NuevoEquipoSuelto(NuevoID(), "CARGADOR", "CARGADOR 1", false, time.Now().UTC())
	err := repo.CrearEquipo(ctx, otro)

	if !errors.Is(err, application.ErrNombreDeEquipoDuplicado) {
		t.Fatalf("esperaba ErrNombreDeEquipoDuplicado, obtuve %v", err)
	}
}

// A diferencia de un número de serie, que es único de fábrica, "Cargador 1"
// es un apodo: si el cargador se rompe y compran otro lo van a seguir
// llamando igual. Sin esto el alta rebotaría con un 409 sin salida.
func TestPostgresRepo_EquipoSuelto_NombreSeReusaTrasLaBaja(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	viejo := crearEquipoSueltoDeTest(t, repo, "CARGADOR", "Cargador 1", false)
	if err := viejo.DarDeBaja(time.Now().UTC()); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.GuardarEquipo(ctx, viejo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	nuevo, _ := domain.NuevoEquipoSuelto(NuevoID(), "CARGADOR", "Cargador 1", false, time.Now().UTC())
	if err := repo.CrearEquipo(ctx, nuevo); err != nil {
		t.Fatalf("el nombre de un equipo dado de baja debería poder reusarse: %v", err)
	}
}

// El listado de la sección "Otros equipos": trae lo que no cuelga de ningún
// carro y nada más. Si se colara una PC de carro, el proyector aparecería
// junto a las 64 computadoras y la sección perdería sentido.
func TestPostgresRepo_ListarEquiposSueltos_NoTraeLasDeCarro(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	crearEquipoDeCarroDeTest(t, repo, carro.ID, 3, "SERIE-EQ-2")
	crearEquipoSueltoDeTest(t, repo, "PROYECTOR", "Proyector Epson", true)
	crearEquipoSueltoDeTest(t, repo, "CARGADOR", "Cargador 1", false)

	sueltos, err := repo.ListarEquiposSueltos(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	var nombres []string
	for _, e := range sueltos {
		if e.EstaEnUnCarro() {
			t.Errorf("se coló una PC de carro: %+v", e)
		}
		nombres = append(nombres, e.Nombre)
	}
	// Ordenados por tipo y después por nombre: CARGADOR antes que PROYECTOR.
	if len(nombres) != 2 || nombres[0] != "Cargador 1" || nombres[1] != "Proyector Epson" {
		t.Errorf("esperaba [Cargador 1, Proyector Epson], obtuve %v", nombres)
	}
}
