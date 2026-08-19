//go:build integration

// Tests de la persistencia de los códigos de recuperación contra Postgres
// real.
package infrastructure

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/auth/application"
	"github.com/ramiro/sgrc/internal/auth/domain"
)

// usuarioConCodigo deja un usuario creado y devuelve su ID.
func usuarioConCodigo(t *testing.T, repo *PostgresRepo, email string) string {
	t.Helper()
	u := usuarioDeTest(email, domain.RolDocente, domain.EstadoAprobada)
	if err := repo.Crear(context.Background(), u); err != nil {
		t.Fatalf("creando el usuario de prueba: %v", err)
	}
	return u.ID
}

func nuevoCodigo(t *testing.T, usuarioID, hash string) *domain.CodigoRecuperacion {
	t.Helper()
	c, err := domain.NuevoCodigoRecuperacion(NuevoID(), usuarioID, hash,
		time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("armando el código: %v", err)
	}
	return c
}

func TestCodigoRecuperacion_CrearYBuscar(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	usuarioID := usuarioConCodigo(t, repo, "ada@escuela.edu.ar")
	c := nuevoCodigo(t, usuarioID, "hash-del-codigo")

	if err := repo.CrearCodigoRecuperacion(ctx, c); err != nil {
		t.Fatalf("no debería fallar creando: %v", err)
	}

	encontrado, err := repo.BuscarCodigoVigenteDe(ctx, usuarioID)
	if err != nil {
		t.Fatalf("no debería fallar buscando: %v", err)
	}
	if encontrado.ID != c.ID || encontrado.CodigoHash != "hash-del-codigo" {
		t.Errorf("código encontrado no coincide: %+v", encontrado)
	}
	if encontrado.UsadoEn != nil || encontrado.Intentos != 0 {
		t.Errorf("un código recién creado tiene que estar sin usar y sin intentos: %+v", encontrado)
	}
	if !encontrado.ExpiraEn.After(encontrado.CreadoEn) {
		t.Error("expira_en tiene que ser posterior a creado_en")
	}
}

func TestCodigoRecuperacion_SinCodigoDevuelveElErrorDeNegocio(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	usuarioID := usuarioConCodigo(t, repo, "ada@escuela.edu.ar")

	// Nunca pgx.ErrNoRows crudo hacia arriba: application espera su sentinel.
	_, err := repo.BuscarCodigoVigenteDe(ctx, usuarioID)
	if !errors.Is(err, application.ErrCodigoNoEncontrado) {
		t.Fatalf("esperaba ErrCodigoNoEncontrado, obtuve %v", err)
	}
}

func TestCodigoRecuperacion_CrearInvalidaElAnterior(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	usuarioID := usuarioConCodigo(t, repo, "ada@escuela.edu.ar")
	primero := nuevoCodigo(t, usuarioID, "hash-viejo")
	if err := repo.CrearCodigoRecuperacion(ctx, primero); err != nil {
		t.Fatalf("creando el primero: %v", err)
	}

	segundo := nuevoCodigo(t, usuarioID, "hash-nuevo")
	if err := repo.CrearCodigoRecuperacion(ctx, segundo); err != nil {
		t.Fatalf("creando el segundo: %v", err)
	}

	// Es la razón de ser del CTE: el tope de 5 intentos es POR código, así que
	// si los códigos se acumularan, pedir veinte multiplicaría por veinte los
	// intentos disponibles para adivinar.
	vigente, err := repo.BuscarCodigoVigenteDe(ctx, usuarioID)
	if err != nil {
		t.Fatalf("no debería fallar buscando: %v", err)
	}
	if vigente.ID != segundo.ID {
		t.Fatalf("el vigente tiene que ser el último pedido, es %q", vigente.ID)
	}

	// Y el nuevo NO puede haberse invalidado a sí mismo: el UPDATE del CTE corre
	// bajo la misma instantánea que el INSERT, así que no ve la fila que se está
	// creando.
	if vigente.UsadoEn != nil {
		t.Fatal("el código nuevo se invalidó a sí mismo")
	}

	var sinUsar int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM codigo_recuperacion WHERE usuario_id = $1 AND usado_en IS NULL`,
		usuarioID).Scan(&sinUsar); err != nil {
		t.Fatalf("contando códigos: %v", err)
	}
	if sinUsar != 1 {
		t.Fatalf("tiene que quedar exactamente 1 código vigente, hay %d", sinUsar)
	}

	// El viejo se conserva como registro, no se borra.
	var total int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM codigo_recuperacion WHERE usuario_id = $1`, usuarioID).Scan(&total); err != nil {
		t.Fatalf("contando códigos: %v", err)
	}
	if total != 2 {
		t.Errorf("los códigos viejos se conservan como registro; esperaba 2 filas, hay %d", total)
	}
}

