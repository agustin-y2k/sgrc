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
	// versiones hace de columna usuario.version_sesion (migración 010).
	// Ausente = 0, que es lo mismo que dice el DEFAULT de la columna.
	versiones map[string]int
}

func Nuevo() *Registro {
	return &Registro{
		roles:       map[string]string{},
		dadosDeBaja: map[string]bool{},
		versiones:   map[string]int{},
	}
}

// Token firma un JWT válido para ese usuario y deja registrado su rol.
// Reusa exactamente el mismo formato que produce
// auth/infrastructure.JWTFirmador, para que los tests ejerciten el
// middleware real y no una imitación.
//
// El token sale con la versión de sesión que la cuenta tenga en ESE
// momento, igual que el firmador real: un token emitido antes de un
// InvalidarSesiones queda viejo, y uno emitido después, no.
func (r *Registro) Token(secret []byte, id, rol string) string {
	r.roles[id] = rol
	claims := &middleware.Claims{
		UserID:        id,
		Rol:           rol,
		VersionSesion: r.versiones[id],
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

// InvalidarSesiones simula lo que hace un cambio de contraseña: incrementa
// la versión de la cuenta, así que todo token emitido antes deja de valer.
// La cuenta sigue APROBADA — es lo que distingue este caso del de arriba.
func (r *Registro) InvalidarSesiones(id string) {
	r.versiones[id]++
}

// Autenticacion devuelve la pieza que se le pasa a RegisterRoutes.
func (r *Registro) Autenticacion(secret []byte) middleware.Autenticacion {
	return middleware.Autenticacion{
		Secret: secret,
		Vigente: func(_ context.Context, usuarioID string) (middleware.EstadoDeCuenta, error) {
			if r.dadosDeBaja[usuarioID] {
				return middleware.EstadoDeCuenta{}, nil
			}
			rol, existe := r.roles[usuarioID]
			return middleware.EstadoDeCuenta{
				Vigente:       existe,
				Rol:           rol,
				VersionSesion: r.versiones[usuarioID],
			}, nil
		},
	}
}
