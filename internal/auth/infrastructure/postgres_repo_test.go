//go:build integration

// Este archivo solo se compila/corre con la tag "integration" (go test
// -tags=integration ./...), porque necesita Docker corriendo para levantar un
// Postgres real con testcontainers-go.
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ramiro/sgrc/internal/auth/application"
	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
	"github.com/ramiro/sgrc/internal/shared/testdb"
)

// levantarPostgresDeTest arranca un contenedor Postgres efímero, le aplica la
// migración real del proyecto (docs/07-modelo-datos.md /
// migrations/001_esquema_inicial.sql) y devuelve un pool conectado.
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

func usuarioDeTest(email string, rol domain.Rol, estado domain.Estado) *domain.Usuario {
	return &domain.Usuario{
		ID:            NuevoID(),
		Nombre:        "Test",
		Apellido:      "Usuario",
		Email:         email,
		PasswordHash:  "hash-de-prueba",
		Rol:           rol,
		Estado:        estado,
		FechaRegistro: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func TestPostgresRepo_CrearYBuscarPorEmail_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	u := usuarioDeTest("ada@escuela.edu.ar", domain.RolDocente, domain.EstadoPendiente)

	if err := repo.Crear(ctx, u); err != nil {
		t.Fatalf("no debería fallar creando: %v", err)
	}

	encontrado, err := repo.BuscarPorEmail(ctx, "ada@escuela.edu.ar")
	if err != nil {
		t.Fatalf("no debería fallar buscando: %v", err)
	}
	if encontrado.ID != u.ID || encontrado.Nombre != u.Nombre || encontrado.Estado != domain.EstadoPendiente {
		t.Errorf("usuario encontrado no coincide: %+v", encontrado)
	}
}

func TestPostgresRepo_BuscarPorEmail_NoExiste_ErrUsuarioNoEncontrado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)

	_, err := repo.BuscarPorEmail(context.Background(), "nadie@escuela.edu.ar")

	if err != application.ErrUsuarioNoEncontrado {
		t.Fatalf("esperaba ErrUsuarioNoEncontrado, obtuve %v", err)
	}
}

func TestPostgresRepo_BuscarPorID_NoExiste_ErrUsuarioNoEncontrado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)

	_, err := repo.BuscarPorID(context.Background(), NuevoID()) // un uuid válido, pero inexistente

	if err != application.ErrUsuarioNoEncontrado {
		t.Fatalf("esperaba ErrUsuarioNoEncontrado, obtuve %v", err)
	}
}

func TestPostgresRepo_Crear_EmailDuplicado_ErrEmailYaRegistrado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	u1 := usuarioDeTest("duplicado@escuela.edu.ar", domain.RolDocente, domain.EstadoPendiente)
	if err := repo.Crear(ctx, u1); err != nil {
		t.Fatalf("la primera creación no debería fallar: %v", err)
	}

	u2 := usuarioDeTest("duplicado@escuela.edu.ar", domain.RolDocente, domain.EstadoPendiente)
	err := repo.Crear(ctx, u2)

	if err != application.ErrEmailYaRegistrado {
		t.Fatalf("esperaba ErrEmailYaRegistrado, obtuve %v", err)
	}
}

func TestPostgresRepo_Guardar_ActualizaEstadoYFechaAprobacion(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	u := usuarioDeTest("aprobar@escuela.edu.ar", domain.RolDocente, domain.EstadoPendiente)
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatalf("no debería fallar creando: %v", err)
	}

	ahora := time.Now().UTC().Truncate(time.Microsecond)
	if err := u.CambiarEstado(domain.EstadoAprobada, ahora); err != nil {
		t.Fatalf("transición de dominio inválida: %v", err)
	}

	if err := repo.Guardar(ctx, u); err != nil {
		t.Fatalf("no debería fallar guardando: %v", err)
	}

	recargado, err := repo.BuscarPorID(ctx, u.ID)
	if err != nil {
		t.Fatalf("no debería fallar recargando: %v", err)
	}
	if recargado.Estado != domain.EstadoAprobada {
		t.Errorf("el estado no se persistió: %s", recargado.Estado)
	}
	if recargado.FechaAprobacion == nil {
		t.Fatal("FechaAprobacion no se persistió")
	}
}

