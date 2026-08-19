// Package security centraliza el hasheo y verificación de contraseñas —
// argon2id, un solo lugar, para que el seed del primer Admin (cmd/) y el
// login/registro normal (internal/auth) nunca puedan divergir en cómo
// hashean o verifican. Antes de este paquete, cmd/seed_admin.go tenía su
// propia copia — se movió acá apenas internal/auth empezó a necesitar lo
// mismo (ver docs/09-seguridad-rbac.md §1).
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parámetros de argon2id. Cambiarlos invalida la verificación de hashes ya
// guardados en la base con los valores anteriores — si algún día hace
// falta ajustar estos números, es una migración de datos, no un cambio de
// código libre.
const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // 64 MB
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// maxHashesEnParalelo es cuántos argon2id pueden estar corriendo a la vez en
// todo el proceso. Los demás esperan su turno.
//
// El número sale de una cuenta, no del gusto: cada hasheo reserva
// argonMemory (64 MB) mientras dura, y el contenedor tiene un tope de
// memoria (mem_limit en docker-compose.yml). Sin este límite, N logins
// simultáneos piden 64·N MB de golpe y el kernel mata el proceso: cuatro
// personas entrando al mismo tiempo alcanzaban para tumbar la API con los
// 256 MB que había, y el contenedor no vuelve solo.
//
// Tres son 192 MB en el peor caso, que entra con holgura en el mem_limit de
// 1g del compose (y bajo el GOMEMLIMIT de 800MiB que se le declara ahí) y
// deja lugar al resto del proceso. Si algún día se sube o baja ese tope,
// este número va con él.
//
// Que ahora sobre memoria no es motivo para subirlo: cada hasheo usa
// argonThreads (4) de CPU, así que más en paralelo no da más logins por
// segundo en un servidor chico — solo hace que compitan entre ellos. Tres
// alcanzan de sobra para una escuela: es la cola la que absorbe el pico de
// las 7:30, y se vacía en décimas de segundo.
//
// Encolar y no rechazar: un login que tarda un segundo de más es una
// molestia; uno que devuelve error porque otros tres estaban entrando al
// mismo tiempo es un sistema que se rompe justo cuando lo usan. El rate
// limit de /login (RateLimit(5, time.Minute) en auth/interfaces/http) es el
// que pone el techo real a cuánta cola se puede formar.
const maxHashesEnParalelo = 3

// turnoDeHasheo lleva los turnos. Un canal con buffer y no un sync.Mutex
// porque lo que hace falta no es exclusión, sino dejar pasar hasta tres.
var turnoDeHasheo = make(chan struct{}, maxHashesEnParalelo)

// tomarTurno bloquea hasta que haya lugar y devuelve la función que lo
// libera, para usarla como `defer tomarTurno()()`.
//
// No recibe context a propósito: cambiaría la firma de HashPassword y
// VerifyPassword, que viajan como valores hasta internal/auth (ver
// cmd/main.go), y la espera acá se mide en el tiempo de uno o dos hasheos
// —décimas de segundo—, no en algo que valga la pena cancelar.
func tomarTurno() func() {
	turnoDeHasheo <- struct{}{}
	return func() { <-turnoDeHasheo }
}

// HashPassword devuelve el hash en formato PHC estándar
// ($argon2id$v=19$m=...,t=...,p=...$salt$hash).
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generando salt: %w", err)
	}

	defer tomarTurno()()
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64Salt, b64Hash), nil
}

// VerifyPassword compara un password en texto plano contra un hash en
// formato PHC. El parseo es manual con strings.Split, no fmt.Sscanf —
// Sscanf con %s no delimita bien contra literales como '$' (solo contra
// espacios en blanco), así que hubiera partido mal la cadena.
func VerifyPassword(password, encoded string) (bool, error) {
	partes := strings.Split(encoded, "$")
	// esperado: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false, fmt.Errorf("formato de hash inválido")
	}

	var version int
	if _, err := fmt.Sscanf(partes[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("versión de hash inválida: %w", err)
	}

	var memory, iterTime uint32
	var threads uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &memory, &iterTime, &threads); err != nil {
		return false, fmt.Errorf("parámetros de hash inválidos: %w", err)
	}
	// Sanity check: si estos vinieran en 0 (hash corrupto o editado a
	// mano en la base), argon2.IDKey podría comportarse de forma
	// indefinida. Mejor un error claro que dejarlo pasar.
	if memory == 0 || iterTime == 0 || threads == 0 {
		return false, fmt.Errorf("parámetros de hash inválidos: memory=%d, time=%d, threads=%d", memory, iterTime, threads)
	}

	salt, err := base64.RawStdEncoding.DecodeString(partes[4])
	if err != nil {
		return false, fmt.Errorf("salt inválido: %w", err)
	}
	esperado, err := base64.RawStdEncoding.DecodeString(partes[5])
	if err != nil {
		return false, fmt.Errorf("hash inválido: %w", err)
	}
	if len(esperado) == 0 {
		return false, fmt.Errorf("hash vacío")
	}

	// El turno se toma acá y no al entrar: todo lo de arriba es parseo
	// barato, y ocupar un lugar de la cola mientras se valida el formato de
	// un hash haría esperar a un login legítimo por uno corrupto.
	defer tomarTurno()()
	obtenido := argon2.IDKey([]byte(password), salt, iterTime, memory, threads, uint32(len(esperado)))
	return subtle.ConstantTimeCompare(obtenido, esperado) == 1, nil
}
