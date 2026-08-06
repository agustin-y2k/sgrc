package infrastructure

import (
	"crypto/rand"
	"fmt"

	"github.com/google/uuid"

	"github.com/ramiro/sgrc/internal/auth/domain"
)

// NuevoID genera un UUID nuevo para una fila de usuario — se llama antes
// del INSERT, no se deja que Postgres lo genere con DEFAULT, porque
// application.Service necesita el ID ya asignado para el evento que
// publica (docente.registro.pendiente) y para devolverlo en la respuesta
// HTTP sin tener que volver a leer la fila recién creada.
func NuevoID() string {
	return uuid.NewString()
}

// alfabetoPasswordTemporal evita caracteres ambiguos al leerlos en voz
// alta o transcribirlos a mano (0/O, 1/l/I) — la temporal se la va a
// dictar un Admin a un usuario por teléfono o en persona (RF-01.6).
const alfabetoPasswordTemporal = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"

// GenerarPasswordTemporal produce una contraseña aleatoria de 12
// caracteres para el reset asistido por Admin (RF-01.6). 12 caracteres de
// un alfabeto de 54 símbolos son ~68 bits de entropía — de sobra para una
// contraseña que el usuario va a cambiar en su próximo login de todas
// formas.
func GenerarPasswordTemporal() (string, error) {
	const longitud = 12
	alfabetoLen := len(alfabetoPasswordTemporal)

	// Descarte de muestras en vez de "% alfabetoLen": con 256 valores
	// posibles de byte y 54 símbolos, un módulo directo favorecería
	// levemente a los primeros 256%54=40 símbolos del alfabeto. El
	// límite (maxValido) es el mayor múltiplo de alfabetoLen que entra
	// en un byte — cualquier byte por encima se descarta y se vuelve a
	// samplear, así cada símbolo queda exactamente equiprobable.
	maxValido := byte(256 - (256 % alfabetoLen))

	resultado := make([]byte, longitud)
	buf := make([]byte, 1)
	for i := 0; i < longitud; {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generando contraseña temporal: %w", err)
		}
		if buf[0] >= maxValido {
			continue // descartado, no sesga la distribución
		}
		resultado[i] = alfabetoPasswordTemporal[int(buf[0])%alfabetoLen]
		i++
	}
	return string(resultado), nil
}

// GenerarCodigoRecuperacion produce el código de un solo uso que se manda
// por mail para recuperar una contraseña olvidada.
//
// Son dígitos y no el alfabeto de arriba porque el recorrido es distinto:
// la temporal se la dicta un Admin y la persona la usa una vez; el código
// lo lee del celular, cambia de aplicación y lo tipea. Con solo dígitos el
// teclado numérico aparece solo, no hay mayúsculas que confundir, y no
// existe la duda de si esa "l" era una ele o un uno.
//
// Seis dígitos son un millón de combinaciones, poco por sí solo — lo que lo
// hace seguro es el resto: quince minutos de vigencia, cinco intentos, un
// solo código vigente por persona y rate limit en el endpoint (ver
// domain.CodigoRecuperacion y migrations/009).
func GenerarCodigoRecuperacion() (string, error) {
	const digitos = "0123456789"
	longitud := domain.LongitudCodigoRecuperacion

	// 250 es el mayor múltiplo de 10 que entra en un byte: los valores 250
	// a 255 se descartan para que los diez dígitos queden equiprobables.
	// Mismo criterio que GenerarPasswordTemporal.
	const maxValido = byte(250)

	resultado := make([]byte, longitud)
	buf := make([]byte, 1)
	for i := 0; i < longitud; {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generando código de recuperación: %w", err)
		}
		if buf[0] >= maxValido {
			continue
		}
		resultado[i] = digitos[int(buf[0])%len(digitos)]
		i++
	}
	return string(resultado), nil
}