func TestPostgresRepo_Guardar_UsuarioInexistente_ErrUsuarioNoEncontrado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)

	u := usuarioDeTest("fantasma@escuela.edu.ar", domain.RolDocente, domain.EstadoPendiente)
	// Nunca se creó — Guardar sobre un id que no existe no debería
	// insertar silenciosamente ni quedarse callado.
	err := repo.Guardar(context.Background(), u)

	if err != application.ErrUsuarioNoEncontrado {
		t.Fatalf("esperaba ErrUsuarioNoEncontrado, obtuve %v", err)
	}
}

func TestPostgresRepo_ContarAdminsAprobados(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	n0, err := repo.ContarAdminsAprobados(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n0 != 0 {
		t.Fatalf("esperaba 0 admins en una base vacía, obtuve %d", n0)
	}

	admin1 := usuarioDeTest("admin1@escuela.edu.ar", domain.RolAdmin, domain.EstadoAprobada)
	admin2 := usuarioDeTest("admin2@escuela.edu.ar", domain.RolAdmin, domain.EstadoAprobada)
	adminPendiente := usuarioDeTest("admin3@escuela.edu.ar", domain.RolAdmin, domain.EstadoPendiente) // no cuenta
	docente := usuarioDeTest("docente@escuela.edu.ar", domain.RolDocente, domain.EstadoAprobada)      // no cuenta

	for _, u := range []*domain.Usuario{admin1, admin2, adminPendiente, docente} {
		if err := repo.Crear(ctx, u); err != nil {
			t.Fatalf("no debería fallar creando %s: %v", u.Email, err)
		}
	}

	n, err := repo.ContarAdminsAprobados(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 2 {
		t.Fatalf("esperaba 2 admins aprobados, obtuve %d", n)
	}
}

func TestPostgresRepo_Eliminar_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	u := usuarioDeTest("baja@escuela.edu.ar", domain.RolDocente, domain.EstadoBaja)
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatalf("no debería fallar creando: %v", err)
	}

	if err := repo.Eliminar(ctx, u.ID); err != nil {
		t.Fatalf("no debería fallar eliminando: %v", err)
	}

	_, err := repo.BuscarPorID(ctx, u.ID)
	if err != application.ErrUsuarioNoEncontrado {
		t.Fatalf("esperaba que ya no exista (ErrUsuarioNoEncontrado), obtuve %v", err)
	}
}

