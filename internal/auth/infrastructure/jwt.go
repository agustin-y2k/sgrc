package infrastructure

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// JWTFirmador crea la función de firma que application.Service necesita
// (application.TokenSigner) — HS256, con el secreto capturado por closure
// para no tener que pasarlo en cada llamada a Firmar.
type JWTFirmador struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTFirmador(secret []byte, ttl time.Duration) *JWTFirmador {
	return &JWTFirmador{secret: secret, ttl: ttl}
}

// Firmar cumple la firma de application.TokenSigner
// (func(u *domain.Usuario) (string, error)) — se pasa como
// f.Firmar directamente al construir el Service.
func (f *JWTFirmador) Firmar(u *domain.Usuario) (string, error) {
	claims := &middleware.Claims{
		UserID:              u.ID,
		Rol:                 string(u.Rol),
		Nombre:              u.Nombre,
		Apellido:            u.Apellido,
		DebeCambiarPassword: u.DebeCambiarPassword,
		// La versión de sesión se toma del usuario tal como está en este
		// momento. Quien invalida las sesiones y además emite un token
		// nuevo (CambiarPassword) tiene que llamar a InvalidarSesiones
		// ANTES de firmar, o el token que entrega nace inválido.
		VersionSesion: u.VersionSesion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(f.ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(f.secret)
}
