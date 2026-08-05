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

	"github.com/ramiro/sgrc/internal/availability/application"
	"github.com/ramiro/sgrc/internal/availability/domain"
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

func crearUsuarioDeTest(t *testing.T, pool *pgxpool.Pool, nombre, apellido, rol, estado string) string {
	t.Helper()
	id := NuevoID()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO usuario (id, nombre, apellido, email, password_hash, rol, estado)
		VALUES ($1, $2, $3, $4, 'hash-de-prueba', $5, $6)
	`, id, nombre, apellido, id+"@escuela.edu.ar", rol, estado)
	if err != nil {
		t.Fatalf("no se pudo crear usuario de prueba: %v", err)
	}
	return id
}

// ── BloqueHorario ───────────────────────────────────────────────────────

func TestPostgresRepo_CrearYListarBloques_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	adminID := crearUsuarioDeTest(t, pool, "Ada", "Lovelace", "ADMIN", "APROBADA")

	b, err := domain.NuevoBloqueHorario(NuevoID(), adminID, domain.Lunes, 8*time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearBloque(context.Background(), b); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	bloques, err := repo.ListarBloquesDeUsuario(context.Background(), adminID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(bloques) != 1 || bloques[0].DiaSemana != domain.Lunes {
		t.Fatalf("bloque no coincide: %+v", bloques)
	}
	if bloques[0].HoraInicio != 8*time.Hour || bloques[0].HoraFin != 12*time.Hour {
		t.Errorf("horas no coinciden tras el round-trip TIME↔Duration: %+v", bloques[0])
	}
}

func TestPostgresRepo_BuscarBloqueDeUsuario_DeOtroUsuario_ErrBloqueNoEncontrado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	dueño := crearUsuarioDeTest(t, pool, "Ada", "Lovelace", "ADMIN", "APROBADA")
	intruso := crearUsuarioDeTest(t, pool, "Alan", "Turing", "ADMIN", "APROBADA")

	b, _ := domain.NuevoBloqueHorario(NuevoID(), dueño, domain.Lunes, 8*time.Hour, 12*time.Hour)
	repo.CrearBloque(context.Background(), b)

	_, err := repo.BuscarBloqueDeUsuario(context.Background(), b.ID, intruso)

	if err != application.ErrBloqueNoEncontrado {
		t.Fatalf("esperaba ErrBloqueNoEncontrado (titularidad ajena), obtuve %v", err)
	}
}

func TestPostgresRepo_GuardarBloque_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	adminID := crearUsuarioDeTest(t, pool, "Ada", "Lovelace", "ADMIN", "APROBADA")

	b, _ := domain.NuevoBloqueHorario(NuevoID(), adminID, domain.Lunes, 8*time.Hour, 12*time.Hour)
	repo.CrearBloque(context.Background(), b)

	actualizado, _ := domain.NuevoBloqueHorario(b.ID, adminID, domain.Martes, 9*time.Hour, 11*time.Hour)
	if err := repo.GuardarBloque(context.Background(), actualizado); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recargado, err := repo.BuscarBloqueDeUsuario(context.Background(), b.ID, adminID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if recargado.DiaSemana != domain.Martes || recargado.HoraInicio != 9*time.Hour || recargado.HoraFin != 11*time.Hour {
		t.Errorf("no se persistió la actualización: %+v", recargado)
	}
}

func TestPostgresRepo_GuardarBloque_DeOtroUsuario_ErrBloqueNoEncontrado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	dueño := crearUsuarioDeTest(t, pool, "Ada", "Lovelace", "ADMIN", "APROBADA")
	intruso := crearUsuarioDeTest(t, pool, "Alan", "Turing", "ADMIN", "APROBADA")

	b, _ := domain.NuevoBloqueHorario(NuevoID(), dueño, domain.Lunes, 8*time.Hour, 12*time.Hour)
	repo.CrearBloque(context.Background(), b)

	// Un intento de "editar" el bloque de otro usuario, forzando su propio
	// UsuarioID en la entidad — la query WHERE id=$1 AND usuario_id=$2 no
	// debe afectar ninguna fila.
	suplantado, _ := domain.NuevoBloqueHorario(b.ID, intruso, domain.Martes, 9*time.Hour, 11*time.Hour)
	err := repo.GuardarBloque(context.Background(), suplantado)

	if err != application.ErrBloqueNoEncontrado {
		t.Fatalf("esperaba ErrBloqueNoEncontrado, obtuve %v", err)
	}

	// El bloque original no debería haberse tocado.
	original, _ := repo.BuscarBloqueDeUsuario(context.Background(), b.ID, dueño)
	if original.DiaSemana != domain.Lunes {
		t.Errorf("el bloque del dueño real no debería haber cambiado: %+v", original)
	}
}

func TestPostgresRepo_EliminarBloqueDeUsuario_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	adminID := crearUsuarioDeTest(t, pool, "Ada", "Lovelace", "ADMIN", "APROBADA")

	b, _ := domain.NuevoBloqueHorario(NuevoID(), adminID, domain.Lunes, 8*time.Hour, 12*time.Hour)
	repo.CrearBloque(context.Background(), b)

	if err := repo.EliminarBloqueDeUsuario(context.Background(), b.ID, adminID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	bloques, _ := repo.ListarBloquesDeUsuario(context.Background(), adminID)
	if len(bloques) != 0 {
		t.Errorf("el bloque debería haberse eliminado, quedan %d", len(bloques))
	}
}

func TestPostgresRepo_EliminarBloqueDeUsuario_DeOtroUsuario_NoElimina(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	dueño := crearUsuarioDeTest(t, pool, "Ada", "Lovelace", "ADMIN", "APROBADA")
	intruso := crearUsuarioDeTest(t, pool, "Alan", "Turing", "ADMIN", "APROBADA")

	b, _ := domain.NuevoBloqueHorario(NuevoID(), dueño, domain.Lunes, 8*time.Hour, 12*time.Hour)
	repo.CrearBloque(context.Background(), b)

	err := repo.EliminarBloqueDeUsuario(context.Background(), b.ID, intruso)

	if err != application.ErrBloqueNoEncontrado {
		t.Fatalf("esperaba ErrBloqueNoEncontrado, obtuve %v", err)
	}
	bloques, _ := repo.ListarBloquesDeUsuario(context.Background(), dueño)
	if len(bloques) != 1 {
		t.Error("el bloque del dueño real no debería haberse eliminado")
	}
}

// ── Excepcion ───────────────────────────────────────────────────────────

func TestPostgresRepo_GuardarYBuscarExcepcion_NoDisponible_RoundTrip(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	adminID := crearUsuarioDeTest(t, pool, "Ada", "Lovelace", "ADMIN", "APROBADA")
	fecha := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)

	e, err := domain.NuevaExcepcion(NuevoID(), adminID, fecha, domain.NoDisponible, nil, nil, nil)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.GuardarExcepcion(context.Background(), e); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recargada, err := repo.BuscarExcepcionDeFecha(context.Background(), adminID, fecha)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if recargada == nil {
		t.Fatal("esperaba encontrar la excepción recién guardada")
	}
	if recargada.Tipo != domain.NoDisponible || recargada.HoraInicio != nil || recargada.HoraFin != nil {
		t.Errorf("excepción NO_DISPONIBLE no debería tener horario: %+v", recargada)
	}
}

func TestPostgresRepo_GuardarYBuscarExcepcion_HorarioModificado_RoundTrip(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	adminID := crearUsuarioDeTest(t, pool, "Ada", "Lovelace", "ADMIN", "APROBADA")
	fecha := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)

	horaInicio, horaFin := 9*time.Hour, 11*time.Hour
	motivo := "reunión externa"
	e, err := domain.NuevaExcepcion(NuevoID(), adminID, fecha, domain.HorarioModificado, &horaInicio, &horaFin, &motivo)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.GuardarExcepcion(context.Background(), e); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recargada, err := repo.BuscarExcepcionDeFecha(context.Background(), adminID, fecha)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if recargada.HoraInicio == nil || *recargada.HoraInicio != 9*time.Hour {
		t.Errorf("HoraInicio no coincide tras el round-trip: %+v", recargada.HoraInicio)
	}
	if recargada.HoraFin == nil || *recargada.HoraFin != 11*time.Hour {
		t.Errorf("HoraFin no coincide tras el round-trip: %+v", recargada.HoraFin)
	}
	if recargada.Motivo == nil || *recargada.Motivo != "reunión externa" {
		t.Errorf("Motivo no coincide: %+v", recargada.Motivo)
	}
}

func TestPostgresRepo_GuardarExcepcion_Upsert_ReemplazaLaAnterior(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	adminID := crearUsuarioDeTest(t, pool, "Ada", "Lovelace", "ADMIN", "APROBADA")
	fecha := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)

	primera, _ := domain.NuevaExcepcion(NuevoID(), adminID, fecha, domain.NoDisponible, nil, nil, nil)
	if err := repo.GuardarExcepcion(context.Background(), primera); err != nil {
		t.Fatalf("primera carga no debería fallar: %v", err)
	}

	horaInicio, horaFin := 9*time.Hour, 11*time.Hour
	segunda, _ := domain.NuevaExcepcion(NuevoID(), adminID, fecha, domain.HorarioModificado, &horaInicio, &horaFin, nil)
	if err := repo.GuardarExcepcion(context.Background(), segunda); err != nil {
		t.Fatalf("segunda carga (upsert) no debería fallar: %v", err)
	}

	recargada, err := repo.BuscarExcepcionDeFecha(context.Background(), adminID, fecha)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if recargada.ID != segunda.ID || recargada.Tipo != domain.HorarioModificado {
		t.Errorf("la segunda carga debería haber reemplazado la primera por completo: %+v", recargada)
	}

	var total int
	pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM horario_admin_excepcion WHERE usuario_id = $1 AND fecha = $2`,
		adminID, fecha).Scan(&total)
	if total != 1 {
		t.Errorf("UNIQUE(usuario_id, fecha) debería garantizar una sola fila, hay %d", total)
	}
}