// Regresión de RF-01.9: el hard delete moría con violación de FK —y el
// handler lo devolvía como 500— si el docente había creado una reserva
// recurrente o figuraba en el snapshot histórico de un ciclo archivado.
func TestPostgresRepo_Eliminar_ConReglaRecurrenteEHistorico_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	u := usuarioDeTest("conhistoria@escuela.edu.ar", domain.RolDocente, domain.EstadoBaja)
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatalf("no debería fallar creando: %v", err)
	}

	cicloID, cursoID, materiaID, equipoID, carroID := NuevoID(), NuevoID(), NuevoID(), NuevoID(), NuevoID()
	sembrar := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO ciclo_lectivo (id, anio, activo) VALUES ($1, 2026, false)`, []any{cicloID}},
		{`INSERT INTO curso (id, ciclo_lectivo_id, nombre) VALUES ($1, $2, '1°A')`, []any{cursoID, cicloID}},
		{`INSERT INTO materia (id, curso_id, nombre) VALUES ($1, $2, 'Matemáticas')`, []any{materiaID, cursoID}},
		{`INSERT INTO carro (id, nombre) VALUES ($1, 'Carro de prueba')`, []any{carroID}},
		{`INSERT INTO equipo (id, carro_id, identificador, numero_serie) VALUES ($1, $2, 1, 'SERIE-12345')`, []any{equipoID, carroID}},
		{`INSERT INTO regla_recurrencia (id, materia_id, creado_por, dia_semana, hora_inicio, hora_fin, fecha_inicio, fecha_fin)
		  VALUES ($1, $2, $3, 'LUNES', '08:00', '09:00', '2026-03-02', '2026-11-30')`, []any{NuevoID(), materiaID, u.ID}},
		{`INSERT INTO historico_uso_docente (id, anio, usuario_id, nombre_docente_snapshot, cantidad_reservas, minutos_totales)
		  VALUES ($1, 2025, $2, 'Ada Lovelace', 10, 600)`, []any{NuevoID(), u.ID}},
	}
	for _, s := range sembrar {
		if _, err := pool.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatalf("sembrando datos: %v", err)
		}
	}

	if err := repo.Eliminar(ctx, u.ID); err != nil {
		t.Fatalf("el hard delete de RF-01.9 no debería fallar: %v", err)
	}

	// Lo asociado no se borra: solo pierde la referencia al usuario.
	var creadoPor *string
	if err := pool.QueryRow(ctx, `SELECT creado_por FROM regla_recurrencia`).Scan(&creadoPor); err != nil {
		t.Fatalf("la regla de recurrencia no debería haberse borrado: %v", err)
	}
	if creadoPor != nil {
		t.Errorf("esperaba creado_por en NULL, quedó %v", *creadoPor)
	}

	var nombre string
	var usuario *string
	if err := pool.QueryRow(ctx,
		`SELECT usuario_id, nombre_docente_snapshot FROM historico_uso_docente`).Scan(&usuario, &nombre); err != nil {
		t.Fatalf("el histórico no debería haberse borrado: %v", err)
	}
	if usuario != nil {
		t.Errorf("esperaba usuario_id en NULL, quedó %v", *usuario)
	}
	if nombre != "Ada Lovelace" {
		t.Errorf("el snapshot del nombre debería sobrevivir, quedó %q", nombre)
	}
}

func TestPostgresRepo_Eliminar_Inexistente_ErrUsuarioNoEncontrado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)

	err := repo.Eliminar(context.Background(), NuevoID())

	if err != application.ErrUsuarioNoEncontrado {
		t.Fatalf("esperaba ErrUsuarioNoEncontrado, obtuve %v", err)
	}
}

func TestPostgresRepo_EmailUnico_ConstraintDeLaBase(t *testing.T) {
	// Este test confirma específicamente que la protección contra emails
	// duplicados no depende solo de la capa de aplicación (que ya chequea
	// BuscarPorEmail antes de crear) — la constraint UNIQUE de la base es la
	// última línea de defensa ante una condición de carrera entre dos registros
	// simultáneos con el mismo email.
	pool := levantarPostgresDeTest(t)
	ctx := context.Background()

	email := fmt.Sprintf("race-%d@escuela.edu.ar", time.Now().UnixNano())
	u1 := usuarioDeTest(email, domain.RolDocente, domain.EstadoPendiente)
	u2 := usuarioDeTest(email, domain.RolDocente, domain.EstadoPendiente)

	if _, err := pool.Exec(ctx, `
		INSERT INTO usuario (id, nombre, apellido, email, password_hash, rol, estado, fecha_registro)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, u1.ID, u1.Nombre, u1.Apellido, u1.Email, u1.PasswordHash, string(u1.Rol), string(u1.Estado), u1.FechaRegistro); err != nil {
		t.Fatalf("el primer insert no debería fallar: %v", err)
	}

	repo := NewPostgresRepo(pool)
	err := repo.Crear(ctx, u2)
	if err != application.ErrEmailYaRegistrado {
		t.Fatalf("esperaba que la constraint de la base también dispare ErrEmailYaRegistrado, obtuve %v", err)
	}
}

func TestPostgresRepo_Listar_SinFiltros(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	for _, u := range []*domain.Usuario{
		usuarioDeTest("l1@escuela.edu.ar", domain.RolDocente, domain.EstadoPendiente),
		usuarioDeTest("l2@escuela.edu.ar", domain.RolAdmin, domain.EstadoAprobada),
	} {
		if err := repo.Crear(ctx, u); err != nil {
			t.Fatalf("no debería fallar creando %s: %v", u.Email, err)
		}
	}

	resultado, total, err := repo.Listar(ctx, nil, nil, paginacion.PorDefecto())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 2 || total != 2 {
		t.Fatalf("esperaba 2 usuarios, obtuve %d (total %d)", len(resultado), total)
	}
}

