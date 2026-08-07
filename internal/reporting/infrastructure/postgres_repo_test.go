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

	"github.com/ramiro/sgrc/internal/reporting/application"
	"github.com/ramiro/sgrc/internal/reporting/domain"
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

var contadorAnioDeTest int32

// crearCicloDeTest arma un ciclo→curso→materia mínimo por SQL directo —
// reporting/infrastructure no importa academic, así que no puede usar su
// domain para esto. Devuelve el cicloID y el materiaID.
func crearCicloDeTest(t *testing.T, pool *pgxpool.Pool) (cicloID, materiaID string) {
	t.Helper()
	ctx := context.Background()
	anio := int(atomic.AddInt32(&contadorAnioDeTest, 1)) + 3000
	cicloID = NuevoID()
	cursoID := NuevoID()
	materiaID = NuevoID()

	if _, err := pool.Exec(ctx, `INSERT INTO ciclo_lectivo (id, anio, activo) VALUES ($1, $2, false)`, cicloID, anio); err != nil {
		t.Fatalf("no se pudo crear ciclo de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO curso (id, ciclo_lectivo_id, nombre) VALUES ($1, $2, '1°A')`, cursoID, cicloID); err != nil {
		t.Fatalf("no se pudo crear curso de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO materia (id, curso_id, nombre) VALUES ($1, $2, 'Matemáticas')`, materiaID, cursoID); err != nil {
		t.Fatalf("no se pudo crear materia de prueba: %v", err)
	}
	return cicloID, materiaID
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

func crearCarroYPCDeTest(t *testing.T, pool *pgxpool.Pool, identificador int) (pcID string) {
	t.Helper()
	ctx := context.Background()
	carroID := NuevoID()
	pcID = NuevoID()
	numeroSerie := fmt.Sprintf("SERIE-%d", time.Now().UnixNano())

	// El nombre del carro es UNIQUE: se deriva del id para que un mismo
	// test pueda crear más de un carro (ej. incidencias en dos PCs).
	if _, err := pool.Exec(ctx, `INSERT INTO carro (id, nombre) VALUES ($1, $2)`, carroID, "Carro-"+carroID[:8]); err != nil {
		t.Fatalf("no se pudo crear carro de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO pc (id, carro_id, identificador, numero_serie, estado) VALUES ($1, $2, $3, $4, 'DISPONIBLE')`,
		pcID, carroID, identificador, numeroSerie,
	); err != nil {
		t.Fatalf("no se pudo crear PC de prueba: %v", err)
	}
	return pcID
}

// insertarReservaDeTest inserta una fila de reserva directo por SQL (sin
// pasar por reservation, que reporting no importa) — un reserva_grupo
// mínimo más su reserva, con el horario dado.
func insertarReservaDeTest(t *testing.T, pool *pgxpool.Pool, materiaID, pcID, docenteID string, horaInicio, horaFin string, estado string) {
	t.Helper()
	insertarReservaEnFecha(t, pool, materiaID, pcID, docenteID,
		time.Now().UTC().Truncate(24*time.Hour), horaInicio, horaFin, estado)
}

// insertarReservaEnFecha es la variante con fecha explícita, para los
// tests del filtro por rango (RF-06.1).
func insertarReservaEnFecha(t *testing.T, pool *pgxpool.Pool, materiaID, pcID, docenteID string, fecha time.Time, horaInicio, horaFin string, estado string) {
	t.Helper()
	ctx := context.Background()
	grupoID := NuevoID()
	reservaID := NuevoID()

	if _, err := pool.Exec(ctx, `
		INSERT INTO reserva_grupo (id, materia_id, creado_por, nombre_docente_snapshot, fecha, hora_inicio, hora_fin, estado)
		VALUES ($1, $2, $3, 'Ada Lovelace', $4, $5::TIME, $6::TIME, $7)
	`, grupoID, materiaID, docenteID, fecha, horaInicio, horaFin, estado); err != nil {
		t.Fatalf("no se pudo crear reserva_grupo de prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO reserva (id, reserva_grupo_id, pc_id, materia_id, nombre_docente_snapshot, fecha, hora_inicio, hora_fin, estado, tipo, creado_por)
		VALUES ($1, $2, $3, $4, 'Ada Lovelace', $5, $6::TIME, $7::TIME, $8, 'NORMAL', $9)
	`, reservaID, grupoID, pcID, materiaID, fecha, horaInicio, horaFin, estado, docenteID); err != nil {
		t.Fatalf("no se pudo crear reserva de prueba: %v", err)
	}
}

// ── Agregaciones en vivo ────────────────────────────────────────────────

func TestPostgresRepo_CalcularUsoPCsDeCiclo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	cicloID, materiaID := crearCicloDeTest(t, pool)
	pcID := crearCarroYPCDeTest(t, pool, 27)
	docenteID := crearUsuarioDeTest(t, pool)

	insertarReservaDeTest(t, pool, materiaID, pcID, docenteID, "08:00", "09:00", "CONFIRMADA") // 60 min
	insertarReservaDeTest(t, pool, materiaID, pcID, docenteID, "10:00", "10:30", "FINALIZADA") // 30 min
	insertarReservaDeTest(t, pool, materiaID, pcID, docenteID, "12:00", "13:00", "CANCELADA")  // no debe contar

	resultado, err := repo.CalcularUsoPCsDeCiclo(context.Background(), cicloID, nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 {
		t.Fatalf("esperaba 1 PC con uso, obtuve %d: %+v", len(resultado), resultado)
	}
	if resultado[0].PCID != pcID {
		t.Errorf("PCID incorrecto: %s", resultado[0].PCID)
	}
	if resultado[0].CantidadReservas != 2 {
		t.Errorf("esperaba 2 reservas contadas (sin la cancelada), obtuve %d", resultado[0].CantidadReservas)
	}
	if resultado[0].MinutosReservados != 90 {
		t.Errorf("esperaba 90 minutos (60+30, sin la cancelada), obtuve %d", resultado[0].MinutosReservados)
	}
}

func TestPostgresRepo_CalcularUsoDocentesDeCiclo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	cicloID, materiaID := crearCicloDeTest(t, pool)
	pcID := crearCarroYPCDeTest(t, pool, 27)
	docenteID := crearUsuarioDeTest(t, pool)

	insertarReservaDeTest(t, pool, materiaID, pcID, docenteID, "08:00", "09:30", "CONFIRMADA") // 90 min

	resultado, err := repo.CalcularUsoDocentesDeCiclo(context.Background(), cicloID, nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].UsuarioID != docenteID {
		t.Fatalf("resultado incorrecto: %+v", resultado)
	}
	if resultado[0].MinutosReservados != 90 {
		t.Errorf("esperaba 90 minutos, obtuve %d", resultado[0].MinutosReservados)
	}
}

// "Cuál se usa más" es la pregunta que trae a alguien a este reporte, así
// que la respuesta tiene que estar en la primera fila. Sin ORDER BY las
// filas salían en el orden del hash de agregación: no aleatorio, pero
// tampoco estable entre llamadas.
func TestPostgresRepo_CalcularUsoPCsDeCiclo_OrdenaDeMayorAMenor(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	cicloID, materiaID := crearCicloDeTest(t, pool)
	docenteID := crearUsuarioDeTest(t, pool)

	pocoUsada := crearCarroYPCDeTest(t, pool, 3)
	muyUsada := crearCarroYPCDeTest(t, pool, 8)

	insertarReservaDeTest(t, pool, materiaID, pocoUsada, docenteID, "08:00", "08:30", "CONFIRMADA") // 30 min
	insertarReservaDeTest(t, pool, materiaID, muyUsada, docenteID, "10:00", "13:00", "CONFIRMADA")  // 180 min

	resultado, err := repo.CalcularUsoPCsDeCiclo(context.Background(), cicloID, nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 2 {
		t.Fatalf("esperaba 2 PCs con uso, obtuve %d: %+v", len(resultado), resultado)
	}
	if resultado[0].PCID != muyUsada {
		t.Errorf("la más usada tiene que venir primera; llegó %+v", resultado)
	}
	if resultado[0].MinutosReservados < resultado[1].MinutosReservados {
		t.Errorf("orden descendente roto: %d antes que %d",
			resultado[0].MinutosReservados, resultado[1].MinutosReservados)
	}
}

func TestPostgresRepo_CalcularUsoDocentesDeCiclo_OrdenaDeMayorAMenor(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	cicloID, materiaID := crearCicloDeTest(t, pool)
	pcID := crearCarroYPCDeTest(t, pool, 27)

	reservaPoco := crearUsuarioDeTest(t, pool)
	reservaMucho := crearUsuarioDeTest(t, pool)

	insertarReservaDeTest(t, pool, materiaID, pcID, reservaPoco, "08:00", "08:30", "CONFIRMADA")  // 30 min
	insertarReservaDeTest(t, pool, materiaID, pcID, reservaMucho, "10:00", "13:00", "CONFIRMADA") // 180 min

	resultado, err := repo.CalcularUsoDocentesDeCiclo(context.Background(), cicloID, nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 2 {
		t.Fatalf("esperaba 2 docentes, obtuve %d: %+v", len(resultado), resultado)
	}
	if resultado[0].UsuarioID != reservaMucho {
		t.Errorf("el de más horas tiene que venir primero; llegó %+v", resultado)
	}
}

func TestPostgresRepo_CalcularUso_OtroCicloNoSeToca(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	_, materiaDelOtroCiclo := crearCicloDeTest(t, pool)
	cicloVacio, _ := crearCicloDeTest(t, pool)
	pcID := crearCarroYPCDeTest(t, pool, 27)
	docenteID := crearUsuarioDeTest(t, pool)

	insertarReservaDeTest(t, pool, materiaDelOtroCiclo, pcID, docenteID, "08:00", "09:00", "CONFIRMADA")

	resultado, err := repo.CalcularUsoPCsDeCiclo(context.Background(), cicloVacio, nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 0 {
		t.Fatalf("esperaba 0 (la reserva es de otro ciclo), obtuve %+v", resultado)
	}
}

// ── Snapshot histórico ──────────────────────────────────────────────────

func TestPostgresRepo_GuardarYListarHistoricoUsoPC(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	pcID := crearCarroYPCDeTest(t, pool, 27)

	h, err := domain.NuevoHistoricoUsoPC(NuevoID(), 5000, pcID, 27, "Carro 1", 900, 12)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.GuardarHistoricoUsoPC(context.Background(), h); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	resultado, err := repo.ListarHistoricoUsoPCPorAnio(context.Background(), 5000)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].MinutosReservados != 900 {
		t.Fatalf("resultado incorrecto: %+v", resultado)
	}
}

func TestPostgresRepo_GuardarYListarHistoricoUsoDocente(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	docenteID := crearUsuarioDeTest(t, pool)

	h, err := domain.NuevoHistoricoUsoDocente(NuevoID(), 5001, &docenteID, "Ada Lovelace", 8, 600)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.GuardarHistoricoUsoDocente(context.Background(), h); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	resultado, err := repo.ListarHistoricoUsoDocentePorAnio(context.Background(), 5001)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].NombreDocenteSnapshot != "Ada Lovelace" {
		t.Fatalf("resultado incorrecto: %+v", resultado)
	}
}

func TestPostgresRepo_GuardarHistoricoUsoDocente_SinUsuarioID_OK(t *testing.T) {
	// Simula el caso donde el docente ya fue eliminado definitivamente —
	// UsuarioID nil, se guarda igual (columna nullable, SET NULL).
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)

	h, err := domain.NuevoHistoricoUsoDocente(NuevoID(), 5002, nil, "Docente Eliminado", 3, 200)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.GuardarHistoricoUsoDocente(context.Background(), h); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	resultado, err := repo.ListarHistoricoUsoDocentePorAnio(context.Background(), 5002)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].UsuarioID != nil {
		t.Fatalf("esperaba UsuarioID nil, obtuve %+v", resultado)
	}
}

// ── Adaptadores ─────────────────────────────────────────────────────────

func TestInfoPCPostgres_IdentificadorYCarroDe(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	pcID := crearCarroYPCDeTest(t, pool, 42)

	info := NewInfoPCPostgres(pool)
	identificador, carroNombre, err := info.IdentificadorYCarroDe(context.Background(), pcID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	// El nombre exacto lo genera el helper a partir del id del carro; lo
	// que importa acá es que el adaptador lo resuelva y no venga vacío.
	if identificador != 42 || !strings.HasPrefix(carroNombre, "Carro-") {
		t.Errorf("valores incorrectos: identificador=%d carro=%q", identificador, carroNombre)
	}
}

func TestInfoUsuarioPostgres_NombreCompletoDe(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	usuarioID := crearUsuarioDeTest(t, pool)

	info := NewInfoUsuarioPostgres(pool)
	nombre, err := info.NombreCompletoDe(context.Background(), usuarioID)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if nombre != "Ada Lovelace" {
		t.Errorf("nombre incorrecto: %q", nombre)
	}
}

func TestPostgresRepo_IDConFormatoInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	infoPC := NewInfoPCPostgres(pool)
	infoUsuario := NewInfoUsuarioPostgres(pool)
	ctx := context.Background()

	casos := []struct {
		nombre string
		fn     func() error
	}{
		{"CalcularUsoPCsDeCiclo", func() error { _, err := repo.CalcularUsoPCsDeCiclo(ctx, "CICLO_ID", nil, nil); return err }},
		{"CalcularUsoDocentesDeCiclo", func() error { _, err := repo.CalcularUsoDocentesDeCiclo(ctx, "CICLO_ID", nil, nil); return err }},
		{"IdentificadorYCarroDe", func() error { _, _, err := infoPC.IdentificadorYCarroDe(ctx, "PC_ID"); return err }},
		{"NombreCompletoDe", func() error { _, err := infoUsuario.NombreCompletoDe(ctx, "USUARIO_ID"); return err }},
	}

	for _, c := range casos {
		err := c.fn()
		if err != application.ErrIDInvalido {
			t.Errorf("%s: esperaba application.ErrIDInvalido, obtuve %v", c.nombre, err)
		}
	}
}

// ── RF-06.3: incidencias por equipo y por carro ────────────────────────

func insertarIncidenciaDeTest(t *testing.T, pool *pgxpool.Pool, pcID, gravedad, estado string, fecha time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO incidencia (id, pc_id, descripcion, gravedad, estado, fecha)
		VALUES ($1, $2, 'no arranca', $3, $4, $5)
	`, NuevoID(), pcID, gravedad, estado, fecha)
	if err != nil {
		t.Fatalf("no se pudo insertar incidencia de prueba: %v", err)
	}
}

func TestCalcularIncidenciasPorPC_AgrupaYCuentaPorEstadoYGravedad(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	pc1 := crearCarroYPCDeTest(t, pool, 1)
	pc2 := crearCarroYPCDeTest(t, pool, 2)
	fecha := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)

	insertarIncidenciaDeTest(t, pool, pc1, "GRAVE", "ABIERTA", fecha)
	insertarIncidenciaDeTest(t, pool, pc1, "LEVE", "RESUELTA", fecha)
	insertarIncidenciaDeTest(t, pool, pc1, "MODERADA", "EN_REPARACION", fecha)
	insertarIncidenciaDeTest(t, pool, pc2, "LEVE", "ABIERTA", fecha)

	resumenes, err := repo.CalcularIncidenciasPorPC(ctx, nil, nil)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resumenes) != 2 {
		t.Fatalf("esperaba 2 PCs con incidencias, obtuve %d", len(resumenes))
	}

	// Viene ordenado por total desc, así que pc1 (3) va primero.
	primero := resumenes[0]
	if primero.PCID != pc1 || primero.Total != 3 {
		t.Fatalf("esperaba pc1 con 3 incidencias primero, obtuve %+v", primero)
	}
	if primero.Abiertas != 1 || primero.Resueltas != 1 || primero.EnReparacion != 1 || primero.Graves != 1 {
		t.Errorf("desglose incorrecto: %+v", primero)
	}
	if primero.CarroNombre == "" {
		t.Errorf("debería traer el nombre del carro para poder mostrarlo")
	}
}

func TestCalcularIncidenciasPorPC_RespetaElRangoDeFechas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	pc := crearCarroYPCDeTest(t, pool, 1)
	insertarIncidenciaDeTest(t, pool, pc, "LEVE", "ABIERTA", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	insertarIncidenciaDeTest(t, pool, pc, "LEVE", "ABIERTA", time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))

	desde := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	resumenes, err := repo.CalcularIncidenciasPorPC(ctx, &desde, nil)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resumenes) != 1 || resumenes[0].Total != 1 {
		t.Fatalf("el filtro por fecha debería dejar solo la incidencia de junio: %+v", resumenes)
	}
}

func TestCalcularIncidenciasPorCarro_AgrupaPorCarro(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	pc := crearCarroYPCDeTest(t, pool, 1)
	fecha := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	insertarIncidenciaDeTest(t, pool, pc, "GRAVE", "ABIERTA", fecha)
	insertarIncidenciaDeTest(t, pool, pc, "LEVE", "RESUELTA", fecha)

	resumenes, err := repo.CalcularIncidenciasPorCarro(ctx, nil, nil)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resumenes) != 1 {
		t.Fatalf("esperaba 1 carro, obtuve %d", len(resumenes))
	}
	if resumenes[0].Total != 2 || resumenes[0].Abiertas != 1 || resumenes[0].Graves != 1 {
		t.Errorf("desglose por carro incorrecto: %+v", resumenes[0])
	}
}

