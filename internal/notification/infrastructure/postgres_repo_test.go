//go:build integration

package infrastructure

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ramiro/sgrc/internal/notification/application"
	"github.com/ramiro/sgrc/internal/notification/domain"
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

func TestPostgresRepo_CrearYBuscarPorID_OK(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	usuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")

	n, err := domain.NuevaNotificacion(NuevoID(), usuarioID, "Tu reserva fue cancelada", domain.TipoReservaCancelada, domain.Referencias{}, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.Crear(context.Background(), n); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	encontrada, err := repo.BuscarPorID(context.Background(), n.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if encontrada.Mensaje != "Tu reserva fue cancelada" || encontrada.Estado != domain.NoLeida {
		t.Errorf("notificación encontrada no coincide: %+v", encontrada)
	}
}

// Los instantes van en TIMESTAMPTZ y este test lo sostiene.
func TestPostgresRepo_CreadaEn_ConservaElInstante(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	usuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")

	escuela := time.FixedZone("ART", -3*60*60)
	instante := time.Date(2026, 8, 1, 3, 31, 37, 0, escuela)

	n, err := domain.NuevaNotificacion(NuevoID(), usuarioID, "Tu reserva fue cancelada", domain.TipoReservaCancelada, domain.Referencias{}, instante)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.Crear(context.Background(), n); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	encontrada, err := repo.BuscarPorID(context.Background(), n.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if !encontrada.CreadaEn.Equal(instante) {
		t.Errorf("el instante no sobrevivió el ida y vuelta:\n  guardado: %s\n  leído:    %s\n  diferencia: %s",
			instante.Format(time.RFC3339), encontrada.CreadaEn.Format(time.RFC3339),
			encontrada.CreadaEn.Sub(instante))
	}
}

func TestPostgresRepo_BuscarPorID_NoExiste_Error(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)

	_, err := repo.BuscarPorID(context.Background(), NuevoID())

	if err != application.ErrNotificacionNoEncontrada {
		t.Fatalf("esperaba ErrNotificacionNoEncontrada, obtuve %v", err)
	}
}

func TestPostgresRepo_Guardar_MarcarLeida(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	usuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")

	n, _ := domain.NuevaNotificacion(NuevoID(), usuarioID, "mensaje", domain.TipoReservaCancelada, domain.Referencias{}, time.Now().UTC().Truncate(time.Microsecond))
	if err := repo.Crear(context.Background(), n); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	ahora := time.Now().UTC().Truncate(time.Microsecond)
	if err := n.MarcarLeida(ahora); err != nil {
		t.Fatalf("transición de dominio inválida: %v", err)
	}
	if err := repo.Guardar(context.Background(), n); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recargada, err := repo.BuscarPorID(context.Background(), n.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if recargada.Estado != domain.Leida || recargada.LeidaEn == nil {
		t.Errorf("no se persistió correctamente: %+v", recargada)
	}
}

func TestPostgresRepo_ListarPorUsuario_FiltraPorEstado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	usuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	otroUsuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")

	n1, _ := domain.NuevaNotificacion(NuevoID(), usuarioID, "no leída", domain.TipoReservaCancelada, domain.Referencias{}, time.Now().UTC().Truncate(time.Microsecond))
	repo.Crear(context.Background(), n1)

	n2, _ := domain.NuevaNotificacion(NuevoID(), usuarioID, "leída", domain.TipoReservaCancelada, domain.Referencias{}, time.Now().UTC().Truncate(time.Microsecond))
	n2.MarcarLeida(time.Now().UTC().Truncate(time.Microsecond))
	repo.Crear(context.Background(), n2)
	repo.Guardar(context.Background(), n2)

	// De otro usuario — no debería aparecer nunca en las consultas de arriba.
	n3, _ := domain.NuevaNotificacion(NuevoID(), otroUsuarioID, "de otro usuario", domain.TipoReservaCancelada, domain.Referencias{}, time.Now().UTC().Truncate(time.Microsecond))
	repo.Crear(context.Background(), n3)

	todas, total, err := repo.ListarPorUsuario(context.Background(), usuarioID, nil, paginacion.PorDefecto())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(todas) != 2 || total != 2 {
		t.Fatalf("esperaba 2 notificaciones sin filtrar, obtuve %d (total %d)", len(todas), total)
	}

	noLeida := domain.NoLeida
	soloNoLeidas, total, err := repo.ListarPorUsuario(context.Background(), usuarioID, &noLeida, paginacion.PorDefecto())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(soloNoLeidas) != 1 || soloNoLeidas[0].ID != n1.ID {
		t.Fatalf("esperaba solo n1 (NO_LEIDA), obtuve %+v", soloNoLeidas)
	}
	// El total sale de COUNT(*) OVER(), que cuenta después del WHERE y antes
	// del LIMIT: con el filtro puesto tiene que contar 1, no 2.
	if total != 1 {
		t.Errorf("total = %d, esperaba 1 con el filtro de estado aplicado", total)
	}
}

// La paginación se verifica contra Postgres real y no solo con el fake porque
// lo que puede salir mal es el SQL: el orden de $n del LIMIT/OFFSET respecto
// de los args del filtro, y que COUNT(*) OVER() cuente antes del recorte.
func TestPostgresRepo_ListarPorUsuario_PaginaYTotal(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	usuarioID := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")

	base := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 5; i++ {
		// Fechas distintas para que el ORDER BY creada_en DESC sea inequívoco.
		n, _ := domain.NuevaNotificacion(NuevoID(), usuarioID, fmt.Sprintf("aviso %d", i), domain.TipoReservaCancelada, domain.Referencias{},
			base.Add(time.Duration(i)*time.Minute))
		if err := repo.Crear(context.Background(), n); err != nil {
			t.Fatalf("creando notificación de test: %v", err)
		}
	}

	primera, total, err := repo.ListarPorUsuario(context.Background(), usuarioID, nil,
		paginacion.Pagina{Numero: 1, Tamanio: 2})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(primera) != 2 || total != 5 {
		t.Fatalf("primera página: %d filas, total %d; esperaba 2 y 5", len(primera), total)
	}
	if primera[0].Mensaje != "aviso 4" {
		t.Errorf("la más reciente debería venir primera, obtuve %q", primera[0].Mensaje)
	}

	tercera, total, err := repo.ListarPorUsuario(context.Background(), usuarioID, nil,
		paginacion.Pagina{Numero: 3, Tamanio: 2})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(tercera) != 1 || total != 5 {
		t.Fatalf("tercera página: %d filas, total %d; esperaba 1 y 5", len(tercera), total)
	}

	// Más allá del final: sin filas de las que leer COUNT(*) OVER(), el total
	// tiene que salir igual de la consulta de respaldo.
	vacia, total, err := repo.ListarPorUsuario(context.Background(), usuarioID, nil,
		paginacion.Pagina{Numero: 9, Tamanio: 2})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(vacia) != 0 || total != 5 {
		t.Fatalf("página vacía: %d filas, total %d; esperaba 0 y 5", len(vacia), total)
	}
}

func TestListadorAdminsPostgres_SoloAdminsAprobados(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	admin1 := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	admin2 := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	crearUsuarioDeTest(t, pool, "ADMIN", "PENDIENTE")  // no cuenta
	crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA") // no cuenta

	listador := NewListadorAdminsPostgres(pool)
	ids, err := listador.IDsDeAdminsAprobados(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("esperaba 2 admins aprobados, obtuve %d: %v", len(ids), ids)
	}
	encontrados := map[string]bool{ids[0]: true}
	if len(ids) > 1 {
		encontrados[ids[1]] = true
	}
	if !encontrados[admin1] || !encontrados[admin2] {
		t.Errorf("no aparecen los dos admins esperados: %v", ids)
	}
}

func TestPostgresRepo_IDConFormatoInvalido_ErrorControlado(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	casos := []struct {
		nombre string
		fn     func() error
	}{
		{"BuscarPorID", func() error { _, err := repo.BuscarPorID(ctx, "NOTIFICACION_ID"); return err }},
		{"ListarPorUsuario", func() error {
			_, _, err := repo.ListarPorUsuario(ctx, "USUARIO_ID", nil, paginacion.PorDefecto())
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

// El aviso de una cuenta pendiente guarda de quién habla, para poder cerrarlo
// cuando esa cuenta se resuelve.
func TestPostgresRepo_ListarNoLeidasSobreUsuario(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	admin1 := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	admin2 := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	pendiente := crearUsuarioDeTest(t, pool, "DOCENTE", "PENDIENTE")
	otro := crearUsuarioDeTest(t, pool, "DOCENTE", "PENDIENTE")

	ahora := time.Now().UTC().Truncate(time.Microsecond)
	nueva := func(destinatario, sobre string, tipo domain.Tipo) *domain.Notificacion {
		n, err := domain.NuevaNotificacion(NuevoID(), destinatario, "pendiente de aprobación",
			tipo, domain.Referencias{SobreUsuarioID: &sobre}, ahora)
		if err != nil {
			t.Fatalf("error de dominio inesperado: %v", err)
		}
		if err := repo.Crear(ctx, n); err != nil {
			t.Fatalf("creando notificación: %v", err)
		}
		return n
	}

	// El mismo pendiente le llega a los dos Admin.
	nueva(admin1, pendiente, domain.TipoDocentePendiente)
	nueva(admin2, pendiente, domain.TipoDocentePendiente)
	// Otra persona, y un tipo distinto: no tienen que aparecer.
	nueva(admin1, otro, domain.TipoDocentePendiente)
	nueva(admin1, pendiente, domain.TipoGeneral)

	encontradas, err := repo.ListarNoLeidasSobreUsuario(ctx, pendiente, domain.TipoDocentePendiente)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(encontradas) != 2 {
		t.Fatalf("esperaba los avisos de los dos Admin, obtuve %d", len(encontradas))
	}
	for _, n := range encontradas {
		if n.SobreUsuarioID == nil || *n.SobreUsuarioID != pendiente {
			t.Errorf("aviso mal referenciado: %+v", n.SobreUsuarioID)
		}
	}

	// Una vez leída sale del conjunto: la consulta es "lo que queda por
	// resolver", no "lo que alguna vez se avisó".
	leida := encontradas[0]
	if err := leida.MarcarLeida(ahora); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.Guardar(ctx, leida); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	quedan, err := repo.ListarNoLeidasSobreUsuario(ctx, pendiente, domain.TipoDocentePendiente)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(quedan) != 1 {
		t.Errorf("esperaba 1 sin leer, obtuve %d", len(quedan))
	}
}

// MarcarLeidasPorTipo cierra de una todas las NO_LEIDA de un tipo, de todos
// los usuarios: es el cierre de los avisos que hablan de un conjunto de cosas
// y no de una persona (licencias por renovar, equipos que quedaron afuera).
//
// Lo que este test protege es que el UPDATE esté acotado por las DOS
// condiciones. Sin el filtro de tipo se llevaría puesta la campana entera; sin
// el de estado pisaría el `leida_en` de avisos que alguien ya había leído
// hace días, y el historial pasaría a decir que se leyeron todos hoy.
func TestPostgresRepo_MarcarLeidasPorTipo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	admin1 := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	admin2 := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")

	ahora := time.Now().UTC().Truncate(time.Microsecond)
	nueva := func(destinatario string, tipo domain.Tipo) *domain.Notificacion {
		n, err := domain.NuevaNotificacion(NuevoID(), destinatario, "hay licencias por renovar",
			tipo, domain.Referencias{}, ahora)
		if err != nil {
			t.Fatalf("error de dominio inesperado: %v", err)
		}
		if err := repo.Crear(ctx, n); err != nil {
			t.Fatalf("creando notificación: %v", err)
		}
		return n
	}

	// El aviso de licencias, a los dos Admin.
	nueva(admin1, domain.TipoLicenciaPorVencer)
	nueva(admin2, domain.TipoLicenciaPorVencer)
	// Un aviso de otro tipo, sin leer: no se puede tocar.
	otroTipo := nueva(admin1, domain.TipoDocentePendiente)
	// Y uno del MISMO tipo que alguien ya leyó ayer: tampoco, porque
	// reescribirlo movería su leida_en a hoy.
	yaLeido := nueva(admin2, domain.TipoLicenciaPorVencer)
	ayer := ahora.AddDate(0, 0, -1)
	if err := yaLeido.MarcarLeida(ayer); err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.Guardar(ctx, yaLeido); err != nil {
		t.Fatalf("guardando el ya leído: %v", err)
	}

	cerradas, err := repo.MarcarLeidasPorTipo(ctx, domain.TipoLicenciaPorVencer, ahora)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if cerradas != 2 {
		t.Fatalf("esperaba cerrar los 2 sin leer de ese tipo, cerró %d", cerradas)
	}

	// El de otro tipo sigue esperando.
	sigueAbierto, err := repo.BuscarPorID(ctx, otroTipo.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if sigueAbierto.Estado != domain.NoLeida {
		t.Error("el cierre por tipo se llevó puesto un aviso de otro tipo")
	}

	// Y el que ya estaba leído conserva CUÁNDO se leyó.
	intacto, err := repo.BuscarPorID(ctx, yaLeido.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if intacto.LeidaEn == nil || !intacto.LeidaEn.Equal(ayer) {
		t.Errorf("se pisó la fecha en que se había leído: %v, esperaba %v", intacto.LeidaEn, ayer)
	}

	// Y es idempotente: correrlo de nuevo no cierra nada porque no queda nada.
	cerradas, err = repo.MarcarLeidasPorTipo(ctx, domain.TipoLicenciaPorVencer, ahora)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if cerradas != 0 {
		t.Errorf("no quedaba nada por cerrar, cerró %d", cerradas)
	}
}
