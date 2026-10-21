package eventbus

import (
	"encoding/json"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/shared/secretos"
)

func TestCodecDurable_RecuperaTipoYRechazaContratosInvalidos(t *testing.T) {
	cifrador, err := secretos.Nuevo("clave-estable-para-eventos-de-prueba-segura")
	if err != nil {
		t.Fatal(err)
	}
	b := &DurableBus{cifrador: cifrador}
	datos, err := json.Marshal(PedidoDeMateriaNuevo{PedidoID: "p1", UsuarioID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	cifrado, err := cifrador.Cifrar(string(datos))
	if err != nil {
		t.Fatal(err)
	}
	e, err := b.decodificar("materia.pedido.nuevo", "eventbus.PedidoDeMateriaNuevo", cifrado)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := e.Payload.(PedidoDeMateriaNuevo); !ok || p.PedidoID != "p1" {
		t.Fatalf("perdió el tipo: %+v", e)
	}
	for _, caso := range []struct{ tipo, clase, payload string }{
		{"materia.pedido.resuelto", "eventbus.PedidoDeMateriaNuevo", cifrado},
		{"materia.pedido.nuevo", "otra.Clase", cifrado},
		{"evento.desconocido", "eventbus.PedidoDeMateriaNuevo", cifrado},
		{"materia.pedido.nuevo", "eventbus.PedidoDeMateriaNuevo", "dato-corrupto"},
	} {
		if _, err := b.decodificar(caso.tipo, caso.clase, caso.payload); err == nil {
			t.Fatalf("aceptó contrato inválido: %+v", caso)
		}
	}
}

func TestCodecDurable_ReposicionConservaElDetalle(t *testing.T) {
	cifrador, err := secretos.Nuevo("clave-estable-para-eventos-de-prueba-segura")
	if err != nil {
		t.Fatal(err)
	}
	payload := ReubicacionesDeUsuario{UsuarioID: "tic", Reservas: []ReservaReubicada{{ReservaID: "r1", EtiquetaAnterior: "nueva", EtiquetaNueva: "antigua"}}}
	datos, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	cifrado, err := cifrador.Cifrar(string(datos))
	if err != nil {
		t.Fatal(err)
	}
	b := &DurableBus{cifrador: cifrador}
	e, err := b.decodificar("reserva.reubicada", "eventbus.ReubicacionesDeUsuario", cifrado)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := e.Payload.(ReubicacionesDeUsuario)
	if !ok || len(p.Reservas) != 1 || p.Reservas[0].EtiquetaNueva != "antigua" {
		t.Fatalf("payload incorrecto: %+v", e)
	}
}
