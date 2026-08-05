// Package eventbus implementa un pub/sub in-process (ver docs/06-arquitectura.md §4).
//
// Reemplaza a un message broker externo (NATS, Kafka): en un monolito no hace
// falta desacoplar procesos que corren en el mismo binario. Si en el futuro el
// proyecto se separa en microservicios, solo se reemplaza InMemoryEventBus por
// una implementación que hable con un broker real — el resto del código
// (quién publica, quién se suscribe) no cambia porque depende de la interfaz
// EventBus, no de esta implementación concreta.
package eventbus

import (
	"log"
	"sync"
)

// Evento es el payload genérico que viaja por el bus.
type Evento struct {
	Tipo    string
	Payload any
}

// EventBus es el contrato que usan los demás paquetes. Nunca importar
// InMemoryEventBus directamente desde otro paquete — inyectar esta interfaz.
type EventBus interface {
	Publish(evento Evento)
	Subscribe(tipo string, handler func(Evento))
}

// InMemoryEventBus es la única implementación mientras el sistema sea un
// monolito. La entrega es síncrona y en memoria — sin persistencia de
// mensajes ni garantías at-least-once, que no hacen falta con un solo
// proceso (si el proceso muere, todo se reinicia junto).
type InMemoryEventBus struct {
	mu       sync.RWMutex
	handlers map[string][]func(Evento)
}

// NewInMemoryEventBus crea un bus vacío, listo para que los paquetes se
// suscriban en el arranque de main.go.
func NewInMemoryEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{
		handlers: make(map[string][]func(Evento)),
	}
}

// Publish notifica a todos los handlers suscritos a ese tipo de evento.
// Corre en la goroutine del llamador; si algún handler necesita no bloquear
// la respuesta HTTP (ej. notificaciones), debe lanzar su propia goroutine
// internamente.
//
// Un panic dentro de un handler NUNCA debe tirar abajo el resto de los
// handlers suscritos al mismo evento, ni al proceso completo — se recupera,
// se loguea, y Publish sigue con el siguiente handler. Sin esto, un bug en
// (por ejemplo) el handler de reportes podría impedir que el handler de
// notificaciones se entere de una cancelación de reserva.
func (b *InMemoryEventBus) Publish(evento Evento) {
	b.mu.RLock()
	handlers := b.handlers[evento.Tipo]
	b.mu.RUnlock()

	for _, h := range handlers {
		invocarConRecover(evento, h)
	}
}

func invocarConRecover(evento Evento, handler func(Evento)) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("eventbus: handler para %q hizo panic (recuperado): %v", evento.Tipo, r)
		}
	}()
	handler(evento)
}

// Subscribe registra un handler para un tipo de evento. Se llama una sola
// vez por handler, típicamente en main.go durante el wiring de paquetes.
func (b *InMemoryEventBus) Subscribe(tipo string, handler func(Evento)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[tipo] = append(b.handlers[tipo], handler)
}