func TestCodigoRecuperacion_CrearNoTocaLosDeOtraPersona(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	unaID := usuarioConCodigo(t, repo, "ada@escuela.edu.ar")
	otraID := usuarioConCodigo(t, repo, "grace@escuela.edu.ar")

	if err := repo.CrearCodigoRecuperacion(ctx, nuevoCodigo(t, unaID, "hash-de-ada")); err != nil {
		t.Fatalf("creando el de ada: %v", err)
	}
	if err := repo.CrearCodigoRecuperacion(ctx, nuevoCodigo(t, otraID, "hash-de-grace")); err != nil {
		t.Fatalf("creando el de grace: %v", err)
	}

	if _, err := repo.BuscarCodigoVigenteDe(ctx, unaID); err != nil {
		t.Fatalf("el código de ada tenía que seguir vigente: %v", err)
	}
}

func TestCodigoRecuperacion_GuardarRegistraIntentosYConsumo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	usuarioID := usuarioConCodigo(t, repo, "ada@escuela.edu.ar")
	c := nuevoCodigo(t, usuarioID, "hash-del-codigo")
	if err := repo.CrearCodigoRecuperacion(ctx, c); err != nil {
		t.Fatalf("creando: %v", err)
	}

	c.RegistrarFallo(time.Now().UTC())
	if err := repo.GuardarCodigoRecuperacion(ctx, c); err != nil {
		t.Fatalf("guardando el intento fallido: %v", err)
	}

	leido, err := repo.BuscarCodigoVigenteDe(ctx, usuarioID)
	if err != nil {
		t.Fatalf("buscando: %v", err)
	}
	if leido.Intentos != 1 {
		t.Errorf("esperaba 1 intento persistido, hay %d", leido.Intentos)
	}

	// Y ahora se consume.
	if err := leido.Usar(time.Now().UTC()); err != nil {
		t.Fatalf("usando: %v", err)
	}
	if err := repo.GuardarCodigoRecuperacion(ctx, leido); err != nil {
		t.Fatalf("guardando el consumo: %v", err)
	}
	if _, err := repo.BuscarCodigoVigenteDe(ctx, usuarioID); !errors.Is(err, application.ErrCodigoNoEncontrado) {
		t.Fatalf("un código consumido no puede volver a aparecer como vigente, obtuve %v", err)
	}
}

func TestCodigoRecuperacion_NoSePuedeConsumirDosVeces(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	usuarioID := usuarioConCodigo(t, repo, "ada@escuela.edu.ar")
	c := nuevoCodigo(t, usuarioID, "hash-del-codigo")
	if err := repo.CrearCodigoRecuperacion(ctx, c); err != nil {
		t.Fatalf("creando: %v", err)
	}

	ahora := time.Now().UTC()
	c.UsadoEn = &ahora
	if err := repo.GuardarCodigoRecuperacion(ctx, c); err != nil {
		t.Fatalf("el primer consumo tiene que funcionar: %v", err)
	}

	// El `usado_en IS NULL` del WHERE: el segundo UPDATE no encuentra fila.
	if err := repo.GuardarCodigoRecuperacion(ctx, c); !errors.Is(err, application.ErrCodigoNoEncontrado) {
		t.Fatalf("el segundo consumo tenía que fallar, obtuve %v", err)
	}
}

func TestCodigoRecuperacion_DosPedidosConcurrentesConsumenUnaSolaVez(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	usuarioID := usuarioConCodigo(t, repo, "ada@escuela.edu.ar")
	c := nuevoCodigo(t, usuarioID, "hash-del-codigo")
	if err := repo.CrearCodigoRecuperacion(ctx, c); err != nil {
		t.Fatalf("creando: %v", err)
	}

	// Dos requests con el código correcto llegando a la vez.
	const intentos = 8
	var wg sync.WaitGroup
	resultados := make([]error, intentos)
	for i := 0; i < intentos; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ahora := time.Now().UTC()
			copia := *c
			copia.UsadoEn = &ahora
			resultados[i] = repo.GuardarCodigoRecuperacion(ctx, &copia)
		}(i)
	}
	wg.Wait()

	exitos := 0
	for i, err := range resultados {
		switch {
		case err == nil:
			exitos++
		case errors.Is(err, application.ErrCodigoNoEncontrado):
			// El esperado para los que perdieron.
		default:
			t.Errorf("goroutine %d falló con un error inesperado: %v", i, err)
		}
	}
	if exitos != 1 {
		t.Fatalf("exactamente uno tenía que consumir el código, lo consumieron %d", exitos)
	}
}

