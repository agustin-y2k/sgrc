package application

import (
	"context"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// Repo es el único contrato que este paquete necesita de infrastructure/.
type Repo interface {
	CrearCarro(ctx context.Context, c *domain.Carro) error
	BuscarCarroPorID(ctx context.Context, id string) (*domain.Carro, error)
	GuardarCarro(ctx context.Context, c *domain.Carro) error
	ListarCarros(ctx context.Context) ([]*domain.Carro, error)

	CrearPC(ctx context.Context, pc *domain.PC) error
	BuscarPCPorID(ctx context.Context, id string) (*domain.PC, error)
	GuardarPC(ctx context.Context, pc *domain.PC) error
	ListarPCsPorCarro(ctx context.Context, carroID string) ([]*domain.PC, error)

	CrearIncidencia(ctx context.Context, i *domain.Incidencia) error
	BuscarIncidenciaPorID(ctx context.Context, id string) (*domain.Incidencia, error)
	GuardarIncidencia(ctx context.Context, i *domain.Incidencia) error
	ListarIncidenciasPorPC(ctx context.Context, pcID string) ([]*domain.Incidencia, error)
}

// ValidadorReservas es el puerto hacia reservation — todavía no existe ese
// paquete, así que se usa un stub que no cancela nada (ver
// infrastructure/validador_reservas_stub.go). Es la respuesta CORRECTA
// hoy — no puede haber ninguna reserva que cancelar si el paquete que las
// crea no está implementado. Mismo criterio que ValidadorReservas en
// academic.
//
// A diferencia del de academic (que solo consulta "¿hay reservas?"), acá
// la interfaz representa una ACCIÓN — cancelar en cascada y notificar —
// porque RF-03.8/03.9 no es una simple validación antes de bloquear una
// operación, es un efecto que debe dispararse cuando una PC cambia de
// estado o se da de baja.
type ValidadorReservas interface {
	CancelarReservasFuturasDePC(ctx context.Context, pcID string, motivo string) (canceladas int, docentesNotificados int, err error)

	// TieneReservasFuturas es la única lectura de este puerto, y existe por
	// la misma razón que TieneReservasDeCiclo en academic: la cascada de
	// RF-03.8/03.9 no puede ser atómica con el guardado de la PC (cruza a
	// reservation, que abre su propia transacción), así que lo que se puede
	// garantizar no es que nunca falle a la mitad, sino que se pueda
	// terminar. Esto es lo que distingue "esta PC ya se dio de baja" de
	// "esta PC se dio de baja pero la cascada quedó pendiente".
	TieneReservasFuturas(ctx context.Context, pcID string) (bool, error)
}

type IDGenerator func() string
