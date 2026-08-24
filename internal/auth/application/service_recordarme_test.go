package application

import (
	"context"
	"testing"

	"github.com/ramiro/sgrc/internal/auth/domain"
)

// La casilla "mantener la sesión iniciada" (RF-01.13) no decide nada en el
// dominio: lo único que hace es llegar entera hasta la firma. Estos tests
// vigilan justamente eso, porque un `false` olvidado en el camino no rompe
// ningún test existente — la sesión simplemente dura menos de lo prometido.
//
// firmarFalso devuelve "token-largo-de-…" cuando recibe recordarme=true.

func TestLogin_LaCasillaLlegaHastaLaFirma(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{
		ID: "u1", Email: "ada@x.com", PasswordHash: "hash:password123", Estado: domain.EstadoAprobada,
	}
	svc := nuevoServicioDeTest(repo)

	res, err := svc.Login(context.Background(), "ada@x.com", "password123", true)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if res.Token != "token-largo-de-u1" {
		t.Fatalf("la casilla no llegó a la firma: token %q", res.Token)
	}
}

func TestLoginConGoogle_LaCasillaLlegaHastaLaFirma(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = usuarioDeGoogle("u1", "ada@escuela.edu.ar", "112233445566", domain.EstadoAprobada)
	svc := nuevoServicioConGoogle(repo, &fakeVerificadorGoogle{identidad: identidadDePrueba()})

	res, err := svc.LoginConGoogle(context.Background(), "un-token", true)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if res.Token != "token-largo-de-u1" {
		t.Fatalf("la casilla no llegó a la firma: token %q", res.Token)
	}
}

// Las dos operaciones que vuelven a firmar tienen que conservar la duración de
// la sesión en curso. Si la degradaran a la corta, alguien que tildó la casilla
// y después edita su perfil se encontraría afuera al día siguiente sin
// entender por qué.

func TestCambiarPassword_ConservaLaSesionLarga(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := nuevoServicioDeTest(repo)

	token, err := svc.CambiarPassword(context.Background(), u.ID, "la-vieja", "una-contraseña-nueva", true)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if token != "token-largo-de-"+u.ID {
		t.Fatalf("el token nuevo perdió la vigencia larga: %q", token)
	}
}

func TestActualizarMisDatos_ConservaLaSesionLarga(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := nuevoServicioDeTest(repo)

	_, token, err := svc.ActualizarMisDatos(context.Background(), u.ID, "Ada", "Lovelace", true)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if token != "token-largo-de-"+u.ID {
		t.Fatalf("el token nuevo perdió la vigencia larga: %q", token)
	}
}

// El contrapunto: sin casilla, la sesión sigue siendo la corta después de
// re-firmar. Si no, "recordarme" sería el comportamiento por omisión.
func TestCambiarPassword_SinCasilla_NoAlargaLaSesion(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.edu.ar")
	svc := nuevoServicioDeTest(repo)

	token, err := svc.CambiarPassword(context.Background(), u.ID, "la-vieja", "una-contraseña-nueva", false)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if token != "token-de-"+u.ID {
		t.Fatalf("una sesión corta no puede volverse larga al cambiar la contraseña: %q", token)
	}
}
