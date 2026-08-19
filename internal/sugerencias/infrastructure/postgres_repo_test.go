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

	"github.com/ramiro/sgrc/internal/shared/paginacion"
	"github.com/ramiro/sgrc/internal/shared/testdb"
	"github.com/ramiro/sgrc/internal/sugerencias/domain"
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

func crearUsuarioDeTest(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := NuevoID()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO usuario (id, nombre, apellido, email, password_hash, rol, estado)
		VALUES ($1, 'Ada', 'Lovelace', $2, 'hash-de-prueba', 'DOCENTE', 'APROBADA')
	`, id, id+"@escuela.edu.ar")
	if err != nil {
		t.Fatalf("no se pudo crear usuario de prueba: %v", err)
	}
	return id
}

func hiloDePrueba(t *testing.T, usuarioID string, ahora time.Time) *domain.Sugerencia {
	t.Helper()
	s, err := domain.Nueva(NuevoID(), NuevoID(), usuarioID, domain.TipoAyuda,
		"No arranca la PC 3", "la enciendo y no pasa nada", "/reservas", "1.10.0", ahora)
	if err != nil {
		t.Fatalf("armando el hilo de prueba: %v", err)
	}
	return s
}

// El caso central: el hilo y su primer mensaje se escriben juntos y vuelven
// juntos. Es lo que rompería si el INSERT de la transacción quedara a medias.
func TestPostgresRepo_CrearYLeerConSuMensaje(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	usuarioID := crearUsuarioDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	s := hiloDePrueba(t, usuarioID, ahora)
	if err := repo.Crear(ctx, s); err != nil {
		t.Fatalf("creando: %v", err)
	}

	leido, err := repo.BuscarPorID(ctx, s.ID)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	if leido.Asunto != "No arranca la PC 3" || leido.Tipo != domain.TipoAyuda {
		t.Errorf("no volvió lo que se guardó: %+v", leido)
	}
	if len(leido.Mensajes) != 1 || leido.Mensajes[0].Texto != "la enciendo y no pasa nada" {
		t.Fatalf("el primer mensaje tenía que volver con el hilo: %+v", leido.Mensajes)
	}
	if leido.Mensajes[0].DeAdmin {
		t.Error("el primer mensaje no es de administración")
	}
	if leido.Pantalla != "/reservas" || leido.Version != "1.10.0" {
		t.Errorf("se perdió desde dónde se escribió: %+v", leido)
	}
}

// Los mensajes vuelven en orden cronológico, que es como se lee una
// conversación.
func TestPostgresRepo_AgregarMensaje_EnOrdenYMoviendoLaActividad(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	usuarioID := crearUsuarioDeTest(t, pool)
	admin := crearUsuarioDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	s := hiloDePrueba(t, usuarioID, ahora)
	if err := repo.Crear(ctx, s); err != nil {
		t.Fatalf("creando: %v", err)
	}

	despues := ahora.Add(2 * time.Hour)
	if err := s.Responder(NuevoID(), admin, true, "vamos para allá", despues); err != nil {
		t.Fatalf("respondiendo: %v", err)
	}
	if err := repo.AgregarMensaje(ctx, s, s.UltimoMensaje()); err != nil {
		t.Fatalf("guardando el mensaje: %v", err)
	}

	leido, err := repo.BuscarPorID(ctx, s.ID)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	if len(leido.Mensajes) != 2 {
		t.Fatalf("esperaba dos mensajes, hay %d", len(leido.Mensajes))
	}
	if leido.Mensajes[0].DeAdmin || !leido.Mensajes[1].DeAdmin {
		t.Errorf("el orden o el lado de los mensajes está mal: %+v", leido.Mensajes)
	}
	if !leido.UltimaActividadEn.Equal(despues) {
		t.Errorf("la actividad no se movió: esperaba %v, obtuve %v", despues, leido.UltimaActividadEn)
	}
	// Contestar no cierra.
	if leido.Estado != domain.Abierta {
		t.Errorf("el hilo tendría que seguir abierto, quedó %q", leido.Estado)
	}
}

// La bandeja del Admin se ordena por actividad, no por creación: un hilo
// viejo al que le acaban de escribir va primero. Y trae los mensajes de todos
// los hilos de la página en una sola consulta.
func TestPostgresRepo_ListarTodas_OrdenaPorActividadYTraeLosMensajes(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	usuarioID := crearUsuarioDeTest(t, pool)
	admin := crearUsuarioDeTest(t, pool)

	ayer := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Microsecond)
	viejo := hiloDePrueba(t, usuarioID, ayer)
	if err := repo.Crear(ctx, viejo); err != nil {
		t.Fatalf("creando el viejo: %v", err)
	}

	reciente := hiloDePrueba(t, usuarioID, time.Now().UTC().Truncate(time.Microsecond))
	if err := repo.Crear(ctx, reciente); err != nil {
		t.Fatalf("creando el reciente: %v", err)
	}

	// Al viejo le contestan recién ahora: pasa a ser el que se movió último.
	ahora := time.Now().UTC().Add(time.Minute).Truncate(time.Microsecond)
	if err := viejo.Responder(NuevoID(), admin, true, "recién lo miro", ahora); err != nil {
		t.Fatalf("respondiendo: %v", err)
	}
	if err := repo.AgregarMensaje(ctx, viejo, viejo.UltimoMensaje()); err != nil {
		t.Fatalf("guardando el mensaje: %v", err)
	}

	hilos, total, err := repo.ListarTodas(ctx, true, paginacion.PorDefecto())
	if err != nil {
		t.Fatalf("listando: %v", err)
	}
	if total != 2 || len(hilos) != 2 {
		t.Fatalf("esperaba 2 hilos, obtuve %d (total %d)", len(hilos), total)
	}
	if hilos[0].ID != viejo.ID {
		t.Errorf("el hilo que se movió último tendría que ir primero")
	}
	// Cada hilo con sus mensajes, sin una consulta por hilo.
	if len(hilos[0].Mensajes) != 2 || len(hilos[1].Mensajes) != 1 {
		t.Errorf("los mensajes no volvieron con cada hilo: %d y %d",
			len(hilos[0].Mensajes), len(hilos[1].Mensajes))
	}
}

func TestPostgresRepo_ListarTodas_FiltraLasResueltas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	usuarioID := crearUsuarioDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	abierta := hiloDePrueba(t, usuarioID, ahora)
	if err := repo.Crear(ctx, abierta); err != nil {
		t.Fatalf("creando: %v", err)
	}
	cerrada := hiloDePrueba(t, usuarioID, ahora)
	if err := repo.Crear(ctx, cerrada); err != nil {
		t.Fatalf("creando: %v", err)
	}
	if err := cerrada.MarcarResuelta(ahora); err != nil {
		t.Fatalf("cerrando: %v", err)
	}
	if err := repo.GuardarEstado(ctx, cerrada); err != nil {
		t.Fatalf("guardando el estado: %v", err)
	}

	soloAbiertas, _, err := repo.ListarTodas(ctx, true, paginacion.PorDefecto())
	if err != nil {
		t.Fatalf("listando: %v", err)
	}
	if len(soloAbiertas) != 1 || soloAbiertas[0].ID != abierta.ID {
		t.Fatalf("esperaba solo la abierta, obtuve %d", len(soloAbiertas))
	}

	todas, _, err := repo.ListarTodas(ctx, false, paginacion.PorDefecto())
	if err != nil {
		t.Fatalf("listando: %v", err)
	}
	if len(todas) != 2 {
		t.Errorf("sin filtro tendrían que venir las dos, vinieron %d", len(todas))
	}
}

func TestPostgresRepo_ListarDeUsuario_SoloLasSuyas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	uno := crearUsuarioDeTest(t, pool)
	otro := crearUsuarioDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	if err := repo.Crear(ctx, hiloDePrueba(t, uno, ahora)); err != nil {
		t.Fatalf("creando: %v", err)
	}
	if err := repo.Crear(ctx, hiloDePrueba(t, otro, ahora)); err != nil {
		t.Fatalf("creando: %v", err)
	}

	hilos, total, err := repo.ListarDeUsuario(ctx, uno, paginacion.PorDefecto())
	if err != nil {
		t.Fatalf("listando: %v", err)
	}
	if total != 1 || len(hilos) != 1 || hilos[0].UsuarioID != uno {
		t.Fatalf("esperaba solo el hilo de uno, obtuve %d (total %d)", len(hilos), total)
	}
	if len(hilos[0].Mensajes) != 1 {
		t.Errorf("el hilo tendría que venir con su mensaje")
	}
}

// Borrar la cuenta se lleva sus conversaciones, y el CHECK del tipo rechaza
// cualquier cosa que la aplicación no conozca.
func TestPostgresRepo_LaBaseSostieneLasReglas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	usuarioID := crearUsuarioDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	s := hiloDePrueba(t, usuarioID, ahora)
	if err := repo.Crear(ctx, s); err != nil {
		t.Fatalf("creando: %v", err)
	}

	// Los tres tipos que conoce la aplicación entran.
	for _, tipo := range domain.Tipos() {
		hilo, err := domain.Nueva(NuevoID(), NuevoID(), usuarioID, tipo, "Asunto", "texto", "", "", ahora)
		if err != nil {
			t.Fatalf("armando el hilo: %v", err)
		}
		if err := repo.Crear(ctx, hilo); err != nil {
			t.Fatalf("la base rechazó el tipo %s que la aplicación conoce: %v", tipo, err)
		}
	}

	if _, err := pool.Exec(ctx, `DELETE FROM usuario WHERE id = $1`, usuarioID); err != nil {
		t.Fatalf("borrando el usuario: %v", err)
	}

	var mensajes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sugerencia_mensaje`).Scan(&mensajes); err != nil {
		t.Fatalf("contando mensajes: %v", err)
	}
	if mensajes != 0 {
		t.Errorf("borrar la cuenta tendría que llevarse sus mensajes, quedaron %d", mensajes)
	}
}
