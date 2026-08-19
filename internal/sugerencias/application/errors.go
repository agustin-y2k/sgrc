package application

import "errors"

// ErrIDInvalido: lo que llegó no tiene forma de UUID. Se traduce acá y no en
// interfaces/http para que el handler no tenga que saber nada de Postgres
// (mismo criterio que en notification).
var ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")

// ErrNoEsTuya: alguien quiso escribir en una conversación ajena. No es un 404
// a propósito: el hilo existe, y decir lo contrario complicaría el mensaje sin
// esconder nada que el otro no sepa ya.
var ErrNoEsTuya = errors.New("esa conversación no es tuya")
