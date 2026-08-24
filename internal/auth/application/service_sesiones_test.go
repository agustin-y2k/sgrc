package application

import (
	"context"
	"testing"

	"github.com/ramiro/sgrc/internal/auth/domain"
)

// Revocación de sesiones.

func TestCambiarPassword_CierraLasSesionesAbiertas(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := nuevoServicioDeTest(repo)

	versionAntes := u.VersionSesion
	if _, err := svc.CambiarPassword(context.Background(), u.ID, "la-vieja", "una-contraseña-nueva", false); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if u.VersionSesion == versionAntes {
		t.Fatal("cambiar la contraseña tiene que invalidar los tokens ya emitidos")
	}
}

func TestCambiarPassword_ElTokenQueDevuelveNoNaceInvalido(t *testing.T) {
	// El orden importa: si se firmara antes de InvalidarSesiones, quien acaba de
	// cambiar su contraseña recibiría un token con la versión vieja y quedaría
	// afuera en el request siguiente — echado por su propio cambio exitoso.
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")

	// firmarConVersion deja registrada la versión que tenía el usuario en el
	// momento exacto de firmar.
	var versionEnElToken int
	svc := servicioConFirmador(repo, func(u *domain.Usuario, _ bool) (string, error) {
		versionEnElToken = u.VersionSesion
		return "token-de-" + u.ID, nil
	})

	if _, err := svc.CambiarPassword(context.Background(), u.ID, "la-vieja", "una-contraseña-nueva", false); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if versionEnElToken != u.VersionSesion {
		t.Fatalf("el token se firmó con la versión %d y la cuenta quedó en %d: nace inválido",
			versionEnElToken, u.VersionSesion)
	}
}

func TestCambiarPassword_ConLaActualEquivocadaNoTocaLasSesiones(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := nuevoServicioDeTest(repo)

	versionAntes := u.VersionSesion
	if _, err := svc.CambiarPassword(context.Background(), u.ID, "la-que-no-es", "una-contraseña-nueva", false); err == nil {
		t.Fatal("esperaba que fallara")
	}

	// Si no, cualquiera podría cerrarle la sesión a otro tirando
	// contraseñas equivocadas contra su cuenta.
	if u.VersionSesion != versionAntes {
		t.Fatal("un intento fallido no puede cerrar sesiones")
	}
}

func TestResetearPassword_CierraLasSesionesAbiertas(t *testing.T) {
	// Es lo que se espera del caso que motiva un reset asistido: alguien perdió
	// el control de su cuenta y pide ayuda.
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := nuevoServicioDeTest(repo)

	versionAntes := u.VersionSesion
	if _, err := svc.ResetearPassword(context.Background(), u.ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if u.VersionSesion == versionAntes {
		t.Fatal("el reset asistido por un Admin tiene que cerrar las sesiones abiertas")
	}
}

func TestRestablecerConCodigo_CierraLasSesionesAbiertas(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")

	versionAntes := u.VersionSesion
	if _, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", codigoDePrueba, "una-contraseña-nueva"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if u.VersionSesion == versionAntes {
		t.Fatal("recuperar la contraseña tiene que cerrar las sesiones abiertas")
	}
}

func TestRestablecerConCodigo_UnCodigoEquivocadoNoCierraSesiones(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := pedirCodigo(t, repo, "ana@escuela.edu.ar")

	versionAntes := u.VersionSesion
	if _, err := svc.RestablecerPasswordConCodigo(context.Background(),
		"ana@escuela.edu.ar", "999999", "una-contraseña-nueva"); err == nil {
		t.Fatal("esperaba que fallara")
	}

	// Si no, el endpoint público de recuperación sería una forma de echar a
	// cualquiera de su sesión sin conocer ni su contraseña ni su código.
	if u.VersionSesion != versionAntes {
		t.Fatal("un código equivocado no puede cerrarle la sesión a nadie")
	}
}

func TestAprobarYDarDeBaja_NoTocanLaVersionDeSesion(t *testing.T) {
	// El estado de la cuenta ya lo verifica el middleware por separado: una
	// cuenta en BAJA no pasa aunque su versión coincida.
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	u.Estado = domain.EstadoPendiente
	svc := nuevoServicioDeTest(repo)

	versionAntes := u.VersionSesion
	if err := svc.Aprobar(context.Background(), u.ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.VersionSesion != versionAntes {
		t.Error("aprobar no tiene por qué cerrar sesiones")
	}

	if err := svc.DarDeBaja(context.Background(), u.ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.VersionSesion != versionAntes {
		t.Error("dar de baja ya se resuelve por el estado, no por la versión")
	}
}
