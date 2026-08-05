// Package authtest arma la middleware.Autenticacion que necesitan los tests
// de interfaces/http de cada paquete, con una verificación de cuenta vigente
// en memoria en lugar de Postgres.
//
// Vive en shared/ y no duplicado en cada paquete de test por el mismo
// criterio que internal/shared/testdb: son siete harnesses que necesitan
// exactamente lo mismo, y si el contrato de Autenticacion cambia conviene
// que haya un solo lugar que actualizar en vez de siete que se pueden
// olvidar.
//
// No lo usa ningún código de producción — cmd/main.go arma su Autenticacion
// con authinfra.NewVerificadorCuentaVigente, contra la base real.
package authtest

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// Registro hace de "tabla usuario" para los tests: recuerda el rol de cada
// ID al que se le emitió un token, para que Autenticacion pueda responder lo
// mismo que respondería la base.
//
// Sin sincronización a propósito: los tests de este repo no usan
// t.Parallel(), y un mutex acá solo escondería el día que alguien lo agregue
// (el race detector lo va a marcar, que es lo que queremos).
type Registro struct {
	roles       map[string]string
	dadosDeBaja map[string]bool
}

func Nuevo() *Registro {
	return &Registro{roles: map[string]string{}, dadosDeBaja: map[string]bool{}}
}

// Token firma un JWT válido para ese usuario y deja registrado su rol.
// Reusa exactamente el mismo formato que produce
// auth/infrastructure.JWTFirmador, para que los tests ejerciten el
// middleware real y no una imitación.
func (r *Registro) Token(secret []byte, id, rol string) string {
	r.roles[id] = rol
	claims := &middleware.Claims{
		UserID: id,
		Rol:    rol,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	firmado, _ := token.SignedString(secret)
	return firmado
}

// DarDeBaja simula lo que hace RF-02.8 en la base: la cuenta deja de estar
// APROBADA. El token que ya se emitió sigue siendo criptográficamente
// válido — de eso se trata la prueba.
func (r *Registro) DarDeBaja(id string) {
	r.dadosDeBaja[id] = true
}

// Autenticacion devuelve la pieza que se le pasa a RegisterRoutes.
func (r *Registro) Autenticacion(secret []byte) middleware.Autenticacion {
	return middleware.Autenticacion{
		Secret: secret,
		Vigente: func(_ context.Context, usuarioID string) (bool, string, error) {
			if r.dadosDeBaja[usuarioID] {
				return false, "", nil
			}
			rol, existe := r.roles[usuarioID]
			return existe, rol, nil
		},
	}
}
