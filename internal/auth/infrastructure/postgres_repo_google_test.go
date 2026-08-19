//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/auth/application"
	"github.com/ramiro/sgrc/internal/auth/domain"
)

// Lo que agregó migrations/001_esquema_inicial.sql solo se puede probar
// contra Postgres de verdad: que password_hash pueda ser NULL, que google_sub
// sea único, y que el CHECK impida una cuenta sin ninguna forma de entrar.

func usuarioDeGoogleDeTest(email, sub string) *domain.Usuario {
	return &domain.Usuario{
		ID:       NuevoID(),
		Nombre:   "Ada",
		Apellido: "Lovelace",
		Email:    email,
		// Sin PasswordHash: es exactamente lo que crea RegistrarConGoogle.
		GoogleSub:     sub,
		Rol:           domain.RolDocente,
		Estado:        domain.EstadoPendiente,
		FechaRegistro: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func TestPostgresRepo_CuentaDeGoogle_SeGuardaSinPasswordYSeBuscaPorSub(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	u := usuarioDeGoogleDeTest("ada@escuela.edu.ar", "112233445566")
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatalf("una cuenta sin contraseña tiene que poder guardarse: %v", err)
	}

	encontrado, err := repo.BuscarPorGoogleSub(ctx, "112233445566")
	if err != nil {
		t.Fatalf("no debería fallar buscando por sub: %v", err)
	}
	if encontrado.ID != u.ID {
		t.Errorf("encontró otra cuenta: %s", encontrado.ID)
	}
	// El NULL de la base tiene que volver como "" y no romper el Scan.
	if encontrado.PasswordHash != "" {
		t.Errorf("password_hash NULL tendría que leerse como vacío, leyó %q", encontrado.PasswordHash)
	}
	if encontrado.PuedeIngresarConPassword() {
		t.Error("una cuenta de Google no puede ingresar con contraseña")
	}
	if !encontrado.PuedeIngresarConGoogle() {
		t.Error("la cuenta tendría que poder ingresar con Google")
	}

	// La misma cuenta leída por email tiene que decir lo mismo: el mapeo es
	// el mismo, pero pasa por otra consulta.
	porEmail, err := repo.BuscarPorEmail(ctx, "ada@escuela.edu.ar")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if porEmail.GoogleSub != "112233445566" || porEmail.PasswordHash != "" {
		t.Errorf("inconsistencia entre consultas: %+v", porEmail)
	}

	if _, err := repo.BuscarPorGoogleSub(ctx, "otro-sub"); !errors.Is(err, application.ErrUsuarioNoEncontrado) {
		t.Errorf("esperaba ErrUsuarioNoEncontrado, hubo: %v", err)
	}
}

// Un sub vacío no puede empatar con las cuentas que no tienen ninguno, que
// en una base recién migrada son todas.
func TestPostgresRepo_BuscarPorGoogleSubVacio_NoEmpataConLasCuentasSinVincular(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	if err := repo.Crear(ctx, usuarioDeTest("con-password@escuela.edu.ar", domain.RolDocente, domain.EstadoAprobada)); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if _, err := repo.BuscarPorGoogleSub(ctx, ""); !errors.Is(err, application.ErrUsuarioNoEncontrado) {
		t.Fatalf("un sub vacío no puede encontrar a nadie, hubo: %v", err)
	}
}

// El caso del docente que ya tenía cuenta con contraseña y ahora entra con
// Google: se le agrega el vínculo y conserva la contraseña.
func TestPostgresRepo_VincularGoogleAUnaCuentaConPassword(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	u := usuarioDeTest("ada@escuela.edu.ar", domain.RolDocente, domain.EstadoAprobada)
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	u.GoogleSub = "112233445566"
	if err := repo.Guardar(ctx, u); err != nil {
		t.Fatalf("no debería fallar vinculando: %v", err)
	}

	encontrado, err := repo.BuscarPorGoogleSub(ctx, "112233445566")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if encontrado.PasswordHash != "hash-de-prueba" {
		t.Errorf("vincular Google no puede borrar la contraseña: %q", encontrado.PasswordHash)
	}
	if !encontrado.PuedeIngresarConPassword() || !encontrado.PuedeIngresarConGoogle() {
		t.Error("la cuenta tendría que poder entrar de las dos formas")
	}
}

// El índice único sobre google_sub es lo que impide que dos cuentas
// reclamen la misma identidad de Google.
func TestPostgresRepo_DosCuentasConElMismoGoogleSub_LasRechazaLaBase(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	if err := repo.Crear(ctx, usuarioDeGoogleDeTest("ada@escuela.edu.ar", "112233445566")); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	err := repo.Crear(ctx, usuarioDeGoogleDeTest("otra@escuela.edu.ar", "112233445566"))

	if err == nil {
		t.Fatal("dos cuentas no pueden compartir la misma cuenta de Google")
	}
	if !errors.Is(err, application.ErrEmailYaRegistrado) {
		t.Errorf("esperaba el error de duplicado, hubo: %v", err)
	}
}

// El índice es PARCIAL (WHERE google_sub IS NOT NULL): tiene que dejar
// convivir cualquier cantidad de cuentas sin vincular.
func TestPostgresRepo_VariasCuentasSinGoogle_Conviven(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	for _, email := range []string{"a@escuela.edu.ar", "b@escuela.edu.ar", "c@escuela.edu.ar"} {
		if err := repo.Crear(ctx, usuarioDeTest(email, domain.RolDocente, domain.EstadoAprobada)); err != nil {
			t.Fatalf("las cuentas sin google_sub tienen que convivir: %v", err)
		}
	}
}

// El CHECK chk_usuario_credencial: una cuenta sin contraseña Y sin Google no
// se puede entrar de ninguna forma, así que la base no la acepta.
func TestPostgresRepo_CuentaSinNingunaCredencial_LaRechazaLaBase(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	u := usuarioDeTest("fantasma@escuela.edu.ar", domain.RolDocente, domain.EstadoAprobada)
	u.PasswordHash = ""

	if err := repo.Crear(ctx, u); err == nil {
		t.Fatal("una cuenta sin contraseña ni Google no tendría que poder existir")
	}
}
