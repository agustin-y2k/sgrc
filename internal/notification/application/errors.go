package application

import "errors"

var (
	ErrNotificacionNoEncontrada = errors.New("notificación no encontrada")

	// ErrIDInvalido: mismo criterio que en el resto del proyecto — un ID
	// sin formato UUID válido se mapea a 400, no a 500.
	ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")
)
