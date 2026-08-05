package eventbus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublish_SinSuscriptores_NoPanickea(t *testing.T) {
	bus := NewInMemoryEventBus()

	// No debería panickear ni bloquear aunque nadie esté suscrito.
	bus.Publish(Evento{Tipo: "algo.que.nadie.escucha"})
}

func TestSubscribe_UnHandler_RecibeElEvento(t *testing.T) {
	bus := NewInMemoryEventBus()
	recibido := make(chan Evento, 1)

	bus.Subscribe("reserva.cancelada", func(e Evento) {
		recibido <- e
	})

	bus.Publish(Evento{Tipo: "reserva.cancelada", Payload: "pc-27"})

	select {
	case e := <-recibido:
		if e.Payload != "pc-27" {
			t.Errorf("payload incorrecto: %v", e.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("el handler nunca recibió el evento")
	}
}

func TestSubscribe_VariosHandlersMismoEvento_TodosReciben(t *testing.T) {
	bus := NewInMemoryEventBus()
	var contador int32

	for i := 0; i < 5; i++ {
		bus.Subscribe("docente.baja", func(e Evento) {
			atomic.AddInt32(&contador, 1)
		})
	}

	bus.Publish(Evento{Tipo: "docente.baja"})

	if got := atomic.LoadInt32(&contador); got != 5 {
		t.Fatalf("esperaba 5 handlers ejecutados, obtuve %d", got)
	}
}

func TestPublish_NoLlamaHandlersDeOtroTipo(t *testing.T) {
	bus := NewInMemoryEventBus()
	llamado := false

	bus.Subscribe("tipo.A", func(e Evento) { llamado = true })
	bus.Publish(Evento{Tipo: "tipo.B"})

	if llamado {
		t.Fatal("un handler de tipo.A no debería dispararse por un evento de tipo.B")
	}
}

func TestPublish_HandlerQuePanickea_NoRompeALosDemas(t *testing.T) {
	bus := NewInMemoryEventBus()
	segundoHandlerLlamado := false

	bus.Subscribe("evento.riesgoso", func(e Evento) {
		panic("boom")
	})
	bus.Subscribe("evento.riesgoso", func(e Evento) {
		segundoHandlerLlamado = true
	})

	// Esto NO debe propagar el panic hacia el test.
	bus.Publish(Evento{Tipo: "evento.riesgoso"})

	if !segundoHandlerLlamado {
		t.Fatal("el panic de un handler no debería impedir que el siguiente se ejecute")
	}
}

func TestPublish_HandlerQuePanickea_NoRompeElProceso(t *testing.T) {
	bus := NewInMemoryEventBus()
	bus.Subscribe("evento.riesgoso", func(e Evento) {
		panic("esto no debería tirar abajo el test")
	})

	// Si Publish no recuperara el panic, esta línea nunca se alcanzaría
	// y el test entero fallaría con el proceso muerto.
	bus.Publish(Evento{Tipo: "evento.riesgoso"})
	t.Log("Publish sobrevivió al panic del handler, como se esperaba")
}

func TestSubscribeYPublish_ConcurrenciaSegura(t *testing.T) {
	// go test -race detecta condiciones de carrera acá si el mutex
	// del bus estuviera mal usado.
	bus := NewInMemoryEventBus()
	var wg sync.WaitGroup
	var contador int64

	// Suscriptores concurrentes.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Subscribe("concurrente", func(e Evento) {
				atomic.AddInt64(&contador, 1)
			})
		}()
	}
	wg.Wait()

	// Publishes concurrentes, ya con los 20 suscriptores registrados.
	var wgPub sync.WaitGroup
	for i := 0; i < 10; i++ {
		wgPub.Add(1)
		go func() {
			defer wgPub.Done()
			bus.Publish(Evento{Tipo: "concurrente"})
		}()
	}
	wgPub.Wait()

	// 20 suscriptores x 10 publishes = 200 invocaciones esperadas.
	if got := atomic.LoadInt64(&contador); got != 200 {
		t.Fatalf("esperaba 200 invocaciones, obtuve %d", got)
	}
}

func TestPublish_PayloadLlegaIntacto(t *testing.T) {
	bus := NewInMemoryEventBus()
	type payload struct {
		DocenteID string
		MateriaID string
	}
	recibido := make(chan payload, 1)

	bus.Subscribe("docente.baja.materia-huerfana", func(e Evento) {
		p, ok := e.Payload.(payload)
		if !ok {
			t.Errorf("el payload no llegó con el tipo esperado: %T", e.Payload)
			return
		}
		recibido <- p
	})

	bus.Publish(Evento{
		Tipo:    "docente.baja.materia-huerfana",
		Payload: payload{DocenteID: "d1", MateriaID: "m1"},
	})

	select {
	case p := <-recibido:
		if p.DocenteID != "d1" || p.MateriaID != "m1" {
			t.Errorf("payload con datos incorrectos: %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("nunca llegó el payload")
	}
}