// La paginación se verifica contra Postgres real porque lo que puede salir
// mal es el SQL: que los $n del LIMIT/OFFSET no pisen los del filtro, y que
// COUNT(*) OVER() cuente antes del recorte y no después.
func TestPostgresRepo_Listar_PaginaYTotal(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		u := usuarioDeTest(fmt.Sprintf("p%d@escuela.edu.ar", i), domain.RolDocente, domain.EstadoAprobada)
		if err := repo.Crear(ctx, u); err != nil {
			t.Fatalf("no debería fallar creando %s: %v", u.Email, err)
		}
	}
	// Uno más que NO matchea el filtro de abajo: si el total contara la tabla
	// entera en vez de las filas filtradas, este lo delataría.
	otro := usuarioDeTest("otro@escuela.edu.ar", domain.RolAdmin, domain.EstadoAprobada)
	if err := repo.Crear(ctx, otro); err != nil {
		t.Fatalf("no debería fallar creando %s: %v", otro.Email, err)
	}

	rolDocente := domain.RolDocente
	primera, total, err := repo.Listar(ctx, nil, &rolDocente, paginacion.Pagina{Numero: 1, Tamanio: 2})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(primera) != 2 || total != 5 {
		t.Fatalf("primera página: %d filas, total %d; esperaba 2 y 5", len(primera), total)
	}

	tercera, total, err := repo.Listar(ctx, nil, &rolDocente, paginacion.Pagina{Numero: 3, Tamanio: 2})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(tercera) != 1 || total != 5 {
		t.Fatalf("tercera página: %d filas, total %d; esperaba 1 y 5", len(tercera), total)
	}

	// Ninguna fila puede repetirse entre páginas: es lo que rompe si el
	// ORDER BY no desempata (fecha_registro empata entre estas cuentas).
	vistos := map[string]bool{}
	for _, pag := range []int{1, 2, 3} {
		filas, _, err := repo.Listar(ctx, nil, &rolDocente, paginacion.Pagina{Numero: pag, Tamanio: 2})
		if err != nil {
			t.Fatalf("no debería fallar: %v", err)
		}
		for _, u := range filas {
			if vistos[u.ID] {
				t.Errorf("el usuario %s apareció en más de una página", u.ID)
			}
			vistos[u.ID] = true
		}
	}
	if len(vistos) != 5 {
		t.Errorf("recorriendo las páginas vi %d usuarios distintos, esperaba 5", len(vistos))
	}

	// Más allá del final: sin filas de las que leer COUNT(*) OVER(), el total
	// tiene que salir igual de la consulta de respaldo.
	vacia, total, err := repo.Listar(ctx, nil, &rolDocente, paginacion.Pagina{Numero: 9, Tamanio: 2})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(vacia) != 0 || total != 5 {
		t.Fatalf("página vacía: %d filas, total %d; esperaba 0 y 5", len(vacia), total)
	}
}

func TestPostgresRepo_Listar_ConAmbosFiltrosCombinados(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	admin1 := usuarioDeTest("f1@escuela.edu.ar", domain.RolAdmin, domain.EstadoAprobada)
	admin2Pendiente := usuarioDeTest("f2@escuela.edu.ar", domain.RolAdmin, domain.EstadoPendiente)
	docenteAprobado := usuarioDeTest("f3@escuela.edu.ar", domain.RolDocente, domain.EstadoAprobada)

	for _, u := range []*domain.Usuario{admin1, admin2Pendiente, docenteAprobado} {
		if err := repo.Crear(ctx, u); err != nil {
			t.Fatalf("no debería fallar creando %s: %v", u.Email, err)
		}
	}

	rolAdmin := domain.RolAdmin
	estadoAprobada := domain.EstadoAprobada
	resultado, _, err := repo.Listar(ctx, &estadoAprobada, &rolAdmin, paginacion.PorDefecto())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].ID != admin1.ID {
		t.Fatalf("esperaba solo admin1 (ADMIN+APROBADA), obtuve %+v", resultado)
	}
}

