//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/auth/application"
	"github.com/agustin-y2k/sgrc/internal/auth/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

// Servicio real, repositorio real y una intercalación determinista tras leer.
type repoSeguridadIntercalado struct {
	application.Repo
	despuesDeLeer func()
}

func (r *repoSeguridadIntercalado) BuscarPorID(ctx context.Context, id string) (*domain.Usuario, error) {
	u, err := r.Repo.BuscarPorID(ctx, id)
	if err == nil && r.despuesDeLeer != nil {
		cambiar := r.despuesDeLeer
		r.despuesDeLeer = nil
		cambiar()
	}
	return u, err
}

func servicioDeSeguridad(repo application.Repo, firmar application.TokenSigner) *application.Service {
	return application.NewService(repo, eventbus.NewInMemoryEventBus(),
		func(s string) (string, error) { return "hash:" + s, nil },
		func(s, h string) (bool, error) { return h == "hash:"+s, nil },
		firmar, NuevoID, func() (string, error) { return "temporal", nil }, GenerarCodigoRecuperacion,
		time.Now, sinMaterias{}, sinCancelaciones{}, nil, true)
}

func TestSeguridad_PerfilNoRevierteCambiosConcurrentes(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	cambios := []struct {
		nombre  string
		aplicar func(*domain.Usuario) (*domain.Usuario, error)
	}{
		{"baja", func(u *domain.Usuario) (*domain.Usuario, error) {
			revocado, err := repo.ActualizarPassword(ctx, u, "hash:temporal", true)
			if err != nil {
				return nil, err
			}
			return repo.CambiarEstado(ctx, revocado, domain.EstadoBaja, time.Now())
		}},
		{"password", func(u *domain.Usuario) (*domain.Usuario, error) {
			return repo.ActualizarPassword(ctx, u, "hash:temporal", true)
		}},
		{"degradacion", func(u *domain.Usuario) (*domain.Usuario, error) { return repo.CambiarRol(ctx, u, domain.RolDocente) }},
	}
	for i, cambio := range cambios {
		t.Run(cambio.nombre, func(t *testing.T) {
			u := usuarioDeTest(fmt.Sprintf("perfil-%d@example.test", i), domain.RolAdmin, domain.EstadoAprobada)
			if err := repo.Crear(ctx, u); err != nil {
				t.Fatal(err)
			}
			var esperado *domain.Usuario
			intercalado := &repoSeguridadIntercalado{Repo: repo, despuesDeLeer: func() {
				var err error
				esperado, err = cambio.aplicar(u)
				if err != nil {
					t.Fatal(err)
				}
			}}
			firmas := 0
			svc := servicioDeSeguridad(intercalado, func(*domain.Usuario, bool) (string, error) { firmas++; return "token", nil })
			perfil, token, err := svc.ActualizarMisDatos(ctx, u.ID, "Nuevo", "Nombre", false, 0)
			if !errors.Is(err, application.ErrUsuarioModificado) || perfil != nil || token != "" || firmas != 0 {
				t.Fatalf("perfil obsoleto: usuario=%v token=%q firmas=%d error=%v", perfil, token, firmas, err)
			}
			actual, err := repo.BuscarPorID(ctx, u.ID)
			if err != nil {
				t.Fatal(err)
			}
			if actual.Estado != esperado.Estado || actual.Rol != esperado.Rol || actual.PasswordHash != esperado.PasswordHash || actual.VersionSesion != esperado.VersionSesion || actual.DebeCambiarPassword != esperado.DebeCambiarPassword || actual.Nombre != u.Nombre {
				t.Fatalf("se perdio un cambio confirmado: actual=%+v esperado=%+v", actual, esperado)
			}
		})
	}
}

