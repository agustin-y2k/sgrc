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

// HashPassword devuelve el hash en formato PHC estándar
// ($argon2id$v=19$m=...,t=...,p=...$salt$hash).
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generando salt: %w", err)
	}

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

	obtenido := argon2.IDKey([]byte(password), salt, iterTime, memory, threads, uint32(len(esperado)))
	return subtle.ConstantTimeCompare(obtenido, esperado) == 1, nil
}
