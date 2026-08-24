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
	// ttlRecordarme es la vigencia de los tokens emitidos con "mantener la
	// sesión iniciada" (RF-01.13). Es un segundo plazo y no un múltiplo del
	// primero: son dos decisiones distintas —cuánto dura una jornada de trabajo
	// y cuánto se le confía a un dispositivo propio— y el despliegue las
	// configura por separado.
	ttlRecordarme time.Duration
}

func NewJWTFirmador(secret []byte, ttl, ttlRecordarme time.Duration) *JWTFirmador {
	return &JWTFirmador{secret: secret, ttl: ttl, ttlRecordarme: ttlRecordarme}
}

// Firmar cumple la firma de application.TokenSigner — se pasa como f.Firmar
// directamente al construir el Service. `recordarme` elige entre las dos
// vigencias configuradas.
func (f *JWTFirmador) Firmar(u *domain.Usuario, recordarme bool) (string, error) {
	vigencia := f.ttl
	if recordarme {
		vigencia = f.ttlRecordarme
	}
	claims := &middleware.Claims{
		UserID:              u.ID,
		Rol:                 string(u.Rol),
		Nombre:              u.Nombre,
		Apellido:            u.Apellido,
		DebeCambiarPassword: u.DebeCambiarPassword,
		// La versión de sesión se toma del usuario tal como está en este momento.
		VersionSesion: u.VersionSesion,
		Recordarme:    recordarme,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(vigencia)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(f.secret)
}
