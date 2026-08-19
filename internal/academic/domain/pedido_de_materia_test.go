package domain

import (
	"errors"
	"testing"
	"time"
)

var ahoraDePrueba = time.Date(2026, time.August, 18, 10, 0, 0, 0, time.UTC)

func idDeMateria(s string) *string { return &s }

// Las dos formas de pedir son excluyentes: o se elige una materia de la
// lista, o se escribe una que todavía no existe. Con las dos —o con
// ninguna— no hay forma de saber qué quiso decir quien pidió.
func TestNuevoPedido_UnaSolaForma(t *testing.T) {
	casos := map[string]struct {
		materiaID *string
		materia   string
		esperado  error
	}{
		"ninguna de las dos":  {nil, "", ErrPedidoSinMateria},
		"las dos a la vez":    {idDeMateria("m1"), "Robótica", ErrPedidoDobleForma},
		"solo texto en curso": {nil, "   ", ErrPedidoSinMateria},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			_, err := NuevoPedidoDeMateria("p1", "u1", c.materiaID, "5°B", c.materia, "porque sí", ahoraDePrueba)
			if !errors.Is(err, c.esperado) {
				t.Errorf("esperaba %v, obtuve %v", c.esperado, err)
			}
		})
	}
}

// El motivo es lo único con lo que cuenta quien decide antes de ir a
// preguntar, y escribirlo hace pensar dos veces a quien pide de más.
func TestNuevoPedido_ExigeMotivo(t *testing.T) {
	_, err := NuevoPedidoDeMateria("p1", "u1", idDeMateria("m1"), "", "", "   ", ahoraDePrueba)
	if !errors.Is(err, ErrMotivoVacio) {
		t.Errorf("esperaba ErrMotivoVacio, obtuve %v", err)
	}
}

func TestNuevoPedido_DeUnaMateriaQueNoExiste(t *testing.T) {
	p, err := NuevoPedidoDeMateria("p1", "u1", nil, " 5°B ", " Robótica ", " la doy desde mayo ", ahoraDePrueba)
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if !p.EsMateriaNueva() {
		t.Error("un pedido sin materiaID es de una materia que hay que crear")
	}
	// Los espacios de más se limpian: llegan de un formulario, no de una API.
	if p.MateriaSolicitada != "Robótica" || p.CursoSolicitado != "5°B" || p.Motivo != "la doy desde mayo" {
		t.Errorf("no se limpiaron los espacios: %+v", p)
	}
	if p.Estado != PedidoPendiente {
		t.Errorf("un pedido nace pendiente, no %q", p.Estado)
	}
}

// Un "no" sin explicación manda a la persona a preguntar por qué, y esa
// conversación empieza mal: quien pidió ya se expuso contando para qué la
// quería.
func TestRechazar_ExigeExplicacion(t *testing.T) {
	p := pedidoPendiente(t)
	if err := p.Rechazar("admin1", "  ", ahoraDePrueba); !errors.Is(err, ErrRechazoSinMotivo) {
		t.Errorf("esperaba ErrRechazoSinMotivo, obtuve %v", err)
	}
	if p.Estado != PedidoPendiente {
		t.Error("un rechazo inválido no puede dejar el pedido resuelto")
	}
}

// Aprobar no exige respuesta: el resultado ya se ve solo, porque la materia
// aparece en la lista al reservar.
func TestAprobar_SinRespuestaEsValido(t *testing.T) {
	p := pedidoPendiente(t)
	if err := p.Aprobar("admin1", "", ahoraDePrueba); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if p.Estado != PedidoAprobado {
		t.Errorf("esperaba APROBADO, quedó %q", p.Estado)
	}
	if p.ResueltoPor == nil || *p.ResueltoPor != "admin1" || p.ResueltoEn == nil {
		t.Error("quién resolvió y cuándo tienen que quedar registrados")
	}
}

// Dos Admin con la lista abierta al mismo tiempo: el segundo no puede pisar
// la decisión del primero.
func TestResolver_UnaSolaVez(t *testing.T) {
	p := pedidoPendiente(t)
	if err := p.Aprobar("admin1", "va", ahoraDePrueba); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if err := p.Rechazar("admin2", "no va", ahoraDePrueba); !errors.Is(err, ErrPedidoResuelto) {
		t.Errorf("esperaba ErrPedidoResuelto, obtuve %v", err)
	}
	if p.Estado != PedidoAprobado {
		t.Error("la primera decisión es la que vale")
	}
}

func pedidoPendiente(t *testing.T) *PedidoDeMateria {
	t.Helper()
	p, err := NuevoPedidoDeMateria("p1", "u1", idDeMateria("m1"), "", "", "me la asignaron", ahoraDePrueba)
	if err != nil {
		t.Fatalf("armando el pedido de prueba: %v", err)
	}
	return p
}
