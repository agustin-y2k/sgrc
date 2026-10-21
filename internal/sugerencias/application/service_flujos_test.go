package application

import (
	"context"
	"errors"
	"testing"
)

func TestResponder_SinContactoConservaElAvisoInterno(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(map[bool]string{false: "seguimiento", true: "respuesta"}[admin], func(t *testing.T) {
			svc, _, bus := servicioDePrueba()
			hilo, err := svc.Escribir(context.Background(), "u1", "AYUDA", "Consulta", "Texto inicial", "", "")
			if err != nil {
				t.Fatal(err)
			}
			bus.eventos = nil
			svc.usuario = &fakeUsuario{err: errors.New("contacto temporalmente no disponible")}
			autor := "u1"
			if admin {
				autor = "admin1"
			}
			actual, err := svc.Responder(context.Background(), hilo.ID, autor, admin, "Respuesta nueva")
			if err != nil || len(actual.Mensajes) != 2 || len(bus.eventos) != 1 {
				t.Fatalf("mensaje confirmado sin aviso: %+v %v eventos=%v", actual, err, bus.eventos)
			}
			tipo := "sugerencia.seguimiento"
			if admin {
				tipo = "sugerencia.respondida"
			}
			if bus.ultimo().Tipo != tipo {
				t.Fatalf("aviso incorrecto: %+v", bus.ultimo())
			}
		})
	}
}
