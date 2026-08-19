package application

import "errors"

// ErrIDInvalido: lo que llegó no tiene forma de UUID. Se traduce acá y no en
// interfaces/http para que el handler no tenga que saber nada de Postgres
// (mismo criterio que en notification).
var ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")