func TestSeguridad_EscriturasEspecificasPreservanCamposActuales(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	u := usuarioDeTest("campos@example.test", domain.RolDocente, domain.EstadoAprobada)
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatal(err)
	}
	perfil, err := repo.ActualizarPerfil(ctx, u, "Ana", "Nuevo")
	if err != nil {
		t.Fatal(err)
	}
	// El cambio de password se decidió antes del perfil y debe preservar su nombre.
	password, err := repo.ActualizarPassword(ctx, u, "hash:nueva", true)
	if err != nil {
		t.Fatal(err)
	}
	if password.Nombre != perfil.Nombre || password.Apellido != perfil.Apellido || password.VersionSesion != 1 {
		t.Fatalf("password sobrescribio perfil: %+v", password)
	}
	// Un token revocado antes de la lectura tampoco puede renovarse.
	firmas := 0
	svc := servicioDeSeguridad(repo, func(*domain.Usuario, bool) (string, error) { firmas++; return "token", nil })
	if _, token, err := svc.ActualizarMisDatos(ctx, u.ID, "Ana", "Nuevo", false, 0); !errors.Is(err, application.ErrUsuarioModificado) || token != "" || firmas != 0 {
		t.Fatalf("se renovo una sesion revocada: token=%q firmas=%d error=%v", token, firmas, err)
	}
	vigente, token, err := svc.ActualizarMisDatos(ctx, u.ID, "Ana", "Nuevo", false, 1)
	if err != nil || token == "" || vigente.VersionSesion != 1 {
		t.Fatalf("una sesion vigente debe poder editar el perfil: usuario=%v token=%q error=%v", vigente, token, err)
	}
	// Una decisión de rol o vínculo Google anterior al reset no restaura el hash.
	rol, err := repo.CambiarRol(ctx, u, domain.RolAdmin)
	if err != nil {
		t.Fatal(err)
	}
	google, err := repo.VincularGoogle(ctx, u, "sub-campos")
	if err != nil {
		t.Fatal(err)
	}
	for _, actual := range []*domain.Usuario{rol, google} {
		if actual.PasswordHash != password.PasswordHash || actual.VersionSesion != 1 || !actual.DebeCambiarPassword || actual.Nombre != perfil.Nombre {
			t.Fatalf("se sobrescribio seguridad o perfil: %+v", actual)
		}
	}
	// El estado tampoco debe deshacer password, perfil ni Google.
	baja, err := repo.CambiarEstado(ctx, google, domain.EstadoBaja, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if baja.PasswordHash != password.PasswordHash || baja.VersionSesion != 1 || baja.GoogleSub != "sub-campos" || baja.Nombre != perfil.Nombre {
		t.Fatalf("baja sobrescribio campos: %+v", baja)
	}
	if _, err := repo.CambiarRol(ctx, rol, domain.RolDocente); !errors.Is(err, application.ErrUsuarioModificado) {
		t.Fatalf("rol obsoleto tras baja: %v", err)
	}
	if _, err := repo.VincularGoogle(ctx, rol, "sub-tardio"); !errors.Is(err, application.ErrUsuarioModificado) {
		t.Fatalf("vinculo obsoleto tras baja: %v", err)
	}
	if _, err := repo.ActualizarPassword(ctx, password, "hash:tardia", false); !errors.Is(err, application.ErrUsuarioModificado) {
		t.Fatalf("password obsoleta tras baja: %v", err)
	}
}

func TestSeguridad_DosPasswordsConcurrentesNoPierdenRevocacion(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	u := usuarioDeTest("passwords@example.test", domain.RolDocente, domain.EstadoAprobada)
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatal(err)
	}
	empezar := make(chan struct{})
	resultados := make(chan error, 2)
	var listos sync.WaitGroup
	listos.Add(2)
	for _, hash := range []string{"hash:uno", "hash:dos"} {
		go func() {
			anterior, err := repo.BuscarPorID(ctx, u.ID)
			listos.Done()
			<-empezar
			if err == nil {
				_, err = repo.ActualizarPassword(ctx, anterior, hash, false)
			}
			resultados <- err
		}()
	}
	listos.Wait()
	close(empezar)
	exitos, conflictos := 0, 0
	for range 2 {
		err := <-resultados
		switch {
		case err == nil:
			exitos++
		case errors.Is(err, application.ErrUsuarioModificado):
			conflictos++
		default:
			t.Fatal(err)
		}
	}
	if exitos != 1 || conflictos != 1 {
		t.Fatalf("exitos=%d conflictos=%d", exitos, conflictos)
	}
	actual, err := repo.BuscarPorID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if actual.VersionSesion != 1 {
		t.Fatalf("revocacion incorrecta: %+v", actual)
	}
	siguiente, err := repo.ActualizarPassword(ctx, actual, "hash:siguiente", true)
	if err != nil {
		t.Fatal(err)
	}
	if siguiente.VersionSesion != 2 {
		t.Fatalf("revocacion no monotona: %+v", siguiente)
	}
}

