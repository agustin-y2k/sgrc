package domain

import (
	"errors"
	"fmt"
	"time"
)

// VigenciaCodigoRecuperacion es el compromiso entre las dos cosas que se
// rompen: más corto y el docente que revisa el mail desde el celular, tipea
// la dirección del sistema y elige una contraseña llega tarde; más largo y
// el código queda utilizable en una casilla abierta en la sala de
// profesores.
const VigenciaCodigoRecuperacion = 15 * time.Minute

// MaxIntentosCodigoRecuperacion: cuántas veces se puede errar un código
// antes de que se queme y haya que pedir otro.
//
// Seis dígitos son un millón de combinaciones; con cinco intentos la
// probabilidad de acertar a ciegas es 5 en 1.000.000, y quien quiera más
// tiene que pedir un código nuevo — que llega al mail de la persona, no al
// suyo, y además invalida el anterior.
const MaxIntentosCodigoRecuperacion = 5

// LongitudCodigoRecuperacion son los dígitos que ve la persona.
const LongitudCodigoRecuperacion = 6

var (
	// ErrCodigoInvalido cubre las tres formas de fallar (no existe, no
	// coincide, ya se usó) a propósito: distinguirlas le diría a quien
	// prueba códigos ajenos si va por buen camino.
	ErrCodigoInvalido = errors.New("el código no es válido")
	// ErrCodigoExpirado sí se distingue, porque le pasa a la persona
	// legítima que tardó y necesita saber que tiene que pedir otro, no que
	// se equivocó al tipear.
	ErrCodigoExpirado = errors.New("el código venció")
	// ErrDemasiadosIntentos aparece cuando el código se quemó a fuerza de
	// errarlo. También hay que pedir uno nuevo.
	ErrDemasiadosIntentos = errors.New("se agotaron los intentos para este código")
)

// CodigoRecuperacion es un código de un solo uso, con vencimiento y tope de
// intentos, que habilita a cambiar la contraseña de una cuenta sin conocer
// la anterior. La entidad guarda el HASH, nunca el código: en claro solo lo
// conoce quien recibió el mail (ver migración 009).
type CodigoRecuperacion struct {
	ID         string
	UsuarioID  string
	CodigoHash string
	CreadoEn   time.Time
	ExpiraEn   time.Time
	// UsadoEn es nil mientras el código sirva. Se marca al completar el
	// cambio de contraseña, y también al agotar los intentos: en los dos
	// casos el código dejó de existir para el sistema.
	UsadoEn  *time.Time
	Intentos int
}

// NuevoCodigoRecuperacion arma el código con su vencimiento ya calculado.
func NuevoCodigoRecuperacion(id, usuarioID, codigoHash string, ahora time.Time) (*CodigoRecuperacion, error) {
	if id == "" || usuarioID == "" || codigoHash == "" {
		return nil, fmt.Errorf("código de recuperación incompleto")
	}
	return &CodigoRecuperacion{
		ID:         id,
		UsuarioID:  usuarioID,
		CodigoHash: codigoHash,
		CreadoEn:   ahora,
		ExpiraEn:   ahora.Add(VigenciaCodigoRecuperacion),
	}, nil
}

// Utilizable dice si el código todavía puede validarse: sin usar, sin
// vencer y con intentos disponibles. Es lo que separa "este código no
// sirve" de "el código que mandaste no coincide", que son dos cosas
// distintas para quien está del otro lado.
func (c *CodigoRecuperacion) Utilizable(ahora time.Time) error {
	if c.UsadoEn != nil {
		return ErrCodigoInvalido
	}
	if c.Intentos >= MaxIntentosCodigoRecuperacion {
		return ErrDemasiadosIntentos
	}
	// !After y no Before: un código con ExpiraEn exactamente igual a ahora
	// ya cumplió su ventana.
	if !c.ExpiraEn.After(ahora) {
		return ErrCodigoExpirado
	}
	return nil
}

// RegistrarFallo suma un intento errado y devuelve si con ese el código
// quedó quemado. Cuándo se quema lo decide el dominio y no el servicio,
// para que la regla viva en un solo lugar: el servicio solo persiste lo que
// esto deja.
func (c *CodigoRecuperacion) RegistrarFallo(ahora time.Time) (quemado bool) {
	c.Intentos++
	if c.Intentos >= MaxIntentosCodigoRecuperacion {
		c.UsadoEn = &ahora
		return true
	}
	return false
}

// Usar marca el código como consumido. Falla si ya no era utilizable, así
// que llamarlo dos veces con el mismo código no cambia dos contraseñas.
func (c *CodigoRecuperacion) Usar(ahora time.Time) error {
	if err := c.Utilizable(ahora); err != nil {
		return err
	}
	c.UsadoEn = &ahora
	return nil
}