func TestPostgresRepo_BuscarExcepcionDeFecha_NoExiste_DevuelveNilSinError(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	adminID := crearUsuarioDeTest(t, pool, "Ada", "Lovelace", "ADMIN", "APROBADA")

	e, err := repo.BuscarExcepcionDeFecha(context.Background(), adminID, time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC))

	if err != nil {
		t.Fatalf("no tener excepción cargada no debería ser un error: %v", err)
	}
	if e != nil {
		t.Errorf("esperaba nil, obtuve %+v", e)
	}
}

// ── ListadorAdminsPostgres ──────────────────────────────────────────────

func TestListadorAdminsPostgres_SoloAdminsAprobados_ConNombreYApellido(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	admin1 := crearUsuarioDeTest(t, pool, "Ada", "Lovelace", "ADMIN", "APROBADA")
	admin2 := crearUsuarioDeTest(t, pool, "Alan", "Turing", "ADMIN", "APROBADA")
	crearUsuarioDeTest(t, pool, "Grace", "Hopper", "ADMIN", "PENDIENTE")    // no cuenta
	crearUsuarioDeTest(t, pool, "Linus", "Torvalds", "DOCENTE", "APROBADA") // no cuenta

	listador := NewListadorAdminsPostgres(pool)
	admins, err := listador.AdminsAprobados(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(admins) != 2 {
		t.Fatalf("esperaba 2 admins aprobados, obtuve %d: %+v", len(admins), admins)
	}
	porID := make(map[string]application.AdminInfo, 2)
	for _, a := range admins {
		porID[a.ID] = a
	}
	if porID[admin1].Nombre != "Ada" || porID[admin1].Apellido != "Lovelace" {
		t.Errorf("datos de admin1 incorrectos: %+v", porID[admin1])
	}
	if porID[admin2].Nombre != "Alan" || porID[admin2].Apellido != "Turing" {
		t.Errorf("datos de admin2 incorrectos: %+v", porID[admin2])
	}
}

// ── ID inválido — mismo patrón de regresión de notification/reservation ──

func TestPostgresRepo_IDConFormatoInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	casos := []struct {
		nombre string
		fn     func() error
	}{
		{"ListarBloquesDeUsuario", func() error { _, err := repo.ListarBloquesDeUsuario(ctx, "USUARIO_ID"); return err }},
		{"BuscarBloqueDeUsuario", func() error { _, err := repo.BuscarBloqueDeUsuario(ctx, "BLOQUE_ID", "USUARIO_ID"); return err }},
		{"GuardarBloque_IDInvalido", func() error {
			b := &domain.BloqueHorario{ID: "BLOQUE_ID", UsuarioID: "USUARIO_ID", DiaSemana: domain.Lunes, HoraInicio: time.Hour, HoraFin: 2 * time.Hour}
			return repo.GuardarBloque(ctx, b)
		}},
		{"EliminarBloqueDeUsuario", func() error { return repo.EliminarBloqueDeUsuario(ctx, "BLOQUE_ID", "USUARIO_ID") }},
	}

	for _, c := range casos {
		err := c.fn()
		if err != application.ErrIDInvalido {
			t.Errorf("%s: esperaba application.ErrIDInvalido, obtuve %v", c.nombre, err)
		}
	}
}