func TestCodigoRecuperacion_SeVanConLaCuenta(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	usuarioID := usuarioConCodigo(t, repo, "ada@escuela.edu.ar")
	if err := repo.CrearCodigoRecuperacion(ctx, nuevoCodigo(t, usuarioID, "hash")); err != nil {
		t.Fatalf("creando: %v", err)
	}

	// ON DELETE CASCADE: sin el hard delete de RF-01.9 fallaría por FK, y
	// además un código sin cuenta no le sirve a nadie.
	if err := repo.Eliminar(ctx, usuarioID); err != nil {
		t.Fatalf("eliminando la cuenta: %v", err)
	}

	var quedan int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM codigo_recuperacion WHERE usuario_id = $1`, usuarioID).Scan(&quedan); err != nil {
		t.Fatalf("contando: %v", err)
	}
	if quedan != 0 {
		t.Fatalf("los códigos tenían que irse con la cuenta, quedan %d", quedan)
	}
}

// ══════════════════════════════════════════════════════════════════ Versión
// de sesión
// ══════════════════════════════════════════════════════════════════

func TestVersionSesion_ArrancaEnCeroYSePersiste(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	u := usuarioDeTest("ada@escuela.edu.ar", domain.RolDocente, domain.EstadoAprobada)
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatalf("creando: %v", err)
	}

	// El DEFAULT 0 de la columna es lo que hace que una cuenta creada antes de
	// que existiera el contador no desloguee a nadie: coincide con el claim
	// ausente en los tokens que ya estaban emitidos.
	leido, err := repo.BuscarPorID(ctx, u.ID)
	if err != nil {
		t.Fatalf("buscando: %v", err)
	}
	if leido.VersionSesion != 0 {
		t.Fatalf("esperaba versión 0 al crear, obtuve %d", leido.VersionSesion)
	}

	leido.InvalidarSesiones()
	if err := repo.Guardar(ctx, leido); err != nil {
		t.Fatalf("guardando: %v", err)
	}

	releido, err := repo.BuscarPorID(ctx, u.ID)
	if err != nil {
		t.Fatalf("releyendo: %v", err)
	}
	if releido.VersionSesion != 1 {
		t.Fatalf("la versión no se persistió: esperaba 1, obtuve %d", releido.VersionSesion)
	}
}

func TestVerificadorCuentaVigente_DevuelveLaVersionDeSesion(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	u := usuarioDeTest("ada@escuela.edu.ar", domain.RolDocente, domain.EstadoAprobada)
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatalf("creando: %v", err)
	}
	u.InvalidarSesiones()
	u.InvalidarSesiones()
	if err := repo.Guardar(ctx, u); err != nil {
		t.Fatalf("guardando: %v", err)
	}

	// Es la consulta que corre en CADA request autenticado: si no trajera la
	// versión, la revocación no existiría por más que la columna estuviera bien
	// guardada.
	cuenta, err := NewVerificadorCuentaVigente(pool).Vigente(ctx, u.ID)
	if err != nil {
		t.Fatalf("verificando: %v", err)
	}
	if !cuenta.Vigente {
		t.Fatal("la cuenta está APROBADA, tendría que estar vigente")
	}
	if cuenta.VersionSesion != 2 {
		t.Fatalf("esperaba versión 2, obtuve %d", cuenta.VersionSesion)
	}
}

func TestCodigoRecuperacion_UsuarioInexistenteEsErrorDeCliente(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	c := nuevoCodigo(t, NuevoID(), "hash") // un UUID válido de nadie
	err := repo.CrearCodigoRecuperacion(ctx, c)
	if !errors.Is(err, application.ErrReferenciaInexistente) {
		t.Fatalf("esperaba ErrReferenciaInexistente (23503), obtuve %v", err)
	}
}