func TestSeguridad_RecuperacionObsoletaNoConsumeCodigo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	u := usuarioDeTest("recuperacion@example.test", domain.RolDocente, domain.EstadoAprobada)
	u.PasswordHash = "hash:anterior"
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatal(err)
	}
	codigo, err := domain.NuevoCodigoRecuperacion(NuevoID(), u.ID, "hash:123456", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CrearCodigoRecuperacion(ctx, codigo); err != nil {
		t.Fatal(err)
	}
	// Un reset confirma después de la lectura inicial y antes de adquirir el
	// bloqueo de recuperación. La copia anterior debe rechazarse sin consumir
	// el código ni sobrescribir la contraseña nueva.
	intercalado := &repoCambioAntesRecuperacion{Repo: repo, cambiar: func() {
		if _, err := repo.ActualizarPassword(ctx, u, "hash:temporal", true); err != nil {
			t.Fatal(err)
		}
	}}
	svc := servicioDeSeguridad(intercalado, func(*domain.Usuario, bool) (string, error) { return "token", nil })
	if _, err := svc.RestablecerPasswordConCodigo(ctx, u.Email, "123456", "password-nueva"); !errors.Is(err, application.ErrUsuarioModificado) {
		t.Fatalf("recuperacion obsoleta: %v", err)
	}
	vigente, err := repo.BuscarCodigoVigenteDe(ctx, u.ID)
	if err != nil {
		t.Fatalf("el rollback perdio el codigo: %v", err)
	}
	if vigente.UsadoEn != nil {
		t.Fatal("se consumio el codigo pese al conflicto")
	}
	actual, err := repo.BuscarPorID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if actual.PasswordHash != "hash:temporal" || actual.VersionSesion != 1 || !actual.DebeCambiarPassword {
		t.Fatalf("se perdio el reset: %+v", actual)
	}
}

type repoCambioAntesRecuperacion struct {
	application.Repo
	cambiar func()
}

func (r *repoCambioAntesRecuperacion) EnRecuperacion(ctx context.Context, id string, fn func(context.Context, application.Repo) error) error {
	r.cambiar()
	return r.Repo.EnRecuperacion(ctx, id, fn)
}

func TestSeguridad_CambioPasswordFirmaLosDatosPersistidos(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	u := usuarioDeTest("token@example.test", domain.RolAdmin, domain.EstadoAprobada)
	u.PasswordHash = "hash:anterior"
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatal(err)
	}
	intercalado := &repoSeguridadIntercalado{Repo: repo, despuesDeLeer: func() {
		perfil, err := repo.ActualizarPerfil(ctx, u, "Nombre nuevo", "Apellido nuevo")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.CambiarRol(ctx, perfil, domain.RolDocente); err != nil {
			t.Fatal(err)
		}
	}}
	var firmado domain.Usuario
	recordarme := false
	svc := servicioDeSeguridad(intercalado, func(actual *domain.Usuario, larga bool) (string, error) {
		firmado = *actual
		recordarme = larga
		return "token", nil
	})
	token, err := svc.CambiarPassword(ctx, u.ID, "anterior", "password-nueva", true)
	if err != nil {
		t.Fatal(err)
	}
	if token != "token" || !recordarme || firmado.Nombre != "Nombre nuevo" || firmado.Apellido != "Apellido nuevo" || firmado.Rol != domain.RolDocente || firmado.VersionSesion != 1 || firmado.DebeCambiarPassword {
		t.Fatalf("se firmaron datos obsoletos: %+v recordarme=%v token=%q", firmado, recordarme, token)
	}
}
