package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func instante(h, m int) time.Time {
	return time.Date(2026, time.August, 7, h, m, 0, 0, time.UTC)
}

func entregaMinima() DatosDeEntrega {
	return DatosDeEntrega{EquipoID: "equipo-1", Nombre: "Ana Pérez", EntregadoPor: "admin-1"}
}

func TestNuevoPrestamo_Espontaneo(t *testing.T) {
	// El caso del trámite: sin reserva, sin cuenta, sin hora de devolución.
	// Los tres campos opcionales a la vez, que es como llega en la práctica.
	p, err := NuevoPrestamo("pr-1", entregaMinima(), instante(9, 0))

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if p.ReservaID != nil {
		t.Error("un préstamo espontáneo no tiene reserva detrás")
	}
	if p.EntregadoAUsuarioID != nil {
		t.Error("quien pide una PC para un trámite puede no tener cuenta")
	}
	if p.DevolucionEstimada != nil {
		t.Error("sin hora pactada, DevolucionEstimada queda vacía")
	}
	if !p.EstaAbierto() {
		t.Error("un préstamo recién creado está abierto")
	}
	if p.EntregadoPor == nil || *p.EntregadoPor != "admin-1" {
		t.Errorf("no quedó registrado quién la entregó: %v", p.EntregadoPor)
	}
}

func TestNuevoPrestamo_ContraReserva(t *testing.T) {
	reservaID := "res-1"
	usuarioID := "usr-1"
	vence := instante(10, 0)
	d := entregaMinima()
	d.ReservaID = &reservaID
	d.UsuarioID = &usuarioID
	d.DevolucionEstimada = &vence

	p, err := NuevoPrestamo("pr-1", d, instante(9, 0))

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if p.ReservaID == nil || *p.ReservaID != reservaID {
		t.Errorf("reservaID = %v, esperaba %q", p.ReservaID, reservaID)
	}
	// El nombre se guarda IGUAL aunque haya cuenta: es un snapshot, para que
	// el registro siga diciendo quién se la llevó si la cuenta se elimina.
	if p.EntregadoANombre != "Ana Pérez" {
		t.Errorf("nombre = %q, esperaba que se guardara aunque haya usuario", p.EntregadoANombre)
	}
}

func TestNuevoPrestamo_NormalizaElNombre(t *testing.T) {
	d := entregaMinima()
	d.Nombre = "  Ana   Pérez  "

	p, err := NuevoPrestamo("pr-1", d, instante(9, 0))

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	// Bordes recortados y espacios internos colapsados: se tipea a las
	// apuradas en el mostrador. La caja no se toca, es un nombre propio.
	if p.EntregadoANombre != "Ana Pérez" {
		t.Errorf("nombre = %q, esperaba %q", p.EntregadoANombre, "Ana Pérez")
	}
}

func TestNuevoPrestamo_NombreInvalido(t *testing.T) {
	casos := []struct {
		nombre   string
		valor    string
		esperado error
	}{
		{"vacío", "", ErrNombreDestinatarioVacio},
		{"puros espacios", "   ", ErrNombreDestinatarioVacio},
		{"demasiado largo", strings.Repeat("a", MaxLargoNombreDestinatario+1), ErrNombreDestinatarioLargo},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			d := entregaMinima()
			d.Nombre = c.valor
			if _, err := NuevoPrestamo("pr-1", d, instante(9, 0)); !errors.Is(err, c.esperado) {
				t.Errorf("esperaba %v, obtuve %v", c.esperado, err)
			}
		})
	}
}

// TestNuevoPrestamo_DevolucionYaVencidaEsValida: el docente que llegó tarde y
// se las llevó igual.
func TestNuevoPrestamo_DevolucionYaVencidaEsValida(t *testing.T) {
	vence := instante(10, 0)
	d := entregaMinima()
	d.DevolucionEstimada = &vence

	p, err := NuevoPrestamo("pr-1", d, instante(10, 15))

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !p.Demorado(instante(10, 15)) {
		t.Error("nace demorada, y está bien: se entregó pasada la hora de devolución")
	}
}

func TestDevolver(t *testing.T) {
	p, err := NuevoPrestamo("pr-1", entregaMinima(), instante(9, 0))
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if err := p.Devolver("admin-2", "  volvió sin el cargador  ", instante(10, 30)); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if p.EstaAbierto() {
		t.Error("tras devolver, el préstamo no está abierto")
	}
	if !p.DevueltoEn.Equal(instante(10, 30)) {
		t.Errorf("devueltoEn = %v, esperaba las 10:30", *p.DevueltoEn)
	}
	if p.RecibidoPor == nil || *p.RecibidoPor != "admin-2" {
		t.Errorf("no quedó registrado quién la recibió: %v", p.RecibidoPor)
	}
	if p.Observaciones != "volvió sin el cargador" {
		t.Errorf("observaciones = %q, esperaba el texto recortado", p.Observaciones)
	}
}

// TestDevolver_DosVeces: pasa de verdad —dos Admin en el mostrador, o un
// doble clic— y tiene que distinguirse de "este equipo nunca salió".
func TestDevolver_DosVeces(t *testing.T) {
	p, err := NuevoPrestamo("pr-1", entregaMinima(), instante(9, 0))
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := p.Devolver("admin-2", "", instante(10, 30)); err != nil {
		t.Fatalf("la primera no debería fallar: %v", err)
	}

	err = p.Devolver("admin-3", "", instante(10, 35))

	if !errors.Is(err, ErrPrestamoYaDevuelto) {
		t.Fatalf("esperaba ErrPrestamoYaDevuelto, obtuve %v", err)
	}
	// Y no pisa los datos de la primera devolución.
	if !p.DevueltoEn.Equal(instante(10, 30)) || *p.RecibidoPor != "admin-2" {
		t.Errorf("la segunda devolución no debería haber tocado nada: %v, %v", *p.DevueltoEn, *p.RecibidoPor)
	}
}

func TestDemorado(t *testing.T) {
	vence := instante(10, 0)

	casos := []struct {
		nombre    string
		estimada  *time.Time
		devuelta  bool
		ahora     time.Time
		esperado  bool
		minEspera int
	}{
		{"antes de la hora", &vence, false, instante(9, 30), false, 0},
		{"justo a la hora", &vence, false, instante(10, 0), false, 0},
		{"quince minutos tarde", &vence, false, instante(10, 15), true, 15},
		{"ya devuelta", &vence, true, instante(10, 15), false, 0},
		// Un "vengo en un rato" no se puede reclamar: no hay contra qué comparar, y
		// tratarlo como vencido convertiría cada préstamo suelto en un aviso.
		{"sin hora pactada", nil, false, instante(23, 0), false, 0},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			d := entregaMinima()
			d.DevolucionEstimada = c.estimada
			p, err := NuevoPrestamo("pr-1", d, instante(9, 0))
			if err != nil {
				t.Fatalf("no debería fallar: %v", err)
			}
			if c.devuelta {
				if err := p.Devolver("admin-2", "", instante(9, 55)); err != nil {
					t.Fatalf("no debería fallar: %v", err)
				}
			}

			if got := p.Demorado(c.ahora); got != c.esperado {
				t.Errorf("Demorado = %v, esperaba %v", got, c.esperado)
			}
			if got := p.MinutosDeDemora(c.ahora); got != c.minEspera {
				t.Errorf("MinutosDeDemora = %d, esperaba %d", got, c.minEspera)
			}
		})
	}
}
