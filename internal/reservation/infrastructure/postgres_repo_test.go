//go:build integration

package infrastructure

import (
	"context"
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

// ── Helpers para las FK que reservation necesita (ciclo→curso→materia,
// carro→pc, usuario) — inserción directa por SQL, sin pasar por
// auth/academic/inventory a propósito (reservation no los importa).

// contadorAnioDeTest asegura un año único por llamada a crearMateriaDeTest
// dentro de un mismo test — ciclo_lectivo.anio tiene una constraint
// UNIQUE, y varios tests crean más de una materia (con su propio
// ciclo/curso) en la misma corrida, así que un año fijo (2026) chocaba en
// la segunda llamada.
var contadorAnioDeTest int32

func crearMateriaDeTest(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	anio := int(atomic.AddInt32(&contadorAnioDeTest, 1)) + 3000
	cicloID := NuevoID()
	cursoID := NuevoID()
	materiaID := NuevoID()

	// activo=false a propósito: este fixture puede crearse varias veces
	// por test (una por materia independiente), y solo puede haber UN
	// ciclo activo a la vez en toda la tabla (idx_ciclo_lectivo_activo_unico,
	// RF-02.1) — estos tests no necesitan que el ciclo esté activo para
	// nada, así que evitamos esa constraint directamente.
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

func crearPCDeTest(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	carroID := NuevoID()
	pcID := NuevoID()

	if _, err := pool.Exec(ctx, `INSERT INTO carro (id, nombre) VALUES ($1, $2)`, carroID, "Carro-"+carroID[:8]); err != nil {
		t.Fatalf("no se pudo crear carro de prueba: %v", err)
	}
	numeroSerie := time.Now().UnixNano() % 1000000000
	if _, err := pool.Exec(ctx,
		`INSERT INTO pc (id, carro_id, identificador, numero_serie, estado) VALUES ($1, $2, 1, $3, 'DISPONIBLE')`,
		pcID, carroID, numeroSerie,
	); err != nil {
		t.Fatalf("no se pudo crear PC de prueba: %v", err)
	}
	return pcID
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
	pcID := crearPCDeTest(t, pool)

	g := nuevoReservaGrupoDeTest(materiaID, time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour)
	if err := repo.CrearReservaGrupo(context.Background(), g); err != nil {
		t.Fatalf("no debería fallar creando grupo: %v", err)
	}

	res, err := domain.NuevaReservaNormal(NuevoID(), g.ID, pcID, materiaID, "Ada Lovelace", nil,
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
	if encontrada.PCID != pcID || encontrada.Estado != domain.ReservaConfirmada {
		t.Errorf("reserva encontrada no coincide: %+v", encontrada)
	}
}

// ── Paginación de ListarReservas ───────────────────────────────────────
//
// Va contra Postgres real y no solo contra el fake porque lo que puede
// salir mal es el SQL: que los $n del LIMIT/OFFSET no pisen los de los
// filtros dinámicos, y que COUNT(*) OVER() cuente antes del recorte.
// Es además el listado que devolvía 2,1 MB en una sola respuesta.

func TestPostgresRepo_ListarReservas_PaginaYTotal(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	materiaID := crearMateriaDeTest(t, pool)
	pcID := crearPCDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// Cinco días distintos sobre la misma PC: sin fechas distintas chocarían
	// contra la constraint EXCLUDE.
	for i := 0; i < 5; i++ {
		fecha := time.Date(2026, 3, 2+i, 0, 0, 0, 0, time.UTC)
		g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
		if err := repo.CrearReservaGrupo(ctx, g); err != nil {
			t.Fatalf("creando grupo: %v", err)
		}
		res, err := domain.NuevaReservaNormal(NuevoID(), g.ID, pcID, materiaID, "Ada Lovelace", nil,
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
		PCID:   &pcID,
		Pagina: paginacion.Pagina{Numero: 1, Tamanio: 2},
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
	pcID := crearPCDeTest(t, pool)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g1 := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g1)
	res1, _ := domain.NuevaReservaNormal(NuevoID(), g1.ID, pcID, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	if err := repo.CrearReserva(context.Background(), res1); err != nil {
		t.Fatalf("la primera reserva no debería fallar: %v", err)
	}

	g2 := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour+30*time.Minute, 9*time.Hour+30*time.Minute)
	repo.CrearReservaGrupo(context.Background(), g2)
	res2, _ := domain.NuevaReservaNormal(NuevoID(), g2.ID, pcID, materiaID, "Ada", nil, fecha, 8*time.Hour+30*time.Minute, 9*time.Hour+30*time.Minute, ahora)

	err := repo.CrearReserva(context.Background(), res2)
	if err != application.ErrSolapamiento {
		t.Fatalf("esperaba application.ErrSolapamiento (constraint EXCLUDE), obtuve %v", err)
	}
}

func TestPostgresRepo_SinSolapamiento_HorariosDistintos_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	pcID := crearPCDeTest(t, pool)
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g1 := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g1)
	res1, _ := domain.NuevaReservaNormal(NuevoID(), g1.ID, pcID, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	if err := repo.CrearReserva(context.Background(), res1); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Horario consecutivo, sin solapar (9-10hs justo después de 8-9hs).
	g2 := nuevoReservaGrupoDeTest(materiaID, fecha, 9*time.Hour, 10*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g2)
	res2, _ := domain.NuevaReservaNormal(NuevoID(), g2.ID, pcID, materiaID, "Ada", nil, fecha, 9*time.Hour, 10*time.Hour, ahora)

	if err := repo.CrearReserva(context.Background(), res2); err != nil {
		t.Fatalf("horarios consecutivos sin solapar no deberían fallar: %v", err)
	}
}

func TestPostgresRepo_SolapamientoEnPCsDistintas_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	pc1 := crearPCDeTest(t, pool)
	pc2 := crearPCDeTest(t, pool)
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
	pcID := crearPCDeTest(t, pool)
	adminID := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pcID, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
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

// TestPostgresRepo_ReservaVencida_ApareceEnElListado confirma que la
// comparación fecha+hora_fin funciona igual que la de la constraint
// EXCLUDE: aritmética date+time, no comparación de texto. Comparar como
// texto da resultados distintos y el listado dejaría de coincidir con lo
// que la constraint considera solapado.
func TestPostgresRepo_ReservaVencida_ApareceEnElListado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	pcID := crearPCDeTest(t, pool)

	ayer := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	g := nuevoReservaGrupoDeTest(materiaID, ayer, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pcID, materiaID, "Ada", nil, ayer, 8*time.Hour, 9*time.Hour, time.Now().UTC())
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
	pcID := crearPCDeTest(t, pool)

	manana := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour)
	g := nuevoReservaGrupoDeTest(materiaID, manana, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pcID, materiaID, "Ada", nil, manana, 8*time.Hour, 9*time.Hour, time.Now().UTC())
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
	pc1 := crearPCDeTest(t, pool)
	pc2 := crearPCDeTest(t, pool)
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
// ReservaGrupo que se materializan a partir de ella. La tabla
// regla_recurrencia_pc se eliminó del esquema en 002 porque solo se
// escribía y nunca se leía.
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

func TestValidadorPCPostgres_Disponible_True(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	pcID := crearPCDeTest(t, pool)

	validador := NewValidadorPCPostgres(pool)
	disponible, err := validador.PCDisponibleParaReservar(context.Background(), pcID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !disponible {
		t.Error("esperaba disponible=true")
	}
}

func TestValidadorPCPostgres_EnMantenimiento_False(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	pcID := crearPCDeTest(t, pool)
	pool.Exec(context.Background(), `UPDATE pc SET estado = 'EN_MANTENIMIENTO' WHERE id = $1`, pcID)

	validador := NewValidadorPCPostgres(pool)
	disponible, err := validador.PCDisponibleParaReservar(context.Background(), pcID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if disponible {
		t.Error("una PC en mantenimiento no debería estar disponible")
	}
}

func TestValidadorPCPostgres_DadaDeBaja_False(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	pcID := crearPCDeTest(t, pool)
	pool.Exec(context.Background(), `UPDATE pc SET dada_de_baja = true WHERE id = $1`, pcID)

	validador := NewValidadorPCPostgres(pool)
	disponible, err := validador.PCDisponibleParaReservar(context.Background(), pcID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if disponible {
		t.Error("una PC dada de baja no debería estar disponible")
	}
}

func TestValidadorPCPostgres_NoExiste_FalseSinError(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorPCPostgres(pool)

	disponible, err := validador.PCDisponibleParaReservar(context.Background(), NuevoID())

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
// inválido, no solo los repos. Bug real encontrado en la prueba manual
// (un materiaId/pcId con placeholder sin reemplazar tiraba 500 en vez de
// 400, porque estos tres adaptadores no tenían el chequeo esIDInvalido
// que sí tienen todos los demás métodos del repo).

func TestValidadorMateriaPostgres_IDInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorMateriaPostgres(pool)

	_, err := validador.DocenteEstaAsignado(context.Background(), "MATERIA_ID", "USUARIO_ID")

	if err != application.ErrIDInvalido {
		t.Fatalf("esperaba application.ErrIDInvalido, obtuve %v", err)
	}
}

func TestValidadorPCPostgres_IDInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorPCPostgres(pool)

	_, err := validador.PCDisponibleParaReservar(context.Background(), "PC_ID")

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
		{"ListarReservasFuturasDePC", func() error {
			_, err := repo.ListarReservasFuturasDePC(ctx, "PC_ID", time.Now())
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

func TestPostgresRepo_CascadaService_CancelarReservasFuturasDePC(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	pcID := crearPCDeTest(t, pool)
	fecha := time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	docenteID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pcID, materiaID, "Ada", &docenteID, fecha, 8*time.Hour, 9*time.Hour, ahora)
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	svc := application.NewService(repo,
		NewValidadorMateriaPostgres(pool), NewValidadorPCPostgres(pool), NewObtenedorNombrePostgres(pool),
		NuevoID, func() time.Time { return ahora }, eventbus.NewInMemoryEventBus())

	canceladas, notificados, err := svc.CancelarReservasFuturasDePC(context.Background(), pcID, "PC dada de baja")

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
	pcID := crearPCDeTest(t, pool)
	fecha := time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	repo.CrearReservaGrupo(context.Background(), g)
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pcID, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	svc := application.NewService(repo,
		NewValidadorMateriaPostgres(pool), NewValidadorPCPostgres(pool), NewObtenedorNombrePostgres(pool),
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

	pc1 := crearPCDeTest(t, pool)
	pc2 := crearPCDeTest(t, pool)
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
// ReglaRecurrencia. La tercera no se borraba nunca — las reglas quedaban
// huérfanas apuntando a materias archivadas. Los bloqueos por evaluación
// (RF-04.7) tampoco: no tienen materia, así que la subconsulta del ciclo no
// los alcanzaba y se acumulaban año tras año.
func TestPostgresRepo_EliminarReservasYGruposDeCiclo_BorraReglasYBloqueos(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	// El año del ciclo tiene que coincidir con el de las fechas: es lo único
	// que ata un bloqueo de evaluación a un ciclo lectivo.
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
	pc := crearPCDeTest(t, pool)
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
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pc, materiaID, "Ada", nil, fecha, 8*time.Hour, 9*time.Hour, ahora)
	repo.CrearReserva(ctx, res)

	// Un bloqueo de evaluación del mismo año, en otra franja para no chocar
	// con la constraint EXCLUDE.
	bloqueo, _ := domain.NuevaReservaEvaluacion(NuevoID(), pc, nil, fecha, 14*time.Hour, 16*time.Hour, ahora)
	if err := repo.CrearReserva(ctx, bloqueo); err != nil {
		t.Fatalf("no se pudo crear el bloqueo: %v", err)
	}

	// Un bloqueo de OTRO año — no debe tocarse.
	bloqueoOtroAnio, _ := domain.NuevaReservaEvaluacion(NuevoID(), pc,
		nil, time.Date(anio+1, 3, 9, 0, 0, 0, 0, time.UTC), 14*time.Hour, 16*time.Hour, ahora)
	if err := repo.CrearReserva(ctx, bloqueoOtroAnio); err != nil {
		t.Fatalf("no se pudo crear el bloqueo de otro año: %v", err)
	}

	_, reservasEliminadas, err := repo.EliminarReservasYGruposDeCiclo(ctx, cicloID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	// La reserva del grupo + el bloqueo de evaluación de ese año.
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
		`SELECT COUNT(*) FROM reserva WHERE tipo = 'EVALUACION_ESTATAL'`).Scan(&bloqueosRestantes); err != nil {
		t.Fatal(err)
	}
	if bloqueosRestantes != 1 {
		t.Errorf("esperaba que sobreviviera solo el bloqueo del otro año, quedaron %d", bloqueosRestantes)
	}
}

// ── Atomicidad (RF-04.5) ────────────────────────────────────────────────

// Regresión: antes de que las operaciones multi-fila corrieran dentro de
// una transacción, un choque de la constraint EXCLUDE a mitad del lote
// devolvía error PERO dejaba commiteados el ReservaGrupo y las Reserva ya
// insertadas. RF-04.5 exige que si hay conflicto no se cree ninguna.
//
// Se usa la misma PC repetida en el pedido porque reproduce el conflicto de
// forma determinística, sin depender de dos requests concurrentes.
func TestCrearReserva_ConflictoAMitadDelLote_NoDejaGrupoParcial(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := crearMateriaDeTest(t, pool)
	pcID := crearPCDeTest(t, pool)
	docenteID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	fecha := time.Date(2027, 5, 10, 0, 0, 0, 0, time.UTC)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	svc := application.NewService(repo,
		validadorMateriaOK{}, validadorPCOK{}, nombreDocenteFijo{}, NuevoID,
		func() time.Time { return ahora }, eventbus.NewInMemoryEventBus())

	_, _, err := svc.CrearReserva(ctx, materiaID, docenteID, false, fecha,
		14*time.Hour, 15*time.Hour, []string{pcID, pcID})
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
	pcID := crearPCDeTest(t, pool)
	docenteID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	svc := application.NewService(repo,
		validadorMateriaOK{}, validadorPCOK{}, nombreDocenteFijo{}, NuevoID,
		func() time.Time { return ahora }, eventbus.NewInMemoryEventBus())

	// Lunes 3, 10 y 17 de mayo de 2027. Se ocupa de antemano el del medio,
	// con un grupo ajeno, para que la segunda ocurrencia choque.
	segundoLunes := time.Date(2027, 5, 10, 0, 0, 0, 0, time.UTC)
	gPrevio := nuevoReservaGrupoDeTest(materiaID, segundoLunes, 14*time.Hour, 15*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, gPrevio); err != nil {
		t.Fatal(err)
	}
	resPrevia, err := domain.NuevaReservaNormal(NuevoID(), gPrevio.ID, pcID, materiaID,
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
		[]string{pcID})
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

type validadorPCOK struct{}

func (validadorPCOK) PCDisponibleParaReservar(context.Context, string) (bool, error) {
	return true, nil
}

// Estos tests no miran los avisos, así que alcanza con no romper el
// contrato: el identificador real lo resuelve ValidadorPCPostgres, que tiene
// su propio test contra la base.
func (validadorPCOK) IdentificadoresDePCs(context.Context, []string) (map[string]int, error) {
	return map[string]int{}, nil
}

type nombreDocenteFijo struct{}

func (nombreDocenteFijo) NombreCompletoDe(context.Context, string) (string, error) {
	return "Ada Lovelace", nil
}

// ── RF-04.2: PCs disponibles en una franja ─────────────────────────────

func TestListarPCsDisponiblesEn_ExcluyeLasOcupadasYLasNoReservables(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	materiaID := crearMateriaDeTest(t, pool)
	libre := crearPCDeTest(t, pool)
	ocupada := crearPCDeTest(t, pool)
	enMantenimiento := crearPCDeTest(t, pool)
	dadaDeBaja := crearPCDeTest(t, pool)

	if _, err := pool.Exec(ctx, `UPDATE pc SET estado='EN_MANTENIMIENTO' WHERE id=$1`, enMantenimiento); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE pc SET dada_de_baja=true WHERE id=$1`, dadaDeBaja); err != nil {
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

	contiene := func(pcs []application.PCDisponible, id string) bool {
		for _, p := range pcs {
			if p.PCID == id {
				return true
			}
		}
		return false
	}

	// Franja que se superpone con la reserva existente.
	disponibles, err := repo.ListarPCsDisponiblesEn(ctx, fecha, 10*time.Hour+30*time.Minute, 11*time.Hour+30*time.Minute)
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
	if contiene(disponibles, dadaDeBaja) {
		t.Error("una PC dada de baja no es reservable (RF-03.4)")
	}

	// Franja pegada al final de la reserva: hora_fin == hora_inicio NO
	// solapa (mismo criterio que la constraint EXCLUDE).
	pegada, err := repo.ListarPCsDisponiblesEn(ctx, fecha, 11*time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !contiene(pegada, ocupada) {
		t.Error("una franja que arranca justo cuando termina la anterior no solapa: la PC debería estar disponible")
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

// ── Hora de pared vs. instante ─────────────────────────────────────────
//
// Las columnas `fecha` (DATE) y `hora_inicio`/`hora_fin` (TIME) son la hora
// de pared de la escuela, no un instante absoluto (docs/07-modelo-datos.md).
// El proceso, en cambio, lee "ahora" en APP_TIMEZONE. Estos tests fijan que
// la comparación entre las dos cosas dé lo correcto, con un "ahora" en la
// zona de la escuela y a minutos del borde — que es donde un error se ve.
// Los tests que ya existían no podían detectar nada de esto: pasaban
// time.Now().UTC() y comparaban contra ayer/mañana.
//
// Los dos primeros pasan también con la implementación vieja (se comprobó):
// están para que siga siendo así. El tercero es el que fallaba, y es el bug
// que motivó el cambio — ver condicionNoTerminada en reserva_repo.go.

// zonaDeLaEscuela: UTC-3 fijo, sin depender de la tzdata del host.
var zonaDeLaEscuela = time.FixedZone("ART", -3*60*60)

func TestPostgresRepo_ReservaEnCurso_NoSeFinalizaAntesDeTiempo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	pcID := crearPCDeTest(t, pool)

	// Clase de 11:00 a 12:00 (hora de la escuela). Son las 11:30: está en
	// curso, falta media hora para que termine.
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	g := nuevoReservaGrupoDeTest(materiaID, fecha, 11*time.Hour, 12*time.Hour)
	if err := repo.CrearReservaGrupo(context.Background(), g); err != nil {
		t.Fatalf("no se pudo crear el grupo: %v", err)
	}
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pcID, materiaID, "Ada", nil,
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

func TestPostgresRepo_ReservasFuturasDePC_IncluyenLasDeHoy(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	pcID := crearPCDeTest(t, pool)

	// Clase de hoy a la tarde. Son las 08:00 de la mañana: falta.
	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	g := nuevoReservaGrupoDeTest(materiaID, fecha, 14*time.Hour, 15*time.Hour)
	if err := repo.CrearReservaGrupo(context.Background(), g); err != nil {
		t.Fatalf("no se pudo crear el grupo: %v", err)
	}
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pcID, materiaID, "Ada", nil,
		fecha, 14*time.Hour, 15*time.Hour, time.Now().UTC().Truncate(time.Microsecond))
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no se pudo crear la reserva: %v", err)
	}

	ahora := time.Date(2026, 3, 9, 8, 0, 0, 0, zonaDeLaEscuela)

	futuras, err := repo.ListarReservasFuturasDePC(context.Background(), pcID, ahora)
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

func TestPostgresRepo_ReservasFuturasDePC_ExcluyenLasQueYaTerminaronHoy(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	materiaID := crearMateriaDeTest(t, pool)
	pcID := crearPCDeTest(t, pool)

	fecha := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	g := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
	if err := repo.CrearReservaGrupo(context.Background(), g); err != nil {
		t.Fatalf("no se pudo crear el grupo: %v", err)
	}
	res, _ := domain.NuevaReservaNormal(NuevoID(), g.ID, pcID, materiaID, "Ada", nil,
		fecha, 8*time.Hour, 9*time.Hour, time.Now().UTC().Truncate(time.Microsecond))
	if err := repo.CrearReserva(context.Background(), res); err != nil {
		t.Fatalf("no se pudo crear la reserva: %v", err)
	}

	// 14:00: la clase de 8 a 9 ya pasó, no tiene sentido "cancelarla".
	ahora := time.Date(2026, 3, 9, 14, 0, 0, 0, zonaDeLaEscuela)

	futuras, err := repo.ListarReservasFuturasDePC(context.Background(), pcID, ahora)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	for _, f := range futuras {
		if f.ID == res.ID {
			t.Error("una clase que ya terminó hoy no debería contar como futura")
		}
	}
}

// El aviso de cancelación nombra las PCs por su número visible ("PC 7"), no
// por su UUID: esa traducción es una consulta y por eso se verifica contra
// la base, no con un fake.
func TestValidadorPCPostgres_IdentificadoresDePCs(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	validador := NewValidadorPCPostgres(pool)
	ctx := context.Background()

	pc1 := crearPCDeTest(t, pool)
	pc2 := crearPCDeTest(t, pool)

	identificadores, err := validador.IdentificadoresDePCs(ctx, []string{pc1, pc2})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(identificadores) != 2 {
		t.Fatalf("esperaba 2 identificadores, obtuve %d: %v", len(identificadores), identificadores)
	}
	if identificadores[pc1] == 0 || identificadores[pc2] == 0 {
		t.Errorf("los identificadores no pueden ser 0: %v", identificadores)
	}

	// Una PC que no existe simplemente no aparece: el aviso sale igual, sin
	// nombrarla, en vez de fallar.
	conFantasma, err := validador.IdentificadoresDePCs(ctx, []string{pc1, "00000000-0000-0000-0000-000000000000"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(conFantasma) != 1 {
		t.Errorf("esperaba solo la PC existente, obtuve %v", conFantasma)
	}

	// Lista vacía: ni siquiera consulta.
	vacio, err := validador.IdentificadoresDePCs(ctx, nil)
	if err != nil || len(vacio) != 0 {
		t.Errorf("con lista vacía esperaba un mapa vacío sin error: %v, %v", vacio, err)
	}
}
