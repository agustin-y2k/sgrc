package eventbus

import (
	"context"

	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
)

type Persistente interface {
	EventBus
	Persistir(context.Context, Evento) error
	EnOperacion(context.Context, func(context.Context) error) error
	SuscribirDurable(string, func(context.Context, Evento) error)
}

// Emitir incluye el evento en la escritura actual. Un error de persistencia
// rechaza el commit exterior aunque el publicador no devuelva un resultado.
func Emitir(ctx context.Context, bus EventBus, e Evento) {
	if bus == nil {
		return
	}
	if durable, ok := bus.(Persistente); ok {
		if err := durable.Persistir(ctx, e); err != nil {
			if !pgtransaccion.Rechazar(ctx, err) {
				panic(err)
			}
		}
		return
	}
	pgtransaccion.AlConfirmar(ctx, func(context.Context) { bus.Publish(e) })
}
func AlConfirmar(ctx context.Context, bus EventBus, fn func(context.Context)) {
	if _, ok := bus.(Persistente); ok {
		fn(ctx)
		return
	}
	pgtransaccion.AlConfirmar(ctx, fn)
}
func EnOperacion(ctx context.Context, bus EventBus, fn func(context.Context) error) error {
	if durable, ok := bus.(Persistente); ok {
		return durable.EnOperacion(ctx, fn)
	}
	return fn(ctx)
}
