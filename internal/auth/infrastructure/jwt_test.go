package infrastructure

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/middleware"
)

const secretoDePrueba = "un-secreto-de-prueba-suficientemente-largo"

func usuarioDePrueba() *domain.Usuario {
	return &domain.Usuario{
		ID:            "u1",
		Rol:           domain.RolDocente,
		Nombre:        "Ada",
		Apellido:      "Lovelace",
		VersionSesion: 3,
	}
}

// claimsDe verifica la firma y devuelve los claims, igual que hace el
// middleware con cada request.
func claimsDe(t *testing.T, token string) *middleware.Claims {
	t.Helper()
	claims := &middleware.Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return []byte(secretoDePrueba), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("el token que acabamos de firmar no valida: %v", err)
	}
	return claims
}

func TestFirmar_SinRecordarme_UsaLaVigenciaNormal(t *testing.T) {
	f := NewJWTFirmador([]byte(secretoDePrueba), time.Hour, 720*time.Hour)

	token, err := f.Firmar(usuarioDePrueba(), false)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	claims := claimsDe(t, token)
	if claims.Recordarme {
		t.Error("el claim rec tiene que quedar en false cuando la casilla no se tildó")
	}
	// Margen de un minuto: entre que se firma y que se compara pasa tiempo real.
	if faltante := time.Until(claims.ExpiresAt.Time); faltante > time.Hour || faltante < 59*time.Minute {
		t.Fatalf("esperaba una vigencia de ~1h, falta %s", faltante)
	}
}

func TestFirmar_ConRecordarme_UsaLaVigenciaLarga(t *testing.T) {
	f := NewJWTFirmador([]byte(secretoDePrueba), time.Hour, 720*time.Hour)

	token, err := f.Firmar(usuarioDePrueba(), true)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	claims := claimsDe(t, token)
	if !claims.Recordarme {
		t.Error("el claim rec tiene que viajar en true: es lo único que le dice a " +
			"las re-firmas posteriores que la sesión era larga")
	}
	if faltante := time.Until(claims.ExpiresAt.Time); faltante > 720*time.Hour || faltante < 719*time.Hour {
		t.Fatalf("esperaba una vigencia de ~720h, falta %s", faltante)
	}
}

// El resto de los claims no puede depender de la casilla: "recordarme" es una
// decisión sobre cuánto dura la sesión, no sobre quién es ni qué puede hacer.
func TestFirmar_LaCasillaNoCambiaLaIdentidadNiElRol(t *testing.T) {
	f := NewJWTFirmador([]byte(secretoDePrueba), time.Hour, 720*time.Hour)
	u := usuarioDePrueba()

	corto, err := f.Firmar(u, false)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	largo, err := f.Firmar(u, true)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	a, b := claimsDe(t, corto), claimsDe(t, largo)
	if a.UserID != b.UserID || a.Rol != b.Rol || a.VersionSesion != b.VersionSesion ||
		a.DebeCambiarPassword != b.DebeCambiarPassword {
		t.Fatalf("los claims de identidad difieren entre sesión corta y larga: %+v vs %+v", a, b)
	}
}
