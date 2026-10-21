package application

import (
	"errors"
	"fmt"
)

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

// ErrCascadaDeJornada: falló la cancelación; jornada y reservas se revierten juntas.
var ErrCascadaDeJornada = errors.New("no se pudo confirmar la jornada y la cancelación de las reservas afectadas")

// ErrDemasiadosTramos: la jornada pedida tiene más tramos de los que puede
// tener el horario de una escuela.
var ErrDemasiadosTramos = fmt.Errorf(
	"la jornada no puede tener más de %d tramos: revisá si no se cargaron repetidos", MaxTramosDeJornada)
