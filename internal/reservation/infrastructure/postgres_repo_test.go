//go:build integration

package infrastructure

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
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

// ── Helpers para las FK que reservation necesita (ciclo→curso→materia,
// carro→equipo, usuario) — inserción directa por SQL, sin pasar por
// auth/academic/inventory a propósito (reservation no los importa).

// contadorAnioDeTest asegura un año único por llamada a crearMateriaDeTest
// dentro de un mismo test — ciclo_lectivo.anio tiene una constraint UNIQUE, y
// varios tests crean más de una materia (con su propio ciclo/curso) en la
// misma corrida, así que un año fijo (2026) chocaba en la segunda llamada.
var contadorAnioDeTest int32

func crearMateriaDeTest(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	anio := int(atomic.AddInt32(&contadorAnioDeTest, 1)) + 3000
	cicloID := NuevoID()
	cursoID := NuevoID()
	materiaID := NuevoID()

	// activo=false a propósito: este fixture puede crearse varias veces por test
	// (una por materia independiente), y solo puede haber UN ciclo activo a la
	// vez en toda la tabla (idx_ciclo_lectivo_activo_unico, RF-02.1) — estos
	// tests no necesitan que el ciclo esté activo para nada, así que evitamos
	// esa constraint directamente.
	if _, err := pool.Exec(ctx, `INSERT INTO ciclo_lectivo (id, anio, activo) VALUES ($1, $2, false)`, cicloID, anio); err != nil {
		t.Fatalf("no se pudo crear ciclo de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO curso (id, ciclo_lectivo_id, nombre) VALUES ($1, $2, '1°A')`, cursoID, cicloID); err != nil {
		t.Fatalf("no se pudo crear curso de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO materia (id, curso_id, nombre) VALUES ($1, $2, 'Matemáticas')`, materiaID, cursoID); err != nil {
		t.Fatalf("no se pudo crear materia de prueba: %v", err)
	}
	return materiaID
}

func crearUsuarioDeTest(t *testing.T, pool *pgxpool.Pool, rol, estado string) string {
	t.Helper()
	id := NuevoID()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO usuario (id, nombre, apellido, email, password_hash, rol, estado)
		VALUES ($1, 'Ada', 'Lovelace', $2, 'hash-de-prueba', $3, $4)
	`, id, id+"@escuela.edu.ar", rol, estado)
	if err != nil {
		t.Fatalf("no se pudo crear usuario de prueba: %v", err)
	}
	return id
}

func crearEquipoDeCarroDeTest(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	carroID := NuevoID()
	equipoID := NuevoID()

	if _, err := pool.Exec(ctx, `INSERT INTO carro (id, nombre) VALUES ($1, $2)`, carroID, "Carro-"+carroID[:8]); err != nil {
		t.Fatalf("no se pudo crear carro de prueba: %v", err)
	}
	numeroSerie := fmt.Sprintf("SERIE-%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx,
		`INSERT INTO equipo (id, carro_id, identificador, numero_serie, estado) VALUES ($1, $2, 1, $3, 'DISPONIBLE')`,
		equipoID, carroID, numeroSerie,
	); err != nil {
		t.Fatalf("no se pudo crear PC de prueba: %v", err)
	}
	return equipoID
}

func nuevoReservaGrupoDeTest(materiaID string, fecha time.Time, horaInicio, horaFin time.Duration) *domain.ReservaGrupo {
	g, _ := domain.NuevoReservaGrupo(NuevoID(), materiaID, nil, "Ada Lovelace", fecha, horaInicio, horaFin, nil, time.Now().UTC().Truncate(time.Microsecond))
	return g
}

// ── ReservaGrupo + Reserva — round trip básico ─────────────────────────

func TestPostgresRepo_CrearYBuscarReservaGrupo_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)

	g := nuevoReservaGrupoDeTest(materiaID, time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour)
	if err := repo.CrearReservaGrupo(context.Background(), g); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	encontrado, err := repo.BuscarReservaGrupoPorID(context.Background(), g.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if encontrado.HoraInicio != 8*time.Hour || encontrado.HoraFin != 9*time.Hour {
		t.Errorf("horario no coincide: %+v", encontrado)
	}
}

func TestPostgresRepo_CrearYBuscarReserva_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)

	g := nuevoReservaGrupoDeTest(materiaID, time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour)
	if err := repo.CrearReservaGrupo(context.Background(), g); err != nil {
		t.Fatalf("no debería fallar creando grupo: %v", err)
	}

	res, err := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada Lovelace", nil,
		g.Fecha, g.HoraInicio, g.HoraFin, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	encontrada, err := repo.BuscarReservaPorID(context.Background(), res.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if encontrada.EquipoID != equipoID || encontrada.Estado != domain.ReservaConfirmada {
		t.Errorf("reserva encontrada no coincide: %+v", encontrada)
	}
}

// ── Paginación de ListarReservas ─────────────────────────────────────── Va
// contra Postgres real y no solo contra el fake porque lo que puede salir mal
// es el SQL: que los $n del LIMIT/OFFSET no pisen los de los filtros
// dinámicos, y que COUNT(*) OVER() cuente antes del recorte.

func TestPostgresRepo_ListarReservas_PaginaYTotal(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// Cinco días distintos sobre la misma PC: sin fechas distintas chocarían
	// contra la constraint EXCLUDE.
	for i := 0; i < 5; i++ {
		fecha := time.Date(2026, 3, 2+i, 0, 0, 0, 0, time.UTC)
		g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
		if err := repo.CrearReservaGrupo(ctx, g); err != nil {
			t.Fatalf("creando grupo: %v", err)
		}
		res, err := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada Lovelace", nil,
			fecha, 8*time.Hour, 9*time.Hour, ahora)
		if err != nil {
			t.Fatalf("error de dominio inesperado: %v", err)
		}
		if err := repo.CrearReserva(ctx, res); err != nil {
			t.Fatalf("creando reserva: %v", err)
		}
	}

	// Con un filtro puesto: es el caso donde el LIMIT/OFFSET tiene que ir
	// DESPUÉS de los $n del WHERE.
	filtro := application.FiltroReservas{
		EquipoID: &equipoID,
		Pagina:   paginacion.Pagina{Numero: 1, Tamanio: 2},
	}

	primera, total, err := repo.ListarReservas(ctx, filtro)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(primera) != 2 || total != 5 {
		t.Fatalf("primera página: %d filas, total %d; esperaba 2 y 5", len(primera), total)
	}
	if primera[0].Reserva.Fecha.Day() != 2 {
		t.Errorf("el orden por fecha no se respetó: primera fila del día %d", primera[0].Reserva.Fecha.Day())
	}

	// Recorrer todas las páginas no puede repetir ni saltear ninguna fila:
	// es lo que rompe si el ORDER BY no es determinista.
	vistas := map[string]bool{}
	for pag := 1; pag <= 3; pag++ {
		filtro.Pagina = paginacion.Pagina{Numero: pag, Tamanio: 2}
		filas, _, err := repo.ListarReservas(ctx, filtro)
		if err != nil {
			t.Fatalf("no debería fallar: %v", err)
		}
		for _, f := range filas {
			if vistas[f.Reserva.ID] {
				t.Errorf("la reserva %s apareció en más de una página", f.Reserva.ID)
			}
			vistas[f.Reserva.ID] = true
		}
	}
	if len(vistas) != 5 {
		t.Errorf("recorriendo las páginas vi %d reservas distintas, esperaba 5", len(vistas))
	}

	// Más allá del final: sin filas de las que leer COUNT(*) OVER(), el total
	// sale de la consulta de respaldo.
	filtro.Pagina = paginacion.Pagina{Numero: 9, Tamanio: 2}
	vacia, total, err := repo.ListarReservas(ctx, filtro)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(vacia) != 0 || total != 5 {
		t.Fatalf("página vacía: %d filas, total %d; esperaba 0 y 5", len(vacia), total)
	}
}

// ── El test más importante de todo reservation: la constraint EXCLUDE ──

func TestPostgresRepo_Solapamiento_ConstraintDeLaBase(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g1 := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g1)
	res1, _ := domain.NuevaReservaNormal(NuevoID(), g1.ID, equipoID, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	if err := repo.CrearReserva(context.Background(), res1); err != nil {
		t.Fatalf("la primera reserva no debería fallar: %v", err)
	}

	g2 := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour+30*time.Minute, 9*time.Hour+30*time.Minute)
	repo.CrearReservaGrupo(context.Background(), g2)
	res2, _ := domain.NuevaReservaNormal(NuevoID(), g2.ID, equipoID, materiaID, "Ada", nil, fecha, 8*time.Hour+30*time.Minute, 9*time.Hour+30*time.Minute, ahora)

	err := repo.CrearReserva(context.Background(), res2)
	if err != application.ErrSolapamiento {
		t.Fatalf("esperaba application.ErrSolapamiento (constraint EXCLUDE), obtuve %v", err)
	}
}

func TestPostgresRepo_SinSolapamiento_HorariosDistintos_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g1 := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g1)
	res1, _ := domain.NuevaReservaNormal(NuevoID(), g1.ID, equipoID, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	if err := repo.CrearReserva(context.Background(), res1); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Horario consecutivo, sin solapar (9-10hs justo después de 8-9hs).
	g2 := nuevoReservaGrupoDeTest(materiaID, fecha, 9*time.Hour, 10*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g2)
	res2, _ := domain.NuevaReservaNormal(NuevoID(), g2.ID, equipoID, materiaID, "Ada", nil, fecha, 9*time.Hour, 10*time.Hour, ahora)

	if err := repo.CrearReserva(context.Background(), res2); err != nil {
		t.Fatalf("horarios consecutivos sin solapar no deberían fallar: %v", err)
	}
}

func TestPostgresRepo_SolapamientoEnEquiposDistintas_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	pc1 := crearEquipoDeCarroDeTest(t, pool)
	pc2 := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)

	res1, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pc1, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	res2, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pc2, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)

	if err := repo.CrearReserva(context.Background(), res1); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.CrearReserva(context.Background(), res2); err != nil {
		t.Fatalf("mismo horario en otra PC no debería solapar: %v", err)
	}
}

func TestPostgresRepo_GuardarReserva_Cancelar(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	adminID := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	repo.CrearReserva(context.Background(), res)

	if err := res.Cancelar(&adminID, "PC rota", ahora); err != nil {
		t.Fatalf("transición de dominio inválida: %v", err)
	}
	if err := repo.GuardarReserva(context.Background(), res); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recargada, err := repo.BuscarReservaPorID(context.Background(), res.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if recargada.Estado != domain.ReservaCancelada {
		t.Errorf("estado no persistido: %s", recargada.Estado)
	}
	if recargada.MotivoCancelacion == nil || *recargada.MotivoCancelacion != "PC rota" {
		t.Errorf("motivo no persistido: %+v", recargada.MotivoCancelacion)
	}
}

// TestPostgresRepo_GuardarReserva_CambiarDeEquipo cubre RF-08.14 en el único
// lugar donde podía fallar.
func TestPostgresRepo_GuardarReserva_CambiarDeEquipo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoViejo := crearEquipoDeCarroDeTest(t, pool)
	equipoNuevo := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoViejo, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	repo.CrearReserva(context.Background(), res)

	res.EquipoID = equipoNuevo
	if err := repo.GuardarReserva(context.Background(), res); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recargada, err := repo.BuscarReservaPorID(context.Background(), res.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if recargada.EquipoID != equipoNuevo {
		t.Errorf("el equipo no se persistió: quedó en %s y se esperaba %s",
			recargada.EquipoID, equipoNuevo)
	}
}

// TestPostgresRepo_GuardarReserva_MoverAUnEquipoOcupado_LoRechazaLaBase: al
// escribir el equipo, este UPDATE pasó a poder violar la constraint EXCLUDE
// de anti-solapamiento, cosa que antes no podía.
func TestPostgresRepo_GuardarReserva_MoverAUnEquipoOcupado_LoRechazaLaBase(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	materiaID := crearMateriaDeTest(t, pool)
	equipoA := crearEquipoDeCarroDeTest(t, pool)
	equipoB := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// Dos reservas en la MISMA franja, cada una con su equipo.
	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(ctx, g)
	resA, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoA, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	repo.CrearReserva(ctx, resA)
	resB, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoB, materiaID, "Grace", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	repo.CrearReserva(ctx, resB)

	// Mover la primera al equipo que ya tiene la segunda.
	resA.EquipoID = equipoB
	if err := repo.GuardarReserva(ctx, resA); err == nil {
		t.Fatal("la base tenía que rechazar el solapamiento y no lo hizo")
	}

	// Y el rechazo no puede dejar la fila a medias.
	recargada, err := repo.BuscarReservaPorID(ctx, resA.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if recargada.EquipoID != equipoA {
		t.Errorf("la reserva quedó movida pese al rechazo: %s", recargada.EquipoID)
	}
}

// TestPostgresRepo_ReservaVencida_ApareceEnElListado confirma que la
// comparación fecha+hora_fin funciona igual que la de la constraint EXCLUDE:
// aritmética date+time, no comparación de texto.
func TestPostgresRepo_ReservaVencida_ApareceEnElListado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)

	ayer := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	g := nuevoReservaGrupoDeTest(materiaID, ayer, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil, ayer, 8*time.Hour, 9*time.Hour, time.Now().UTC())
	repo.CrearReserva(context.Background(), res)

	vencidas, err := repo.ListarReservasConfirmadasVencidas(context.Background(), time.Now().UTC(), 100)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	encontrada := false
	for _, v := range vencidas {
		if v.ID == res.ID {
			encontrada = true
		}
	}
	if !encontrada {
		t.Errorf("la reserva de ayer debería aparecer como vencida, vencidas: %+v", vencidas)
	}
}

func TestPostgresRepo_ReservaFutura_NoApareceComoVencida(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)

	manana := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour)
	g := nuevoReservaGrupoDeTest(materiaID, manana, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil, manana, 8*time.Hour, 9*time.Hour, time.Now().UTC())
	repo.CrearReserva(context.Background(), res)

	vencidas, err := repo.ListarReservasConfirmadasVencidas(context.Background(), time.Now().UTC(), 100)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	for _, v := range vencidas {
		if v.ID == res.ID {
			t.Errorf("una reserva de mañana no debería aparecer como vencida")
		}
	}
}

func TestPostgresRepo_ListarReservasFuturasDeMateria(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	otraMateriaID := crearMateriaDeTest(t, pool)
	pc1 := crearEquipoDeCarroDeTest(t, pool)
	pc2 := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// Dos reservas de la materia que nos importa (distintas PCs, mismo grupo lógico).
	g1 := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g1)
	res1, _ := domain.NuevaReservaNormal(NuevoID(), g1.ID, pc1, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	repo.CrearReserva(context.Background(), res1)

	// Una reserva de OTRA materia — no debería aparecer en el resultado.
	g2 := nuevoReservaGrupoDeTest(otraMateriaID, fecha, 10*time.Hour, 11*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g2)
	res2, _ := domain.NuevaReservaNormal(NuevoID(), g2.ID, pc2, otraMateriaID, "Ada", nil, fecha, 10*time.Hour, 11*time.Hour, ahora)
	repo.CrearReserva(context.Background(), res2)

	futuras, err := repo.ListarReservasFuturasDeMateria(context.Background(), materiaID, fecha.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(futuras) != 1 || futuras[0].ID != res1.ID {
		t.Fatalf("esperaba solo la reserva de materiaID, obtuve %+v", futuras)
	}
}

// ── ReglaRecurrencia ────────────────────────────────────────────────────

// La regla no guarda sus PCs: la relación con las PCs vive en los
// ReservaGrupo que se materializan a partir de ella.
func TestPostgresRepo_ReglaRecurrencia_Crear(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	usuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")

	regla, err := domain.NuevaReglaRecurrencia(NuevoID(), materiaID, usuarioID, domain.Lunes,
		8*time.Hour, 9*time.Hour, time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}

	if err := repo.CrearReglaRecurrencia(context.Background(), regla); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
}

func TestPostgresRepo_ListarGruposFuturosDeRegla(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	usuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")

	regla, _ := domain.NuevaReglaRecurrencia(NuevoID(), materiaID, usuarioID, domain.Lunes,
		8*time.Hour, 9*time.Hour, time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC))
	repo.CrearReglaRecurrencia(context.Background(), regla)

	g1 := nuevoReservaGrupoDeTest(materiaID, time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour)
	g1.ReglaRecurrenciaID = &regla.ID
	g2 := nuevoReservaGrupoDeTest(materiaID, time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour)
	g2.ReglaRecurrenciaID = &regla.ID
	repo.CrearReservaGrupo(context.Background(), g1)
	repo.CrearReservaGrupo(context.Background(), g2)

	futuros, err := repo.ListarGruposFuturosDeRegla(context.Background(), regla.ID, time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(futuros) != 1 || futuros[0].ID != g2.ID {
		t.Fatalf("esperaba solo g2 (posterior al 2 de marzo), obtuve %+v", futuros)
	}
}

// ── Validadores ─────────────────────────────────────────────────────────

func TestValidadorMateriaPostgres_Asignado_True(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	materiaID := crearMateriaDeTest(t, pool)
	usuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	pool.Exec(context.Background(), `INSERT INTO docente_materia (id, usuario_id, materia_id, rol) VALUES ($1, $2, $3, 'TITULAR')`,
		NuevoID(), usuarioID, materiaID)

	validador := NewValidadorMateriaPostgres(pool)
	asignado, err := validador.DocenteEstaAsignado(context.Background(), materiaID, usuarioID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !asignado {
		t.Error("esperaba asignado=true")
	}
}

func TestValidadorMateriaPostgres_NoAsignado_False(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	materiaID := crearMateriaDeTest(t, pool)
	usuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")

	validador := NewValidadorMateriaPostgres(pool)
	asignado, err := validador.DocenteEstaAsignado(context.Background(), materiaID, usuarioID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if asignado {
		t.Error("esperaba asignado=false, sin fila en docente_materia")
	}
}

func TestValidadorEquipoPostgres_Disponible_True(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	equipoID := crearEquipoDeCarroDeTest(t, pool)

	validador := NewValidadorEquipoPostgres(pool)
	disponible, err := validador.EquipoDisponibleParaReservar(context.Background(), equipoID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !disponible {
		t.Error("esperaba disponible=true")
	}
}

func TestValidadorEquipoPostgres_EnMantenimiento_False(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	pool.Exec(context.Background(), `UPDATE equipo SET estado = 'EN_MANTENIMIENTO' WHERE id = $1`, equipoID)

	validador := NewValidadorEquipoPostgres(pool)
	disponible, err := validador.EquipoDisponibleParaReservar(context.Background(), equipoID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if disponible {
		t.Error("una PC en mantenimiento no debería estar disponible")
	}
}

func TestValidadorEquipoPostgres_DadaDeBaja_False(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	pool.Exec(context.Background(), `UPDATE equipo SET dado_de_baja = true WHERE id = $1`, equipoID)

	validador := NewValidadorEquipoPostgres(pool)
	disponible, err := validador.EquipoDisponibleParaReservar(context.Background(), equipoID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if disponible {
		t.Error("una PC dada de baja no debería estar disponible")
	}
}

func TestValidadorEquipoPostgres_NoExiste_FalseSinError(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorEquipoPostgres(pool)

	disponible, err := validador.EquipoDisponibleParaReservar(context.Background(), NuevoID())

	if err != nil {
		t.Fatalf("una PC inexistente no debería ser un error, solo false: %v", err)
	}
	if disponible {
		t.Error("una PC inexistente nunca debería estar disponible")
	}
}

func TestObtenedorNombrePostgres_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	usuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")

	obtenedor := NewObtenedorNombrePostgres(pool)
	nombre, err := obtenedor.NombreCompletoDe(context.Background(), usuarioID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if nombre != "Ada Lovelace" {
		t.Errorf("nombre incorrecto: %q", nombre)
	}
}

// ── Regresión: los validadores también deben mapear IDs con formato
// inválido, no solo los repos.

func TestValidadorMateriaPostgres_IDInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorMateriaPostgres(pool)

	_, err := validador.DocenteEstaAsignado(context.Background(), "MATERIA_ID", "USUARIO_ID")

	if err != application.ErrIDInvalido {
		t.Fatalf("esperaba application.ErrIDInvalido, obtuve %v", err)
	}
}

func TestValidadorEquipoPostgres_IDInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorEquipoPostgres(pool)

	_, err := validador.EquipoDisponibleParaReservar(context.Background(), "PC_ID")

	if err != application.ErrIDInvalido {
		t.Fatalf("esperaba application.ErrIDInvalido, obtuve %v", err)
	}
}

func TestObtenedorNombrePostgres_IDInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	obtenedor := NewObtenedorNombrePostgres(pool)

	_, err := obtenedor.NombreCompletoDe(context.Background(), "USUARIO_ID")

	if err != application.ErrIDInvalido {
		t.Fatalf("esperaba application.ErrIDInvalido, obtuve %v", err)
	}
}

// ── ID inválido — mismo patrón de regresión de academic/inventory ─────

func TestPostgresRepo_IDConFormatoInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	casos := []struct {
		nombre string
		fn     func() error
	}{
		{"BuscarReservaGrupoPorID", func() error { _, err := repo.BuscarReservaGrupoPorID(ctx, "GRUPO_ID"); return err }},
		{"BuscarReservaPorID", func() error { _, err := repo.BuscarReservaPorID(ctx, "RESERVA_ID"); return err }},
		{"ListarReservasPorGrupo", func() error { _, err := repo.ListarReservasPorGrupo(ctx, "GRUPO_ID"); return err }},
		{"ListarReservasFuturasDeEquipo", func() error {
			_, err := repo.ListarReservasFuturasDeEquipo(ctx, "PC_ID", time.Now())
			return err
		}},
	}

	for _, c := range casos {
		err := c.fn()
		if err != application.ErrIDInvalido {
			t.Errorf("%s: esperaba application.ErrIDInvalido, obtuve %v", c.nombre, err)
		}
	}
}

// ── Cascadas — probadas también end-to-end contra Postgres real ───────

func TestPostgresRepo_CascadaService_CancelarReservasFuturasDeEquipo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	docenteID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", &docenteID, fecha, 8*time.Hour, 9*time.Hour, ahora)
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	svc := application.NewService(repo,
		NewValidadorMateriaPostgres(pool), NewValidadorEquipoPostgres(pool), jornadaLibre{}, NewObtenedorNombrePostgres(pool),
		NuevoID, func() time.Time { return ahora }, eventbus.NewInMemoryEventBus())

	canceladas, notificados, err := svc.CancelarReservasFuturasDeEquipo(context.Background(), equipoID, "PC dada de baja")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if canceladas != 1 || notificados != 1 {
		t.Fatalf("esperaba 1/1, obtuve %d/%d", canceladas, notificados)
	}

	recargada, err := repo.BuscarReservaPorID(context.Background(), res.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if recargada.Estado != domain.ReservaCancelada {
		t.Errorf("la reserva debería quedar cancelada: %s", recargada.Estado)
	}
}

func TestPostgresRepo_CascadaService_CancelarReservasFuturasDeMateria(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	svc := application.NewService(repo,
		NewValidadorMateriaPostgres(pool), NewValidadorEquipoPostgres(pool), jornadaLibre{}, NewObtenedorNombrePostgres(pool),
		NuevoID, func() time.Time { return ahora }, eventbus.NewInMemoryEventBus())

	canceladas, err := svc.CancelarReservasFuturasDeMateria(context.Background(), materiaID, "Docente dado de baja")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if canceladas != 1 {
		t.Fatalf("esperaba 1, obtuve %d", canceladas)
	}

	grupoRecargado, err := repo.BuscarReservaGrupoPorID(context.Background(), g.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if grupoRecargado.Estado != domain.GrupoCancelada {
		t.Errorf("el grupo debería quedar CANCELADA: %s", grupoRecargado.Estado)
	}
}

// ── EliminarReservasYGruposDeCiclo (cascada de archivado) ──────────────

func TestPostgresRepo_EliminarReservasYGruposDeCiclo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	// Necesitamos el cicloID real (crearMateriaDeTest no lo devuelve hoy,
	// así que lo armamos acá directo para tener control total del ciclo).
	cicloID := NuevoID()
	cursoID := NuevoID()
	materiaDelCiclo := NuevoID()
	otraMateriaDeOtroCiclo := crearMateriaDeTest(t, pool) // ciclo distinto, no debe tocarse

	anio := int(atomic.AddInt32(&contadorAnioDeTest, 1)) + 3000
	if _, err := pool.Exec(ctx, `INSERT INTO ciclo_lectivo (id, anio, activo) VALUES ($1, $2, false)`, cicloID, anio); err != nil {
		t.Fatalf("no se pudo crear ciclo de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO curso (id, ciclo_lectivo_id, nombre) VALUES ($1, $2, '1°A')`, cursoID, cicloID); err != nil {
		t.Fatalf("no se pudo crear curso de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO materia (id, curso_id, nombre) VALUES ($1, $2, 'Matemáticas')`, materiaDelCiclo, cursoID); err != nil {
		t.Fatalf("no se pudo crear materia de prueba: %v", err)
	}

	pc1 := crearEquipoDeCarroDeTest(t, pool)
	pc2 := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// Un grupo con 2 reservas, del ciclo que vamos a "archivar".
	g := nuevoReservaGrupoDeTest(materiaDelCiclo, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(ctx, g)
	res1, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pc1, materiaDelCiclo, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	repo.CrearReserva(ctx, res1)
	res2, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pc2, materiaDelCiclo, "Ada", nil, fecha, 9*time.Hour+1*time.Minute, 10*time.Hour, ahora)
	repo.CrearReserva(ctx, res2)

	// Una reserva de OTRA materia, de OTRO ciclo — no debería tocarse.
	gOtroCiclo := nuevoReservaGrupoDeTest(otraMateriaDeOtroCiclo, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(ctx, gOtroCiclo)
	resOtroCiclo, _ := domain.NuevaReservaNormal(NuevoID(), gOtroCiclo.ID, pc1, otraMateriaDeOtroCiclo, "Ada", nil, fecha, 10*time.Hour, 11*time.Hour, ahora)
	repo.CrearReserva(ctx, resOtroCiclo)

	gruposEliminados, reservasEliminadas, err := repo.EliminarReservasYGruposDeCiclo(ctx, cicloID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if gruposEliminados != 1 {
		t.Errorf("esperaba 1 grupo eliminado, obtuve %d", gruposEliminados)
	}
	if reservasEliminadas != 2 {
		t.Errorf("esperaba 2 reservas eliminadas, obtuve %d", reservasEliminadas)
	}

	// El grupo del ciclo archivado ya no debería existir.
	if _, err := repo.BuscarReservaGrupoPorID(ctx, g.ID); err != application.ErrReservaGrupoNoEncontrado {
		t.Errorf("esperaba que el grupo ya no exista, obtuve %v", err)
	}

	// La reserva de otro ciclo debe seguir intacta.
	if _, err := repo.BuscarReservaGrupoPorID(ctx, gOtroCiclo.ID); err != nil {
		t.Errorf("la reserva de otro ciclo no debería haberse tocado: %v", err)
	}
}

// RF-02.4 nombra tres cosas a borrar al archivar: ReservaGrupo, Reserva y
// ReglaRecurrencia.
func TestPostgresRepo_EliminarReservasYGruposDeCiclo_BorraReglasYBloqueos(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	// El año del ciclo tiene que coincidir con el de las fechas: es lo único
	// que ata un bloqueo administrativo a un ciclo lectivo.
	const anio = 2026
	cicloID, cursoID, materiaID := NuevoID(), NuevoID(), NuevoID()
	if _, err := pool.Exec(ctx, `INSERT INTO ciclo_lectivo (id, anio, activo) VALUES ($1, $2, false)`, cicloID, anio); err != nil {
		t.Fatalf("no se pudo crear ciclo: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO curso (id, ciclo_lectivo_id, nombre) VALUES ($1, $2, '1°A')`, cursoID, cicloID); err != nil {
		t.Fatalf("no se pudo crear curso: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO materia (id, curso_id, nombre) VALUES ($1, $2, 'Matemáticas')`, materiaID, cursoID); err != nil {
		t.Fatalf("no se pudo crear materia: %v", err)
	}

	usuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	equipo := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Date(anio, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	regla, _ := domain.NuevaReglaRecurrencia(NuevoID(), materiaID, usuarioID, domain.Lunes,
		8*time.Hour, 9*time.Hour, fecha, fecha.AddDate(0, 1, 0))
	if err := repo.CrearReglaRecurrencia(ctx, regla); err != nil {
		t.Fatalf("no se pudo crear la regla: %v", err)
	}

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	g.ReglaRecurrenciaID = &regla.ID
	repo.CrearReservaGrupo(ctx, g)
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipo, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	repo.CrearReserva(ctx, res)

	// Un bloqueo administrativo del mismo año, en otra franja para no chocar
	// con la constraint EXCLUDE.
	bloqueo, _ := domain.NuevaReservaBloqueo(NuevoID(), equipo, nil, fecha, 14*time.Hour, 16*time.Hour, "Jornada docente", ahora)
	if err := repo.CrearReserva(ctx, bloqueo); err != nil {
		t.Fatalf("no se pudo crear el bloqueo: %v", err)
	}

	// Un bloqueo de OTRO año — no debe tocarse.
	bloqueoOtroAnio, _ := domain.NuevaReservaBloqueo(NuevoID(), equipo,
		nil, time.Date(anio+1, 3, 9, 0, 0, 0, 0, time.UTC), 14*time.Hour, 16*time.Hour, "Jornada docente", ahora)
	if err := repo.CrearReserva(ctx, bloqueoOtroAnio); err != nil {
		t.Fatalf("no se pudo crear el bloqueo de otro año: %v", err)
	}

	_, reservasEliminadas, err := repo.EliminarReservasYGruposDeCiclo(ctx, cicloID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	// La reserva del grupo + el bloqueo administrativo de ese año.
	if reservasEliminadas != 2 {
		t.Errorf("esperaba 2 reservas eliminadas (1 del grupo + 1 bloqueo), obtuve %d", reservasEliminadas)
	}

	var reglas int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM regla_recurrencia WHERE id = $1`, regla.ID).Scan(&reglas); err != nil {
		t.Fatal(err)
	}
	if reglas != 0 {
		t.Errorf("la regla de recurrencia quedó huérfana (RF-02.4)")
	}

	var bloqueosRestantes int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reserva WHERE tipo = 'BLOQUEO'`).Scan(&bloqueosRestantes); err != nil {
		t.Fatal(err)
	}
	if bloqueosRestantes != 1 {
		t.Errorf("esperaba que sobreviviera solo el bloqueo del otro año, quedaron %d", bloqueosRestantes)
	}
}

// ── Atomicidad (RF-04.5) ────────────────────────────────────────────────

// Regresión: antes de que las operaciones multi-fila corrieran dentro de una
// transacción, un choque de la constraint EXCLUDE a mitad del lote devolvía
// error PERO dejaba commiteados el ReservaGrupo y las Reserva ya insertadas.
func TestCrearReserva_ConflictoAMitadDelLote_NoDejaGrupoParcial(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	docenteID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	fecha := time.Date(2027, 5, 10, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	svc := application.NewService(repo,
		validadorMateriaOK{}, validadorEquipoOK{}, jornadaLibre{}, nombreDocenteFijo{}, NuevoID,
		func() time.Time { return ahora }, eventbus.NewInMemoryEventBus())

	_, _, err := svc.CrearReserva(ctx, materiaID, docenteID, false, fecha,
		14*time.Hour, 15*time.Hour, []string{equipoID, equipoID})
	if err == nil {
		t.Fatalf("se esperaba un error de solapamiento")
	}

	var grupos, reservas int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM reserva_grupo WHERE materia_id=$1`, materiaID).Scan(&grupos); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM reserva WHERE materia_id=$1`, materiaID).Scan(&reservas); err != nil {
		t.Fatal(err)
	}

	if grupos != 0 {
		t.Errorf("la operación falló pero dejó %d reserva_grupo commiteado(s)", grupos)
	}
	if reservas != 0 {
		t.Errorf("la operación falló pero dejó %d reserva(s) commiteada(s)", reservas)
	}
}

// Misma garantía para la recurrencia: si una de las N fechas choca, no
// puede quedar ni la regla ni las ocurrencias anteriores.
func TestCrearReservaRecurrente_ConflictoEnUnaFecha_NoDejaReglaNiGrupos(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	docenteID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	svc := application.NewService(repo,
		validadorMateriaOK{}, validadorEquipoOK{}, jornadaLibre{}, nombreDocenteFijo{}, NuevoID,
		func() time.Time { return ahora }, eventbus.NewInMemoryEventBus())

	// Lunes 3, 10 y 17 de mayo de 2027. Se ocupa de antemano el del medio,
	// con un grupo ajeno, para que la segunda ocurrencia choque.
	segundoLunes := time.Date(2027, 5, 10, 0, 0, 0, 0, time.UTC)
	gPrevio := nuevoReservaGrupoDeTest(materiaID, segundoLunes, 14*time.Hour, 15*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, gPrevio); err != nil {
		t.Fatal(err)
	}
	resPrevia, err := domain.NuevaReservaNormal(NuevoID(), gPrevio.ID, equipoID, materiaID,
		"Otro Docente", nil, segundoLunes, 14*time.Hour, 15*time.Hour, ahora)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CrearReserva(ctx, resPrevia); err != nil {
		t.Fatal(err)
	}

	_, err = svc.CrearReservaRecurrente(ctx, materiaID, docenteID, false, domain.Lunes,
		14*time.Hour, 15*time.Hour,
		time.Date(2027, 5, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 5, 17, 0, 0, 0, 0, time.UTC),
		[]string{equipoID})
	if err == nil {
		t.Fatalf("se esperaba un error de solapamiento en la segunda fecha")
	}

	var reglas int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM regla_recurrencia WHERE materia_id=$1`, materiaID).Scan(&reglas); err != nil {
		t.Fatal(err)
	}
	if reglas != 0 {
		t.Errorf("quedó %d regla(s) de recurrencia tras el fallo", reglas)
	}

	// Solo debe seguir existiendo el grupo previo que creamos a mano.
	var grupos int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM reserva_grupo WHERE materia_id=$1`, materiaID).Scan(&grupos); err != nil {
		t.Fatal(err)
	}
	if grupos != 1 {
		t.Errorf("esperaba solo el grupo previo, hay %d — quedaron ocurrencias a medias", grupos)
	}
}

type validadorMateriaOK struct{}

func (validadorMateriaOK) DocenteEstaAsignado(context.Context, string, string) (bool, error) {
	return true, nil
}

func (validadorMateriaOK) MateriaAceptaReservas(context.Context, string) (bool, error) {
	return true, nil
}

type validadorEquipoOK struct{}

func (validadorEquipoOK) EquipoDisponibleParaReservar(context.Context, string) (bool, error) {
	return true, nil
}

func (validadorEquipoOK) EquiposNoReservables(context.Context, []string) ([]string, error) {
	return nil, nil
}

func (validadorEquipoOK) EquipoEstaEnInventario(context.Context, string) (bool, error) {
	return true, nil
}

// Estos tests no miran los avisos, así que alcanza con no romper el contrato:
// la etiqueta real la resuelve ValidadorEquipoPostgres, que tiene su propio
// test contra la base.
func (validadorEquipoOK) EtiquetasDeEquipos(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

type nombreDocenteFijo struct{}

func (nombreDocenteFijo) NombreCompletoDe(context.Context, string) (string, error) {
	return "Ada Lovelace", nil
}

func (nombreDocenteFijo) ContactosDe(_ context.Context, usuarioIDs []string) (map[string]application.Contacto, error) {
	contactos := make(map[string]application.Contacto, len(usuarioIDs))
	for _, id := range usuarioIDs {
		contactos[id] = application.Contacto{Nombre: "Ada Lovelace", Email: id + "@escuela.edu.ar"}
	}
	return contactos, nil
}

// ── RF-04.2: PCs disponibles en una franja ─────────────────────────────

func TestListarEquiposDisponiblesEn_ExcluyeLasOcupadasYLasNoReservables(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := crearMateriaDeTest(t, pool)
	libre := crearEquipoDeCarroDeTest(t, pool)
	ocupada := crearEquipoDeCarroDeTest(t, pool)
	enMantenimiento := crearEquipoDeCarroDeTest(t, pool)
	dadoDeBaja := crearEquipoDeCarroDeTest(t, pool)

	if _, err := pool.Exec(ctx, `UPDATE equipo SET estado='EN_MANTENIMIENTO' WHERE id=$1`, enMantenimiento); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE equipo SET dado_de_baja=true WHERE id=$1`, dadoDeBaja); err != nil {
		t.Fatal(err)
	}

	fecha := time.Date(2027, 4, 12, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// Ocupa una PC de 10:00 a 11:00.
	g := nuevoReservaGrupoDeTest(materiaID, fecha, 10*time.Hour, 11*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, g); err != nil {
		t.Fatal(err)
	}
	res, err := domain.NuevaReservaNormal(NuevoID(), g.ID, ocupada, materiaID, "Ada", nil,
		fecha, 10*time.Hour, 11*time.Hour, ahora)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CrearReserva(ctx, res); err != nil {
		t.Fatal(err)
	}

	contiene := func(equipos []application.EquipoDisponible, id string) bool {
		for _, p := range equipos {
			if p.EquipoID == id {
				return true
			}
		}
		return false
	}

	// Franja que se superpone con la reserva existente.
	disponibles, err := repo.ListarEquiposDisponiblesEn(ctx, fecha, 10*time.Hour+30*time.Minute, 11*time.Hour+30*time.Minute, "")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !contiene(disponibles, libre) {
		t.Error("la PC libre debería aparecer")
	}
	if contiene(disponibles, ocupada) {
		t.Error("la PC con una reserva solapada NO debería aparecer")
	}
	if contiene(disponibles, enMantenimiento) {
		t.Error("una PC EN_MANTENIMIENTO no es reservable (RF-03.3)")
	}
	if contiene(disponibles, dadoDeBaja) {
		t.Error("una PC dada de baja no es reservable (RF-03.4)")
	}

	// Franja pegada al final de la reserva: hora_fin == hora_inicio NO
	// solapa (mismo criterio que la constraint EXCLUDE).
	pegada, err := repo.ListarEquiposDisponiblesEn(ctx, fecha, 11*time.Hour, 12*time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if !contiene(pegada, ocupada) {
		t.Error("una franja que arranca justo cuando termina la anterior no solapa: la PC debería estar disponible")
	}
}

// ── RF-03.21: la lista se ordena para la materia que se está reservando ─ Es
// el corazón de la funcionalidad y se prueba contra la base porque ahí vive:
// el tramo, la resolución de la marca más específica y el orden salen todos
// de la misma consulta.

// materiaEnCursoDeTest crea una materia con el nombre y el curso pedidos,
// para poder probar el alcance por año y división.
func materiaEnCursoDeTest(t *testing.T, pool *pgxpool.Pool, nombreMateria, nombreCurso string) string {
	t.Helper()
	ctx := context.Background()
	anio := int(atomic.AddInt32(&contadorAnioDeTest, 1)) + 3000
	cicloID, cursoID, materiaID := NuevoID(), NuevoID(), NuevoID()

	if _, err := pool.Exec(ctx, `INSERT INTO ciclo_lectivo (id, anio, activo) VALUES ($1, $2, false)`, cicloID, anio); err != nil {
		t.Fatalf("no se pudo crear ciclo de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO curso (id, ciclo_lectivo_id, nombre) VALUES ($1, $2, $3)`, cursoID, cicloID, nombreCurso); err != nil {
		t.Fatalf("no se pudo crear curso de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO materia (id, curso_id, nombre) VALUES ($1, $2, $3)`, materiaID, cursoID, nombreMateria); err != nil {
		t.Fatalf("no se pudo crear materia de prueba: %v", err)
	}
	return materiaID
}

func marcarPreferencia(t *testing.T, pool *pgxpool.Pool, equipoID, materia string, anio *int, division *string, prioridad int) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO equipo_preferencia (equipo_id, materia_nombre, anio, division, prioridad) VALUES ($1, $2, $3, $4, $5)`,
		equipoID, materia, anio, division, prioridad)
	if err != nil {
		t.Fatalf("no se pudo marcar la preferencia: %v", err)
	}
}

func TestListarEquiposDisponiblesEn_OrdenaPorPreferenciaDeMateria(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	// La materia que reserva: Matemática de 3°B.
	materiaID := materiaEnCursoDeTest(t, pool, "Matemática", "3°B")

	miPreferida := crearEquipoDeCarroDeTest(t, pool)
	neutral := crearEquipoDeCarroDeTest(t, pool)
	ajenaDebil := crearEquipoDeCarroDeTest(t, pool)
	ajenaFuerte := crearEquipoDeCarroDeTest(t, pool)

	tres, be := 3, "B"
	// Sin acento y en minúscula a propósito: el match va por nombre
	// normalizado, así que "matematica" tiene que encontrar a "Matemática".
	marcarPreferencia(t, pool, miPreferida, "matematica", &tres, &be, 2)
	marcarPreferencia(t, pool, ajenaDebil, "Dibujo Técnico", nil, nil, 3)
	marcarPreferencia(t, pool, ajenaFuerte, "Dibujo Técnico", nil, nil, 1)

	fecha := time.Date(2027, 4, 12, 0, 0, 0, 0, time.UTC)
	disponibles, err := repo.ListarEquiposDisponiblesEn(ctx, fecha, 10*time.Hour, 11*time.Hour, materiaID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	posicion := map[string]int{}
	tramo := map[string]application.TramoPreferencia{}
	for i, e := range disponibles {
		posicion[e.EquipoID] = i
		tramo[e.EquipoID] = e.Tramo
	}

	if tramo[miPreferida] != application.TramoPreferente {
		t.Errorf("la marcada para mi materia debería ser PREFERENTE, es %s", tramo[miPreferida])
	}
	if tramo[neutral] != application.TramoNeutral {
		t.Errorf("la que no prefiere nadie debería ser NEUTRAL, es %s", tramo[neutral])
	}
	if tramo[ajenaFuerte] != application.TramoDeOtraMateria {
		t.Errorf("la de otra materia debería ser DE_OTRA_MATERIA, es %s", tramo[ajenaFuerte])
	}

	if !(posicion[miPreferida] < posicion[neutral]) {
		t.Error("la preferente de mi materia va antes que la neutral")
	}
	if !(posicion[neutral] < posicion[ajenaDebil]) {
		t.Error("la neutral va antes que las de otra materia")
	}
	// Dentro del tramo ajeno, cuanto más fuerte el reclamo de la otra
	// materia, más abajo: prioridad 1 queda por debajo de prioridad 3.
	if !(posicion[ajenaDebil] < posicion[ajenaFuerte]) {
		t.Error("el reclamo ajeno más fuerte tiene que quedar más abajo")
	}

	for _, e := range disponibles {
		if e.EquipoID == miPreferida && e.MotivoDePreferencia() != "Preferente para matematica de 3°B" {
			t.Errorf("motivo inesperado: %q", e.MotivoDePreferencia())
		}
		if e.EquipoID == neutral && e.MotivoDePreferencia() != "" {
			t.Errorf("una neutral no tiene motivo, tiene %q", e.MotivoDePreferencia())
		}
	}
}

// Compartir el nombre de la materia no alcanza cuando el Admin acotó el
// alcance: la marca de "Matemática de 3°B" es ajena para Matemática de 5°A.
func TestListarEquiposDisponiblesEn_LaMarcaAcotadaNoAplicaAOtroCurso(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	otroCurso := materiaEnCursoDeTest(t, pool, "Matemática", "5°A")
	equipoID := crearEquipoDeCarroDeTest(t, pool)

	tres, be := 3, "B"
	marcarPreferencia(t, pool, equipoID, "Matemática", &tres, &be, 1)

	fecha := time.Date(2027, 4, 12, 0, 0, 0, 0, time.UTC)
	disponibles, err := repo.ListarEquiposDisponiblesEn(ctx, fecha, 10*time.Hour, 11*time.Hour, otroCurso)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	for _, e := range disponibles {
		if e.EquipoID != equipoID {
			continue
		}
		if e.Tramo != application.TramoDeOtraMateria {
			t.Errorf("esperaba DE_OTRA_MATERIA para un curso distinto, obtuve %s", e.Tramo)
		}
		return
	}
	t.Fatal("el equipo tendría que estar en la lista: la preferencia ordena, no oculta")
}

// La marca más específica gana: un equipo preferente de toda la materia Y de
// 3°B en particular tiene que resolver por la de 3°B cuando reserva 3°B.
func TestListarEquiposDisponiblesEn_GanaLaMarcaMasEspecifica(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := materiaEnCursoDeTest(t, pool, "Matemática", "3°B")
	equipoID := crearEquipoDeCarroDeTest(t, pool)

	tres, be := 3, "B"
	marcarPreferencia(t, pool, equipoID, "Matemática", nil, nil, 1)
	marcarPreferencia(t, pool, equipoID, "Matemática", &tres, &be, 4)

	fecha := time.Date(2027, 4, 12, 0, 0, 0, 0, time.UTC)
	disponibles, err := repo.ListarEquiposDisponiblesEn(ctx, fecha, 10*time.Hour, 11*time.Hour, materiaID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	for _, e := range disponibles {
		if e.EquipoID != equipoID {
			continue
		}
		if e.PreferenciaAnio != 3 || e.PreferenciaDivision != "B" {
			t.Errorf("esperaba que ganara la marca de 3°B, obtuve %d°%s", e.PreferenciaAnio, e.PreferenciaDivision)
		}
		return
	}
	t.Fatal("el equipo tendría que estar en la lista")
}

// Un Admin puede reservar sin materia. Ahí no hay ninguna preferencia que
// aplicar y la lista sale entera y neutral, con el orden de siempre.
func TestListarEquiposDisponiblesEn_SinMateria_TodoNeutral(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	equipoID := crearEquipoDeCarroDeTest(t, pool)
	marcarPreferencia(t, pool, equipoID, "Dibujo Técnico", nil, nil, 1)

	fecha := time.Date(2027, 4, 12, 0, 0, 0, 0, time.UTC)
	disponibles, err := repo.ListarEquiposDisponiblesEn(ctx, fecha, 10*time.Hour, 11*time.Hour, "")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	for _, e := range disponibles {
		if e.Tramo != application.TramoNeutral {
			t.Errorf("sin materia todo tiene que ser NEUTRAL, %s salió %s", e.Etiqueta, e.Tramo)
		}
	}
}

// Cambiar el equipo de una reserva usa el mismo orden: es la otra pantalla
// donde se elige una máquina, y la materia sale del propio grupo.
func TestListarEquiposLibresEnLaSerie_OrdenaPorPreferencia(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := materiaEnCursoDeTest(t, pool, "Matemática", "3°B")
	preferida := crearEquipoDeCarroDeTest(t, pool)
	marcarPreferencia(t, pool, preferida, "Matemática", nil, nil, 1)
	crearEquipoDeCarroDeTest(t, pool)

	fecha := time.Date(2027, 4, 12, 0, 0, 0, 0, time.UTC)
	g := nuevoReservaGrupoDeTest(materiaID, fecha, 10*time.Hour, 11*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, g); err != nil {
		t.Fatal(err)
	}

	libres, err := repo.ListarEquiposLibresEnLaSerie(ctx, g.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(libres) == 0 {
		t.Fatal("esperaba equipos libres")
	}
	if libres[0].EquipoID != preferida {
		t.Errorf("la preferente de la materia del grupo tiene que ir primera, fue %s", libres[0].Etiqueta)
	}
	if libres[0].Tramo != application.TramoPreferente {
		t.Errorf("esperaba PREFERENTE, obtuve %s", libres[0].Tramo)
	}
}

// ── RF-04.1: la materia archivada no admite reservas nuevas ────────────

func TestMateriaAceptaReservas_FalsoSiEstaArchivadaEnCualquierNivel(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorMateriaPostgres(pool)
	ctx := context.Background()

	crear := func(archivarMateria, archivarCurso, archivarCiclo bool) string {
		cicloID, cursoID, materiaID := NuevoID(), NuevoID(), NuevoID()
		anio := int(time.Now().UnixNano() % 100000)
		if _, err := pool.Exec(ctx,
			`INSERT INTO ciclo_lectivo (id, anio, activo, archivado) VALUES ($1, $2, false, $3)`,
			cicloID, anio, archivarCiclo); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO curso (id, ciclo_lectivo_id, nombre, archivado) VALUES ($1, $2, '1°A', $3)`,
			cursoID, cicloID, archivarCurso); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO materia (id, curso_id, nombre, archivado) VALUES ($1, $2, 'Matemáticas', $3)`,
			materiaID, cursoID, archivarMateria); err != nil {
			t.Fatal(err)
		}
		return materiaID
	}

	casos := []struct {
		nombre                          string
		materia, curso, ciclo, esperado bool
	}{
		{"nada archivado", false, false, false, true},
		{"materia archivada", true, false, false, false},
		{"curso archivado", false, true, false, false},
		{"ciclo archivado", false, false, true, false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			materiaID := crear(c.materia, c.curso, c.ciclo)
			acepta, err := validador.MateriaAceptaReservas(ctx, materiaID)
			if err != nil {
				t.Fatalf("no debería fallar: %v", err)
			}
			if acepta != c.esperado {
				t.Errorf("acepta=%v, esperaba %v", acepta, c.esperado)
			}
		})
	}

	t.Run("materia inexistente", func(t *testing.T) {
		acepta, err := validador.MateriaAceptaReservas(ctx, NuevoID())
		if err != nil {
			t.Fatalf("no debería fallar: %v", err)
		}
		if acepta {
			t.Error("una materia que no existe no puede aceptar reservas")
		}
	})
}

// ── Hora de pared vs.

// zonaDeLaEscuela: UTC-3 fijo, sin depender de la tzdata del host.
var zonaDeLaEscuela = time.FixedZone("ART", -3*60*60)

func TestPostgresRepo_ReservaEnCurso_NoSeFinalizaAntesDeTiempo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)

	// Clase de 11:00 a 12:00 (hora de la escuela). Son las 11:30: está en
	// curso, falta media hora para que termine.
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	g := nuevoReservaGrupoDeTest(materiaID, fecha, 11*time.Hour, 12*time.Hour)
	if err := repo.CrearReservaGrupo(context.Background(), g); err != nil {
		t.Fatalf("no se pudo crear el grupo: %v", err)
	}
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil,
		fecha, 11*time.Hour, 12*time.Hour, time.Now().UTC().Truncate(time.Microsecond))
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no se pudo crear la reserva: %v", err)
	}

	ahora := time.Date(2026, 3, 9, 11, 30, 0, 0, zonaDeLaEscuela)

	vencidas, err := repo.ListarReservasConfirmadasVencidas(context.Background(), ahora, 100)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	for _, v := range vencidas {
		if v.ID == res.ID {
			t.Error("una clase que todavía está en curso se marcó como vencida: el job la finaliza " +
				"antes de que termine y, como la constraint EXCLUDE solo aplica a CONFIRMADA, " +
				"deja la PC libre para que otro reserve encima")
		}
	}
}

func TestPostgresRepo_ReservasFuturasDeEquipo_IncluyenLasDeHoy(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)

	// Clase de hoy a la tarde. Son las 08:00 de la mañana: falta.
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	g := nuevoReservaGrupoDeTest(materiaID, fecha, 14*time.Hour, 15*time.Hour)
	if err := repo.CrearReservaGrupo(context.Background(), g); err != nil {
		t.Fatalf("no se pudo crear el grupo: %v", err)
	}
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil,
		fecha, 14*time.Hour, 15*time.Hour, time.Now().UTC().Truncate(time.Microsecond))
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no se pudo crear la reserva: %v", err)
	}

	ahora := time.Date(2026, 3, 9, 8, 0, 0, 0, zonaDeLaEscuela)

	futuras, err := repo.ListarReservasFuturasDeEquipo(context.Background(), equipoID, ahora)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	encontrada := false
	for _, f := range futuras {
		if f.ID == res.ID {
			encontrada = true
		}
	}
	if !encontrada {
		t.Error("la reserva de esta tarde no aparece como futura: sacar una PC de servicio " +
			"(RF-03.8/03.9) no cancelaría ninguna reserva del día en curso")
	}
}

func TestPostgresRepo_ReservasFuturasDeEquipo_ExcluyenLasQueYaTerminaronHoy(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)

	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	if err := repo.CrearReservaGrupo(context.Background(), g); err != nil {
		t.Fatalf("no se pudo crear el grupo: %v", err)
	}
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil,
		fecha, 8*time.Hour, 9*time.Hour, time.Now().UTC().Truncate(time.Microsecond))
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no se pudo crear la reserva: %v", err)
	}

	// 14:00: la clase de 8 a 9 ya pasó, no tiene sentido "cancelarla".
	ahora := time.Date(2026, 3, 9, 14, 0, 0, 0, zonaDeLaEscuela)

	futuras, err := repo.ListarReservasFuturasDeEquipo(context.Background(), equipoID, ahora)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	for _, f := range futuras {
		if f.ID == res.ID {
			t.Error("una clase que ya terminó hoy no debería contar como futura")
		}
	}
}

// El aviso de cancelación nombra los equipos como los reconoce la gente ("PC
// 7", "Proyector Epson"), no por su UUID: esa traducción es una consulta y
// por eso se verifica contra la base, no con un fake.
func TestValidadorEquipoPostgres_EtiquetasDeEquipos(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorEquipoPostgres(pool)
	ctx := context.Background()

	equipo := crearEquipoDeCarroDeTest(t, pool)
	// Un equipo suelto: sin carro, sin número, con nombre.
	proyector := NuevoID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO equipo (id, tipo, nombre, reservable) VALUES ($1, 'PROYECTOR', 'Proyector Epson', true)`,
		proyector); err != nil {
		t.Fatalf("no se pudo crear el equipo suelto: %v", err)
	}

	etiquetas, err := validador.EtiquetasDeEquipos(ctx, []string{equipo, proyector})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(etiquetas) != 2 {
		t.Fatalf("esperaba 2 etiquetas, obtuve %d: %v", len(etiquetas), etiquetas)
	}
	// Una PC de carro se nombra por su número; el proyector, por su nombre.
	if !strings.HasPrefix(etiquetas[equipo], "PC ") {
		t.Errorf("una PC de carro se nombra por su número: %q", etiquetas[equipo])
	}
	if etiquetas[proyector] != "Proyector Epson" {
		t.Errorf("un equipo suelto se nombra por su nombre: %q", etiquetas[proyector])
	}

	// Un equipo que no existe simplemente no aparece: el aviso sale igual,
	// sin nombrarlo, en vez de fallar.
	conFantasma, err := validador.EtiquetasDeEquipos(ctx, []string{equipo, "00000000-0000-0000-0000-000000000000"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(conFantasma) != 1 {
		t.Errorf("esperaba solo la que existe, obtuve %v", conFantasma)
	}
}

// crearEquipoSueltoDeTest: algo prestable que NO está en ningún carro,
// como el proyector. Sin carro, sin identificador y sin número de serie.
func crearEquipoSueltoDeTest(t *testing.T, pool *pgxpool.Pool, nombre string) string {
	t.Helper()
	equipoID := NuevoID()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO equipo (id, tipo, nombre, reservable, estado) VALUES ($1, 'PROYECTOR', $2, true, 'DISPONIBLE')`,
		equipoID, nombre,
	); err != nil {
		t.Fatalf("no se pudo crear equipo suelto de prueba: %v", err)
	}
	return equipoID
}

// El peor modo de falla al sumar los equipos sueltos: ListarReservas hacía
// INNER JOIN a carro, así que la reserva de un proyector no se veía distinta
// — desaparecía de la consulta entera, total paginado incluido.
func TestPostgresRepo_ListarReservas_TraeLasDeUnEquipoSinCarro(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoSueltoDeTest(t, pool, "Proyector Epson")
	ahora := time.Now().UTC().Truncate(time.Microsecond)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, g); err != nil {
		t.Fatalf("creando grupo: %v", err)
	}
	res, err := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada Lovelace", nil,
		fecha, 8*time.Hour, 9*time.Hour, ahora)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearReserva(ctx, res); err != nil {
		t.Fatalf("creando reserva: %v", err)
	}

	filas, total, err := repo.ListarReservas(ctx, application.FiltroReservas{
		EquipoID: &equipoID,
		Pagina:   paginacion.Pagina{Numero: 1, Tamanio: 10},
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(filas) != 1 || total != 1 {
		t.Fatalf("esperaba 1 fila y total 1; obtuve %d y %d", len(filas), total)
	}
	if filas[0].Etiqueta != "Proyector Epson" {
		t.Errorf("esperaba la etiqueta del equipo, obtuve %q", filas[0].Etiqueta)
	}
	// Los dos campos viejos quedan vacíos, no en basura: una pantalla que
	// arme el rótulo con ellos escribe "PC 0 · " y se nota en el acto.
	if filas[0].Identificador != 0 || filas[0].CarroNombre != "" {
		t.Errorf("esperaba identificador 0 y carro vacío, obtuve %d y %q",
			filas[0].Identificador, filas[0].CarroNombre)
	}
}

// Una PC de carro tiene que seguir trayendo las tres cosas: la etiqueta
// nueva no reemplaza al identificador ni al carro, que otras pantallas usan.
func TestPostgresRepo_ListarReservas_ElEquipoDeCarroSigueTrayendoTodo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, g); err != nil {
		t.Fatalf("creando grupo: %v", err)
	}
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada Lovelace", nil,
		fecha, 8*time.Hour, 9*time.Hour, ahora)
	if err := repo.CrearReserva(ctx, res); err != nil {
		t.Fatalf("creando reserva: %v", err)
	}

	filas, _, err := repo.ListarReservas(ctx, application.FiltroReservas{
		EquipoID: &equipoID,
		Pagina:   paginacion.Pagina{Numero: 1, Tamanio: 10},
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("esperaba 1 fila, obtuve %d", len(filas))
	}
	if filas[0].Etiqueta != "PC 1" || filas[0].Identificador != 1 || filas[0].CarroNombre == "" {
		t.Errorf("esperaba PC 1 con carro, obtuve %q / %d / %q",
			filas[0].Etiqueta, filas[0].Identificador, filas[0].CarroNombre)
	}
}

// TestPostgresRepo_Bloqueo_ElMotivoVuelveDeLaBase El motivo del bloqueo vive
// en la fila del bloqueo y no solo en el texto de las cancelaciones que
// disparó.
func TestPostgresRepo_Bloqueo_ElMotivoVuelveDeLaBase(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC()

	bloqueo, err := domain.NuevaReservaBloqueo(NuevoID(), equipoID, nil, fecha,
		8*time.Hour, 10*time.Hour, "Jornada docente", ahora)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearReserva(ctx, bloqueo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	vuelto, err := repo.BuscarReservaPorID(ctx, bloqueo.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if vuelto.Tipo != domain.TipoBloqueo {
		t.Errorf("tipo = %q, esperaba BLOQUEO", vuelto.Tipo)
	}
	if vuelto.MotivoBloqueo != "Jornada docente" {
		t.Errorf("motivo = %q, esperaba el que se guardó", vuelto.MotivoBloqueo)
	}

	// Y en el calendario del equipo, que es donde alguien va a mirar por qué
	// no puede reservar esa franja.
	bloques, err := repo.CalendarioDeEquipo(ctx, equipoID, fecha, fecha)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(bloques) != 1 || bloques[0].Reserva.MotivoBloqueo != "Jornada docente" {
		t.Errorf("el calendario debería traer el motivo: %+v", bloques)
	}
}

// El CHECK `chk_reserva_tipo_coherente` no deja que exista un bloqueo sin
// motivo, ni siquiera escribiendo directo en la base.
func TestPostgresRepo_Bloqueo_SinMotivoLoRechazaLaBase(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	ctx := context.Background()
	equipoID := crearEquipoDeCarroDeTest(t, pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO reserva (id, equipo_id, fecha, hora_inicio, hora_fin, estado, tipo)
		VALUES ($1, $2, DATE '2026-03-09', TIME '08:00', TIME '10:00', 'CONFIRMADA', 'BLOQUEO')
	`, NuevoID(), equipoID)
	if err == nil {
		t.Fatal("la base debería rechazar un bloqueo sin motivo")
	}
}

// Y al revés: una reserva normal no lleva motivo de bloqueo. Un segundo lugar
// donde escribir para qué es la clase se desincroniza de la materia solo.
func TestPostgresRepo_ReservaNormal_NoAceptaMotivoDeBloqueo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	ctx := context.Background()
	equipoID := crearEquipoDeCarroDeTest(t, pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO reserva (id, equipo_id, fecha, hora_inicio, hora_fin, estado, tipo, motivo_bloqueo)
		VALUES ($1, $2, DATE '2026-03-09', TIME '08:00', TIME '10:00', 'CONFIRMADA', 'NORMAL', 'algo')
	`, NuevoID(), equipoID)
	if err == nil {
		t.Fatal("la base debería rechazar una reserva normal con motivo de bloqueo")
	}
}

// El pre-chequeo del lote: una sola consulta para todos los equipos y todas
// las fechas, que además dice QUÉ chocó.
func TestPostgresRepo_BuscarSolapamientos_LoteCompleto(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	pc1 := crearEquipoDeCarroDeTest(t, pool)
	pc2 := crearEquipoDeCarroDeTest(t, pool)
	lunes := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	otroLunes := lunes.AddDate(0, 0, 7)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// Ocupa pc1 el segundo lunes, de 10 a 12.
	g := nuevoReservaGrupoDeTest(materiaID, otroLunes, 10*time.Hour, 12*time.Hour)
	if err := repo.CrearReservaGrupo(context.Background(), g); err != nil {
		t.Fatalf("no se pudo crear el grupo: %v", err)
	}
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pc1, materiaID, "Ada Lovelace", nil, otroLunes, 10*time.Hour, 12*time.Hour, ahora)
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no se pudo crear la reserva: %v", err)
	}

	// Una serie de dos lunes sobre los dos equipos, de 11 a 13.
	conflictos, err := repo.BuscarSolapamientos(context.Background(),
		[]string{pc1, pc2}, []time.Time{lunes, otroLunes}, 11*time.Hour, 13*time.Hour)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if len(conflictos) != 1 {
		t.Fatalf("esperaba un solo choque, obtuve %+v", conflictos)
	}
	c := conflictos[0]
	if c.EquipoID != pc1 {
		t.Errorf("esperaba el choque en pc1, obtuve %s", c.EquipoID)
	}
	if c.Docente != "Ada Lovelace" {
		t.Errorf("esperaba el nombre de quien la tiene, obtuve %q", c.Docente)
	}
	// La etiqueta ya resuelta: quien reserva no sabe qué es un UUID.
	if !strings.HasPrefix(c.Etiqueta, "PC ") {
		t.Errorf("esperaba una etiqueta legible, obtuve %q", c.Etiqueta)
	}
	if c.HoraInicio != 10*time.Hour || c.HoraFin != 12*time.Hour {
		t.Errorf("horario incorrecto: %v–%v", c.HoraInicio, c.HoraFin)
	}
}

// Los bordes que se tocan no se pisan, igual que la constraint EXCLUDE. Es
// el caso más común que existe: la clase de 8 a 10 y la de 10 a 12.
func TestPostgresRepo_BuscarSolapamientos_BordeQueSeToca_NoCuenta(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 10*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil, fecha, 8*time.Hour, 10*time.Hour, ahora)
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	conflictos, err := repo.BuscarSolapamientos(context.Background(),
		[]string{equipoID}, []time.Time{fecha}, 10*time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(conflictos) != 0 {
		t.Fatalf("una reserva contigua no es un choque: %+v", conflictos)
	}
}

// ── Clases que cruzan la medianoche ─────────────────────────────────── Lo
// que hace falta para una escuela nocturna, y lo que ninguna cantidad de
// tests de dominio puede garantizar: que el SQL y la constraint EXCLUDE de la
// base entiendan que una clase del lunes a las 22:00 sigue ocupando la
// máquina el martes a la 01:00.

func TestPostgresRepo_ClaseNocturna_SePuedeCrear(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	lunes := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, lunes, 22*time.Hour, 1*time.Hour)
	if err := repo.CrearReservaGrupo(context.Background(), g); err != nil {
		t.Fatalf("el grupo nocturno no debería fallar: %v", err)
	}
	res, err := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil, lunes, 22*time.Hour, 1*time.Hour, ahora)
	if err != nil {
		t.Fatalf("dominio: %v", err)
	}
	// Sin fin_de_pared(), tsrange recibiría un fin ANTERIOR al inicio y Postgres
	// rechazaría el INSERT con "range lower bound must be less than or equal to
	// range upper bound".
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("una clase de 22:00 a 01:00 tiene que poder crearse: %v", err)
	}
}

// El caso que hace la diferencia entre "anda" y "parece que anda": la segunda
// reserva es de OTRA FECHA (el martes de madrugada) y aun así choca, porque
// la del lunes todavía está ocupando esa máquina.
func TestPostgresRepo_ClaseNocturna_ChocaConLaMadrugadaSiguiente(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	lunes := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	martes := lunes.AddDate(0, 0, 1)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g1 := nuevoReservaGrupoDeTest(materiaID, lunes, 22*time.Hour, 1*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g1)
	res1, _ := domain.NuevaReservaNormal(NuevoID(), g1.ID, equipoID, materiaID, "Ada", nil, lunes, 22*time.Hour, 1*time.Hour, ahora)
	if err := repo.CrearReserva(context.Background(), res1); err != nil {
		t.Fatalf("la nocturna del lunes no debería fallar: %v", err)
	}

	// El martes de 00:30 a 02:00 pisa la última media hora de la clase del lunes.
	g2 := nuevoReservaGrupoDeTest(materiaID, martes, 30*time.Minute, 2*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g2)
	res2, _ := domain.NuevaReservaNormal(NuevoID(), g2.ID, equipoID, materiaID, "Ada", nil, martes, 30*time.Minute, 2*time.Hour, ahora)

	if err := repo.CrearReserva(context.Background(), res2); err != application.ErrSolapamiento {
		t.Fatalf("esperaba ErrSolapamiento contra la nocturna del día anterior, obtuve %v", err)
	}

	// Y el pre-chequeo tiene que verlo ANTES de llegar a la constraint, o el
	// docente recibiría un error de base en vez de un mensaje que nombre la
	// máquina y a quién la tiene.
	conflictos, err := repo.BuscarSolapamientos(context.Background(),
		[]string{equipoID}, []time.Time{martes}, 30*time.Minute, 2*time.Hour)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(conflictos) != 1 {
		t.Fatalf("esperaba 1 conflicto con la clase del lunes, obtuve %d: %+v", len(conflictos), conflictos)
	}
}

// La contracara: a las 02:00 del martes la clase del lunes ya terminó, así
// que la máquina está libre.
func TestPostgresRepo_ClaseNocturna_DespuesDeQueTermina_Libre(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	lunes := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	martes := lunes.AddDate(0, 0, 1)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g1 := nuevoReservaGrupoDeTest(materiaID, lunes, 22*time.Hour, 1*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g1)
	res1, _ := domain.NuevaReservaNormal(NuevoID(), g1.ID, equipoID, materiaID, "Ada", nil, lunes, 22*time.Hour, 1*time.Hour, ahora)
	repo.CrearReserva(context.Background(), res1)

	g2 := nuevoReservaGrupoDeTest(materiaID, martes, 1*time.Hour, 3*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g2)
	res2, _ := domain.NuevaReservaNormal(NuevoID(), g2.ID, equipoID, materiaID, "Ada", nil, martes, 1*time.Hour, 3*time.Hour, ahora)

	if err := repo.CrearReserva(context.Background(), res2); err != nil {
		t.Fatalf("de 01:00 a 03:00 arranca justo cuando la otra termina: %v", err)
	}
}

// ── Clases nocturnas, del lado de la LECTURA ────────────────────────── Los
// tres de arriba cubren el alta: que se pueda crear, que choque con la
// madrugada siguiente y que a las 02:00 la máquina ya esté libre.

// El bug más visible: pedir una franja que cruza la medianoche no filtraba
// nada, hacía reventar la consulta entera con "range lower bound must be less
// than or equal to range upper bound".
func TestListarEquiposDisponiblesEn_FranjaQueCruzaMedianoche_NoRevienta(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	lunes := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)

	// De 19:00 a 03:00 del día siguiente, con la máquina enteramente libre.
	disponibles, err := repo.ListarEquiposDisponiblesEn(ctx, lunes, 19*time.Hour, 3*time.Hour, "")
	if err != nil {
		t.Fatalf("una franja que termina al día siguiente es válida: %v", err)
	}
	if !contieneEquipo(disponibles, equipoID) {
		t.Error("la máquina está libre toda la noche: tiene que aparecer")
	}
}

// Y la contracara: si en el medio de esa noche hay algo, no aparece.
func TestListarEquiposDisponiblesEn_FranjaQueCruzaMedianoche_VeLoQueYaEsta(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	lunes := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	martes := lunes.AddDate(0, 0, 1)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// Alguien ya tiene la máquina el martes de 01:00 a 02:00, en el medio de
	// la franja lunes 19:00 → martes 03:00.
	g := nuevoReservaGrupoDeTest(materiaID, martes, 1*time.Hour, 2*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, g); err != nil {
		t.Fatal(err)
	}
	res, err := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil,
		martes, 1*time.Hour, 2*time.Hour, ahora)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CrearReserva(ctx, res); err != nil {
		t.Fatal(err)
	}

	disponibles, err := repo.ListarEquiposDisponiblesEn(ctx, lunes, 19*time.Hour, 3*time.Hour, "")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if contieneEquipo(disponibles, equipoID) {
		t.Error("la reserva del martes a la 01:00 cae dentro de la franja: no puede ofrecerse")
	}
}

// El bug silencioso, que es el peor de los dos: la clase del lunes a las
// 22:00 ocupa la madrugada del martes, pero está fechada el LUNES. Filtrando
// por `fecha = el día consultado` no aparece nunca, así que la pantalla
// ofrecía una máquina que el alta iba a rechazar con un 409.
func TestListarEquiposDisponiblesEn_LaNocturnaDeAyerOcupaEstaMadrugada(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := crearMateriaDeTest(t, pool)
	ocupado := crearEquipoDeCarroDeTest(t, pool)
	libre := crearEquipoDeCarroDeTest(t, pool)
	lunes := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	martes := lunes.AddDate(0, 0, 1)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, lunes, 22*time.Hour, 1*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, g); err != nil {
		t.Fatal(err)
	}
	res, err := domain.NuevaReservaNormal(NuevoID(), g.ID, ocupado, materiaID, "Ada", nil,
		lunes, 22*time.Hour, 1*time.Hour, ahora)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CrearReserva(ctx, res); err != nil {
		t.Fatal(err)
	}

	// Martes de 00:30 a 02:00: pisa la última media hora de la clase del lunes.
	disponibles, err := repo.ListarEquiposDisponiblesEn(ctx, martes, 30*time.Minute, 2*time.Hour, "")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if contieneEquipo(disponibles, ocupado) {
		t.Error("la nocturna del lunes sigue ocupando la máquina el martes a las 00:30")
	}
	if !contieneEquipo(disponibles, libre) {
		t.Error("la otra máquina está libre y tiene que seguir apareciendo")
	}

	// Mirar el día anterior no puede bloquear el día entero: a las 02:00 la
	// clase del lunes ya terminó.
	disponibles, err = repo.ListarEquiposDisponiblesEn(ctx, martes, 2*time.Hour, 3*time.Hour, "")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !contieneEquipo(disponibles, ocupado) {
		t.Error("a las 02:00 la clase del lunes ya terminó: la máquina está libre")
	}
}

// La otra mitad de la pantalla tiene que contar la misma historia: si el
// equipo no está entre los libres, tiene que estar entre los ocupados, con el
// nombre de quien lo tiene.
func TestListarEquiposOcupadosEn_VeLaNocturnaDeAyer(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	lunes := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	martes := lunes.AddDate(0, 0, 1)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, lunes, 22*time.Hour, 1*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, g); err != nil {
		t.Fatal(err)
	}
	res, err := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil,
		lunes, 22*time.Hour, 1*time.Hour, ahora)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CrearReserva(ctx, res); err != nil {
		t.Fatal(err)
	}

	ocupados, err := repo.ListarEquiposOcupadosEn(ctx, martes, 30*time.Minute, 2*time.Hour)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(ocupados) != 1 {
		t.Fatalf("esperaba la nocturna del lunes entre los ocupados, obtuve %d: %+v", len(ocupados), ocupados)
	}
	if ocupados[0].EquipoID != equipoID {
		t.Errorf("esperaba el equipo %s, obtuve %s", equipoID, ocupados[0].EquipoID)
	}
	if ocupados[0].DocenteNombre == "" {
		t.Error("sin el nombre de quien la tiene, no hay a quién pedírsela")
	}

	// Y una franja que cruza la medianoche tampoco puede reventar de este lado.
	if _, err := repo.ListarEquiposOcupadosEn(ctx, lunes, 19*time.Hour, 3*time.Hour); err != nil {
		t.Fatalf("una franja que termina al día siguiente es válida: %v", err)
	}
}

// Cambiar de máquina en una serie (RF-08.14) mira todas las fechas que le
// quedan.
func TestListarEquiposLibresEnLaSerie_LaNocturnaDeAyerOcupaUnaOcurrencia(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := crearMateriaDeTest(t, pool)
	ocupado := crearEquipoDeCarroDeTest(t, pool)
	libre := crearEquipoDeCarroDeTest(t, pool)
	lunes := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	martes := lunes.AddDate(0, 0, 1)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// La nocturna del lunes, sobre el equipo que después no va a poder ofrecerse.
	gNoche := nuevoReservaGrupoDeTest(materiaID, lunes, 22*time.Hour, 1*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, gNoche); err != nil {
		t.Fatal(err)
	}
	resNoche, err := domain.NuevaReservaNormal(NuevoID(), gNoche.ID, ocupado, materiaID, "Ada", nil,
		lunes, 22*time.Hour, 1*time.Hour, ahora)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CrearReserva(ctx, resNoche); err != nil {
		t.Fatal(err)
	}

	// La serie desde la que se quiere cambiar: el martes de 00:30 a 02:00.
	gSerie := nuevoReservaGrupoDeTest(materiaID, martes, 30*time.Minute, 2*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, gSerie); err != nil {
		t.Fatal(err)
	}

	libres, err := repo.ListarEquiposLibresEnLaSerie(ctx, gSerie.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if contieneEquipo(libres, ocupado) {
		t.Error("esa máquina la tiene la nocturna del lunes: ofrecerla es mandar al docente a un 409")
	}
	if !contieneEquipo(libres, libre) {
		t.Error("la otra máquina está libre y tiene que poder elegirse")
	}
}

// El pre-chequeo del alta tenía media ventana: miraba el día anterior —la
// nocturna de ayer— pero no el siguiente.
func TestPostgresRepo_BuscarSolapamientos_FranjaNocturna_VeLaDelDiaSiguiente(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	lunes := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	martes := lunes.AddDate(0, 0, 1)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// Ya hay algo el martes de 01:00 a 02:00.
	g := nuevoReservaGrupoDeTest(materiaID, martes, 1*time.Hour, 2*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, g); err != nil {
		t.Fatal(err)
	}
	res, err := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada", nil,
		martes, 1*time.Hour, 2*time.Hour, ahora)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CrearReserva(ctx, res); err != nil {
		t.Fatal(err)
	}

	// Se pide el lunes de 19:00 a 03:00: pisa esa reserva de lleno.
	conflictos, err := repo.BuscarSolapamientos(ctx,
		[]string{equipoID}, []time.Time{lunes}, 19*time.Hour, 3*time.Hour)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(conflictos) != 1 {
		t.Fatalf("esperaba 1 conflicto con la reserva del martes, obtuve %d: %+v", len(conflictos), conflictos)
	}

	// Y la contracara, para que la ventana más ancha no invente conflictos:
	// de 19:00 a 23:00 del lunes no hay nada que chocar.
	conflictos, err = repo.BuscarSolapamientos(ctx,
		[]string{equipoID}, []time.Time{lunes}, 19*time.Hour, 23*time.Hour)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(conflictos) != 0 {
		t.Errorf("esa franja no cruza la medianoche y no toca nada, obtuve %+v", conflictos)
	}
}

func contieneEquipo(equipos []application.EquipoDisponible, id string) bool {
	for _, p := range equipos {
		if p.EquipoID == id {
			return true
		}
	}
	return false
}

// jornadaLibre hace de institución que todavía no declaró su jornada, que es
// el estado en que no hay restricción horaria.
type jornadaLibre struct{}

func (jornadaLibre) PermiteReserva(_ context.Context, _ time.Time, _, _ time.Duration) (bool, error) {
	return true, nil
}

// Sin jornada declarada no hay cierre que deducir: el barrido cae a la hora
// configurada.
func (jornadaLibre) CierreDeLaJornada(_ context.Context, _ time.Time) (application.CierreDeJornada, error) {
	return application.CierreDeJornada{}, nil
}

// ── ListarReservasFuturas (el insumo del cambio de jornada) ─────────────

// La consulta que alimenta el conteo que se le muestra al Admin antes de
// achicar la jornada. Se prueba contra Postgres porque tiene cuatro JOINs y
// el filtro de "todavía no terminó" apoyado en fin_de_pared: un LEFT que
// debería ser INNER, o al revés, no lo ve ningún test con dobles.
func TestPostgresRepo_ListarReservasFuturas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	materiaID := crearMateriaDeTest(t, pool)
	pcDeCarro := crearEquipoDeCarroDeTest(t, pool)
	proyector := crearEquipoSueltoDeTest(t, pool, "Proyector Epson")
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	manana := time.Date(2099, 3, 9, 0, 0, 0, 0, time.UTC)
	ayer := time.Date(2020, 3, 9, 0, 0, 0, 0, time.UTC)

	crearReserva := func(equipoID string, fecha time.Time) *domain.Reserva {
		t.Helper()
		g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
		if err := repo.CrearReservaGrupo(ctx, g); err != nil {
			t.Fatalf("no se pudo crear el grupo: %v", err)
		}
		r, err := domain.NuevaReservaNormal(NuevoID(), g.ID, equipoID, materiaID, "Ada Lovelace",
			nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
		if err != nil {
			t.Fatalf("error de dominio inesperado: %v", err)
		}
		if err := repo.CrearReserva(ctx, r); err != nil {
			t.Fatalf("no se pudo crear la reserva: %v", err)
		}
		return r
	}

	futuraDeCarro := crearReserva(pcDeCarro, manana)
	// El equipo suelto es el caso que rompe si el JOIN con carro fuera INNER:
	// un proyector no está en ningún carro y su reserva se caería de la
	// consulta, o sea del conteo que decide qué se cancela.
	futuraSuelta := crearReserva(proyector, manana)
	crearReserva(pcDeCarro, ayer) // ya terminó

	// Un bloqueo administrativo: nunca estuvo sujeto a la jornada (RF-04.7).
	bloqueo, err := domain.NuevaReservaBloqueo(NuevoID(), pcDeCarro, nil, manana,
		14*time.Hour, 16*time.Hour, "Mantenimiento", ahora)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearReserva(ctx, bloqueo); err != nil {
		t.Fatalf("no se pudo crear el bloqueo: %v", err)
	}

	// Una cancelada tampoco cuenta: ya no ocupa nada.
	cancelada := crearReserva(proyector, manana.AddDate(0, 0, 1))
	if err := cancelada.Cancelar(nil, "de prueba", ahora); err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.GuardarReserva(ctx, cancelada); err != nil {
		t.Fatalf("no se pudo cancelar: %v", err)
	}

	futuras, err := repo.ListarReservasFuturas(ctx, ahora)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	ids := map[string]application.ReservaDetallada{}
	for _, f := range futuras {
		ids[f.Reserva.ID] = f
	}
	if len(futuras) != 2 {
		t.Fatalf("esperaba las dos futuras de clase, obtuve %d: %+v", len(futuras), ids)
	}
	if _, ok := ids[futuraDeCarro.ID]; !ok {
		t.Error("falta la reserva de la PC de carro")
	}
	if _, ok := ids[futuraSuelta.ID]; !ok {
		t.Error("falta la reserva del equipo suelto: el JOIN con carro tiene que ser LEFT")
	}
	if _, ok := ids[bloqueo.ID]; ok {
		t.Error("un bloqueo administrativo no está sujeto a la jornada")
	}
	if _, ok := ids[cancelada.ID]; ok {
		t.Error("una reserva cancelada no ocupa nada")
	}

	// Los nombres tienen que venir resueltos: son lo que se le muestra al
	// Admin para que entienda qué está por cancelar.
	if d := ids[futuraSuelta.ID]; d.Etiqueta != "Proyector Epson" {
		t.Errorf("un equipo suelto se nombra por su nombre, obtuve %q", d.Etiqueta)
	}
	if d := ids[futuraDeCarro.ID]; d.MateriaNombre == "" || d.CarroNombre == "" {
		t.Errorf("faltan los nombres resueltos: %+v", d)
	}
	if d := ids[futuraDeCarro.ID]; d.Reserva.NombreDocenteSnapshot == nil ||
		*d.Reserva.NombreDocenteSnapshot != "Ada Lovelace" {
		t.Errorf("falta el docente, que es a quién le cae la cancelación: %+v", d.Reserva)
	}
}

// La reserva que cruza la medianoche no terminó hasta que termina de verdad,
// y eso lo decide fin_de_pared. Sin esto, una clase de 22:00 a 01:00
// desaparecería del conteo apenas pasa la medianoche.
func TestPostgresRepo_ListarReservasFuturas_LaQueCruzaLaMedianoche(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	materiaID := crearMateriaDeTest(t, pool)
	pc := crearEquipoDeCarroDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	fecha := time.Date(2099, 3, 9, 0, 0, 0, 0, time.UTC)
	g := nuevoReservaGrupoDeTest(materiaID, fecha, 22*time.Hour, 1*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, g); err != nil {
		t.Fatalf("no se pudo crear el grupo: %v", err)
	}
	r, err := domain.NuevaReservaNormal(NuevoID(), g.ID, pc, materiaID, "Ada", nil,
		fecha, 22*time.Hour, 1*time.Hour, ahora)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearReserva(ctx, r); err != nil {
		t.Fatalf("no se pudo crear la reserva: %v", err)
	}

	// A las 23:00 de ese mismo día la clase está en curso: no terminó.
	enPlenaClase := fecha.Add(23 * time.Hour)
	futuras, err := repo.ListarReservasFuturas(ctx, enPlenaClase)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(futuras) != 1 {
		t.Fatalf("la clase de la noche todavía no terminó: %+v", futuras)
	}

	// A las 02:00 del día siguiente sí terminó.
	yaTermino := fecha.AddDate(0, 0, 1).Add(2 * time.Hour)
	futuras, err = repo.ListarReservasFuturas(ctx, yaTermino)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(futuras) != 0 {
		t.Errorf("a las 02:00 la clase de la noche anterior ya terminó: %+v", futuras)
	}
}