// Un ID sin formato UUID tiene que mapear a application.ErrIDInvalido (400),
// nunca a un 500 crudo de Postgres: es un error del cliente.
func TestPostgresRepo_IDConFormatoInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	casos := []struct {
		nombre string
		fn     func() error
	}{
		{"BuscarPorID", func() error { _, err := repo.BuscarPorID(ctx, "USUARIO_ID"); return err }},
		{"Eliminar", func() error { return repo.Eliminar(ctx, "USUARIO_ID") }},
	}

	for _, c := range casos {
		err := c.fn()
		if err != application.ErrIDInvalido {
			t.Errorf("%s: esperaba application.ErrIDInvalido, obtuve %v", c.nombre, err)
		}
	}
}

// ── RF-01.8: nunca cero Admins, ni con pedidos concurrentes ────────────

// Prueba determinística del mecanismo que cierra el TOCTOU: el conteo de
// Admins toma un lock sobre las filas que cuenta, así que una segunda
// transacción que quiera contar lo mismo QUEDA BLOQUEADA hasta que la primera
// termine — en vez de leer el mismo número y dejar pasar las dos bajas.
func TestContarAdminsAprobados_BloqueaALecturasConcurrentes(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	for _, email := range []string{"admin1@escuela.edu.ar", "admin2@escuela.edu.ar"} {
		if err := repo.Crear(ctx, usuarioDeTest(email, domain.RolAdmin, domain.EstadoAprobada)); err != nil {
			t.Fatal(err)
		}
	}

	// Transacción 1: cuenta (y bloquea) y se queda abierta.
	tx1Lista := make(chan struct{})
	tx1Suelta := make(chan struct{})
	tx1Termino := make(chan error, 1)
	go func() {
		tx1Termino <- repo.EnTransaccion(context.Background(), func(r application.Repo) error {
			if _, err := r.ContarAdminsAprobados(context.Background()); err != nil {
				return err
			}
			close(tx1Lista)
			<-tx1Suelta
			return nil
		})
	}()
	<-tx1Lista

	// Transacción 2: intenta contar mientras la primera sigue abierta. Si
	// el lock funciona, no puede avanzar y el contexto expira.
	ctx2, cancelar := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelar()
	err2 := repo.EnTransaccion(ctx2, func(r application.Repo) error {
		_, err := r.ContarAdminsAprobados(ctx2)
		return err
	})

	close(tx1Suelta)
	if err := <-tx1Termino; err != nil {
		t.Fatalf("la primera transacción no debería fallar: %v", err)
	}

	if err2 == nil {
		t.Fatal("la segunda lectura no quedó bloqueada: dos bajas concurrentes podrían ver ambas el mismo conteo y dejar el sistema sin Admins (RF-01.8)")
	}
	if !errors.Is(err2, context.DeadlineExceeded) {
		t.Fatalf("se esperaba que la segunda transacción quedara esperando el lock, obtuve: %v", err2)
	}
}

