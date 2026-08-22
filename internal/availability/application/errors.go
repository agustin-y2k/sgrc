package application

import "errors"

var (
	ErrBloqueNoEncontrado = errors.New("bloque de horario no encontrado")

	// ErrExcepcionNoEncontrada: uso interno de infrastructure/ — no tener una
	// excepción cargada para una fecha dada es el caso normal (RF-07.4 es
	// opcional), así que Repo.BuscarExcepcionDeFecha lo traduce a (nil, nil)
	// antes de devolverle algo a application/.
	ErrExcepcionNoEncontrada = errors.New("excepción no encontrada")

	// ErrIDInvalido: mismo criterio que en el resto del proyecto — un ID
	// sin formato UUID válido se mapea a 400, no a 500.
	ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")
)

// ErrCascadaDeJornada: la jornada nueva se guardó pero no se pudieron
// cancelar las reservas que quedaron afuera. Es un estado incompleto y hay
// que decirlo: la jornada ya rige y esas clases siguen en pie.
var ErrCascadaDeJornada = errors.New("la jornada se guardó, pero no se pudieron cancelar las reservas que quedaron afuera")
