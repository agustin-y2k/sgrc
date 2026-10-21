package application

import (
	"context"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/auth/domain"
)

// Intercala un cambio después de la lectura y antes de que el servicio escriba.
// La copia reproduce el contrato de PostgreSQL, incluso con el fake anterior.
type repoConCambioTrasLectura struct {
	Repo
	cambiar func()
}

func (r *repoConCambioTrasLectura) BuscarPorID(ctx context.Context, id string) (*domain.Usuario, error) {
	u, err := r.Repo.BuscarPorID(ctx, id)
	if err != nil {
		return nil, err
	}
	copia := *u
	if r.cambiar != nil {
		cambiar := r.cambiar
		r.cambiar = nil
		cambiar()
	}
	return &copia, nil
}

func TestActualizarMisDatos_NoRevierteCambiosDeSeguridad(t *testing.T) {
	casos := []struct {
		nombre  string
		cambiar func(*domain.Usuario)
	}{
		{"baja y revocacion", func(u *domain.Usuario) { u.Estado = domain.EstadoBaja; u.InvalidarSesiones() }},
		{"reset de password", func(u *domain.Usuario) {
			u.PasswordHash = "hash:temporal"
			u.DebeCambiarPassword = true
			u.InvalidarSesiones()
		}},
		{"degradacion", func(u *domain.Usuario) { u.Rol = domain.RolDocente }},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			repo := nuevoFakeRepo()
			repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Nombre: "Ana", Apellido: "Perez", Estado: domain.EstadoAprobada, Rol: domain.RolAdmin, PasswordHash: "hash:anterior"}
			intercalado := &repoConCambioTrasLectura{Repo: repo, cambiar: func() { caso.cambiar(repo.usuarios["u1"]) }}
			svc := nuevoServicioDeTest(intercalado)
			firmas := 0
			svc.firmar = func(*domain.Usuario, bool) (string, error) { firmas++; return "token", nil }
			u, token, err := svc.ActualizarMisDatos(context.Background(), "u1", "Nuevo", "Nombre", false, 0)
			if err == nil || u != nil || token != "" || firmas != 0 {
				t.Fatalf("una lectura obsoleta no puede emitir sesion: usuario=%v token=%q error=%v firmas=%d", u, token, err, firmas)
			}
			esperado := &domain.Usuario{Estado: domain.EstadoAprobada, Rol: domain.RolAdmin, PasswordHash: "hash:anterior"}
			caso.cambiar(esperado)
			actual := repo.usuarios["u1"]
			if actual.Estado != esperado.Estado || actual.Rol != esperado.Rol || actual.PasswordHash != esperado.PasswordHash || actual.VersionSesion != esperado.VersionSesion || actual.DebeCambiarPassword != esperado.DebeCambiarPassword {
				t.Fatalf("el perfil revirtio seguridad: %+v", actual)
			}
		})
	}
}

func TestCambiarPassword_NoSobrescribeUnResetConcurrente(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.test")
	svc := nuevoServicioDeTest(repo)
	svc.hash = func(string) (string, error) {
		repo.usuarios[u.ID].PasswordHash = "hash:temporal"
		repo.usuarios[u.ID].DebeCambiarPassword = true
		repo.usuarios[u.ID].InvalidarSesiones()
		return "hash:nueva", nil
	}
	token, err := svc.CambiarPassword(context.Background(), u.ID, "la-vieja", "una-password-nueva", false)
	if err == nil || token != "" {
		t.Fatalf("un cambio validado con la password vieja no puede ganar al reset: token=%q error=%v", token, err)
	}
	actual := repo.usuarios[u.ID]
	if actual.PasswordHash != "hash:temporal" || actual.VersionSesion != 1 || !actual.DebeCambiarPassword {
		t.Fatalf("se perdio el reset: %+v", actual)
	}
}

func TestActualizarMisDatos_NoRenuevaSesionRevocadaAntesDeLeer(t *testing.T) {
	repo := nuevoFakeRepo()
	u := docenteAprobado(repo, "ana@escuela.test")
	u.InvalidarSesiones()
	svc := nuevoServicioDeTest(repo)
	_, token, err := svc.ActualizarMisDatos(context.Background(), u.ID, "Nuevo", "Nombre", false, 0)
	if err == nil || token != "" {
		t.Fatalf("una sesion revocada no puede obtener una nueva: token=%q error=%v", token, err)
	}
	if repo.usuarios[u.ID].Nombre == "Nuevo" {
		t.Fatal("se modifico el perfil usando una sesion revocada")
	}
}

func TestActualizarMisDatos_NoFirmaParaCuentaInactiva(t *testing.T) {
	for _, estado := range []domain.Estado{domain.EstadoPendiente, domain.EstadoRechazada, domain.EstadoBaja} {
		t.Run(string(estado), func(t *testing.T) {
			repo := nuevoFakeRepo()
			u := docenteAprobado(repo, "ana@escuela.test")
			u.Estado = estado
			svc := nuevoServicioDeTest(repo)
			_, token, err := svc.ActualizarMisDatos(context.Background(), u.ID, "Nuevo", "Nombre", false, 0)
			if err == nil || token != "" {
				t.Fatalf("se emitio sesion para %s: token=%q error=%v", estado, token, err)
			}
		})
	}
}