// RF-06.1: el uso por PC es filtrable por rango de fechas.
func TestCalcularUsoPCsDeCiclo_RespetaElRangoDeFechas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	cicloID, materiaID := crearCicloDeTest(t, pool)
	pcID := crearCarroYPCDeTest(t, pool, 1)
	usuarioID := crearUsuarioDeTest(t, pool)

	insertarReservaEnFecha(t, pool, materiaID, pcID, usuarioID,
		time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), "08:00", "09:00", "FINALIZADA")
	insertarReservaEnFecha(t, pool, materiaID, pcID, usuarioID,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), "08:00", "10:00", "FINALIZADA")

	// Sin filtro: las dos.
	todos, err := repo.CalcularUsoPCsDeCiclo(ctx, cicloID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 1 || todos[0].CantidadReservas != 2 || todos[0].MinutosReservados != 180 {
		t.Fatalf("sin filtro esperaba 2 reservas / 180 min: %+v", todos)
	}

	// Solo el segundo semestre.
	desde := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	filtrado, err := repo.CalcularUsoPCsDeCiclo(ctx, cicloID, &desde, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtrado) != 1 || filtrado[0].CantidadReservas != 1 || filtrado[0].MinutosReservados != 120 {
		t.Fatalf("con desde=julio esperaba 1 reserva / 120 min: %+v", filtrado)
	}
}
