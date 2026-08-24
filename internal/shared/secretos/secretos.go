// Package secretos cifra y descifra los datos que el sistema tiene que poder
// LEER de vuelta, a diferencia de una contraseña de usuario, que se hashea.
//
// Hoy lo usa una sola cosa: las contraseñas de las cuentas de cada equipo
// (RF-03.22). A un hash no se le puede preguntar cuál era la contraseña, y ahí
// el punto entero es poder decírsela a quien tiene que abrir la máquina.
//
// Qué protege y qué no, porque la diferencia importa:
//
//   - SÍ protege la copia del archivo. `make backup` produce un volcado SQL en
//     texto plano que se copia a un pendrive, se manda por correo o queda en
//     una carpeta compartida. Sin esto, ese archivo ES la lista de contraseñas
//     de todas las máquinas de la institución.
//   - NO protege de quien tenga la clave. Vive en el mismo `.env` que la
//     contraseña de la base, en el mismo servidor. Alguien que entra al
//     servidor tiene las dos cosas.
//   - NO decide quién ve qué. Eso es control de acceso y vive en la
//     aplicación, no acá.
package secretos

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

var (
	// ErrSinClave es lo que se responde cuando el despliegue no configuró
	// CUENTAS_SECRET. No es un fallo del sistema: es una función que ese
	// despliegue no habilitó, igual que el correo o el ingreso con Google.
	ErrSinClave = errors.New("este despliegue no tiene configurada la clave para guardar contraseñas (CUENTAS_SECRET en el .env)")

	// ErrNoSePudoDescifrar cubre los tres casos que son el mismo problema
	// desde afuera: la clave cambió, el texto se corrompió, o alguien lo
	// alteró. No se distinguen a propósito — un mensaje que dijera "la clave
	// es incorrecta" versus "el dato está alterado" le contaría a un atacante
	// cuál de las dos cosas tiene mal.
	ErrNoSePudoDescifrar = errors.New("no se pudo descifrar el dato guardado")
)

// Cifrador guarda y recupera secretos con AES-256-GCM. GCM y no CBC porque
// además de cifrar autentica: un valor alterado en la base no descifra a
// basura silenciosa, falla.
type Cifrador struct {
	aead cipher.AEAD
}

// Nuevo deriva la clave de cifrado del secreto del .env. Devuelve nil sin
// error cuando el secreto está vacío: el despliegue que no lo configura sigue
// funcionando, solo que sin poder guardar contraseñas. Quien lo use tiene que
// tratar el nil como "esta función no está disponible" — Cifrar y Descifrar
// sobre un *Cifrador nil responden ErrSinClave en vez de romper.
func Nuevo(secreto string) (*Cifrador, error) {
	if secreto == "" {
		return nil, nil
	}
	// SHA-256 del secreto y no el secreto crudo: AES-256 necesita exactamente
	// 32 bytes, y pedirle a quien despliega que escriba 32 bytes exactos en el
	// .env es una restricción que no aporta nada y que se equivoca fácil.
	clave := sha256.Sum256([]byte(secreto))
	bloque, err := aes.NewCipher(clave[:])
	if err != nil {
		return nil, fmt.Errorf("armando el cifrador: %w", err)
	}
	aead, err := cipher.NewGCM(bloque)
	if err != nil {
		return nil, fmt.Errorf("armando GCM: %w", err)
	}
	return &Cifrador{aead: aead}, nil
}

// Disponible dice si este despliegue puede guardar secretos. Es lo que
// consulta la capa de aplicación para responder "esto no está configurado"
// antes de intentar nada.
func (c *Cifrador) Disponible() bool { return c != nil }

// Cifrar devuelve el texto listo para guardar en una columna de texto: el
// nonce y el resultado juntos, en base64.
//
// El nonce es nuevo en cada llamada, y por eso cifrar dos veces la misma
// contraseña da dos textos distintos. Es deliberado: con un nonce fijo, dos
// filas iguales se verían iguales en la base, y eso le diría a quien mire el
// volcado que dos máquinas comparten la contraseña sin necesidad de
// descifrar nada.
func (c *Cifrador) Cifrar(texto string) (string, error) {
	if !c.Disponible() {
		return "", ErrSinClave
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generando el nonce: %w", err)
	}
	// Seal agrega el resultado atrás del nonce, así queda todo en un solo
	// valor y no hay que guardar dos columnas que podrían separarse.
	sellado := c.aead.Seal(nonce, nonce, []byte(texto), nil)
	return base64.StdEncoding.EncodeToString(sellado), nil
}

// Descifrar recupera el texto original de lo que devolvió Cifrar.
func (c *Cifrador) Descifrar(guardado string) (string, error) {
	if !c.Disponible() {
		return "", ErrSinClave
	}
	crudo, err := base64.StdEncoding.DecodeString(guardado)
	if err != nil {
		return "", ErrNoSePudoDescifrar
	}
	n := c.aead.NonceSize()
	if len(crudo) < n {
		return "", ErrNoSePudoDescifrar
	}
	claro, err := c.aead.Open(nil, crudo[:n], crudo[n:], nil)
	if err != nil {
		return "", ErrNoSePudoDescifrar
	}
	return string(claro), nil
}