// Y el invariante de punta a punta: dos bajas concurrentes de los dos
// últimos Admins nunca pueden dejar el sistema en cero.
func TestUltimoAdmin_DosBajasConcurrentes_NuncaQuedanCeroAdmins(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	admin1 := usuarioDeTest("admin1@escuela.edu.ar", domain.RolAdmin, domain.EstadoAprobada)
	admin2 := usuarioDeTest("admin2@escuela.edu.ar", domain.RolAdmin, domain.EstadoAprobada)
	if err := repo.Crear(ctx, admin1); err != nil {
		t.Fatal(err)
	}
	if err := repo.Crear(ctx, admin2); err != nil {
		t.Fatal(err)
	}

	svc := application.NewService(repo, eventbus.NewInMemoryEventBus(),
		func(string) (string, error) { return "hash", nil },
		func(string, string) (bool, error) { return true, nil },
		func(*domain.Usuario) (string, error) { return "token", nil },
		NuevoID, func() (string, error) { return "temporal", nil },
		GenerarCodigoRecuperacion,
		time.Now, sinMaterias{}, sinCancelaciones{},
		nil,   // este test es sobre la concurrencia del guard de Admins, no sobre Google
		false) // ni sobre la recuperación por email

	errores := make(chan error, 2)
	var listos sync.WaitGroup
	listos.Add(2)
	arrancar := make(chan struct{})

	for _, id := range []string{admin1.ID, admin2.ID} {
		go func(usuarioID string) {
			listos.Done()
			<-arrancar
			errores <- svc.DarDeBaja(context.Background(), usuarioID)
		}(id)
	}
	listos.Wait()
	close(arrancar)

	err1, err2 := <-errores, <-errores

	exitos := 0
	for _, err := range []error{err1, err2} {
		if err == nil {
			exitos++
		} else if !errors.Is(err, application.ErrUltimoAdmin) {
			t.Fatalf("error inesperado (se esperaba ErrUltimoAdmin): %v", err)
		}
	}
	if exitos != 1 {
		t.Errorf("exactamente una baja debía prosperar, prosperaron %d", exitos)
	}

	quedan, err := repo.ContarAdminsAprobados(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if quedan < 1 {
		t.Errorf("RF-01.8 violado: quedaron %d Admins activos", quedan)
	}
}

// Degradar es el otro camino que reduce la cantidad de Admins, y no compite
// solo consigo mismo: una baja y una degradación simultáneas sobre los dos
// últimos Admins tienen que verse entre sí.
func TestUltimoAdmin_BajaYDegradacionConcurrentes_NuncaQuedanCeroAdmins(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	admin1 := usuarioDeTest("admin1@escuela.edu.ar", domain.RolAdmin, domain.EstadoAprobada)
	admin2 := usuarioDeTest("admin2@escuela.edu.ar", domain.RolAdmin, domain.EstadoAprobada)
	if err := repo.Crear(ctx, admin1); err != nil {
		t.Fatal(err)
	}
	if err := repo.Crear(ctx, admin2); err != nil {
		t.Fatal(err)
	}

	svc := application.NewService(repo, eventbus.NewInMemoryEventBus(),
		func(string) (string, error) { return "hash", nil },
		func(string, string) (bool, error) { return true, nil },
		func(*domain.Usuario) (string, error) { return "token", nil },
		NuevoID, func() (string, error) { return "temporal", nil },
		GenerarCodigoRecuperacion,
		time.Now, sinMaterias{}, sinCancelaciones{},
		nil,
		false)

	errores := make(chan error, 2)
	var listos sync.WaitGroup
	listos.Add(2)
	arrancar := make(chan struct{})

	go func() {
		listos.Done()
		<-arrancar
		errores <- svc.DarDeBaja(context.Background(), admin1.ID)
	}()
	go func() {
		listos.Done()
		<-arrancar
		// El tercer ID es de quien pide: solo importa para la regla de que
		// nadie se degrada a sí mismo, que acá no es lo que se prueba.
		errores <- svc.DegradarADocente(context.Background(), admin2.ID, "otro-admin")
	}()
	listos.Wait()
	close(arrancar)

	err1, err2 := <-errores, <-errores

	exitos := 0
	for _, err := range []error{err1, err2} {
		if err == nil {
			exitos++
		} else if !errors.Is(err, application.ErrUltimoAdmin) {
			t.Fatalf("error inesperado (se esperaba ErrUltimoAdmin): %v", err)
		}
	}
	if exitos != 1 {
		t.Errorf("exactamente una de las dos debía prosperar, prosperaron %d", exitos)
	}

	quedan, err := repo.ContarAdminsAprobados(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if quedan < 1 {
		t.Errorf("RF-01.8 violado: quedaron %d Admins activos", quedan)
	}
}

type sinMaterias struct{}

func (sinMaterias) MateriasDeDocente(context.Context, string) ([]string, error) { return nil, nil }
func (sinMaterias) QuedaOtroDocenteActivo(context.Context, string, string) (bool, error) {
	return false, nil
}
func (sinMaterias) RemoverAsignacionesDeDocente(context.Context, string) error { return nil }

type sinCancelaciones struct{}

func (sinCancelaciones) CancelarReservasFuturasDeMateria(context.Context, string, string) (int, error) {
	return 0, nil
}
