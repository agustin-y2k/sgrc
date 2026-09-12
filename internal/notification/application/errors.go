package application

import "errors"

var (
	ErrNotificacionNoEncontrada = errors.New("notificación no encontrada")
	// ErrNoEsTuAviso: alguien quiso cerrar un aviso ajeno. No es un 404 a
	// propósito —la notificación existe— y es una regla del dominio: quién puede
	// marcar qué como leído. Mismo criterio que ErrNoEsTuya en sugerencias.
	ErrNoEsTuAviso = errors.New("no podés marcar como leída una notificación que no es tuya")

	// ErrAvisoSinLeer: borrar algo que todavía no se leyó lo haría desaparecer
	// sin que nadie sepa que existió. Marcarlo leído primero es un clic.
	ErrAvisoSinLeer = errors.New("marcá el aviso como leído antes de borrarlo")

	// ErrIDInvalido: mismo criterio que en el resto del proyecto — un ID
	// sin formato UUID válido se mapea a 400, no a 500.
	ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")
)
