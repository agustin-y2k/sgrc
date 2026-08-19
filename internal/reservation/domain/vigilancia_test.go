package domain

import (
	"testing"
	"time"
)

// La clase de prueba: el 10 de agosto de 2026, de 8 a 9.
var (
	diaDeClase = time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	deOchoANue = [2]time.Duration{8 * time.Hour, 9 * time.Hour}
)

func alas(hora, minuto int) time.Time {
	return time.Date(2026, time.August, 10, hora, minuto, 0, 0, time.UTC)
}

func TestCorrespondeRecordar(t *testing.T) {
	casos := []struct {
		nombre   string
		ahora    time.Time
		esperado bool
	}{
		{"dos horas antes", alas(6, 0), false},
		{"una hora y un minuto antes", alas(6, 59), false},
		{"justo una hora antes", alas(7, 0), true},
		{"media hora antes", alas(7, 30), true},
		// Si el proceso estuvo caído, el recordatorio sale tarde en vez de
		// perderse: a las 8:10 todavía sirve saber que la reserva se libera a las
		// 8:40.
		{"ya empezó pero no terminó", alas(8, 10), true},
		{"justo cuando termina", alas(9, 0), false},
		{"después de terminada", alas(10, 0), false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := CorrespondeRecordar(diaDeClase, deOchoANue[0], deOchoANue[1], AntelacionDelRecordatorio, c.ahora)
			if got != c.esperado {
				t.Errorf("CorrespondeRecordar = %v, esperaba %v", got, c.esperado)
			}
		})
	}
}

func TestCorrespondeRecordar_OtroDiaNoCuenta(t *testing.T) {
	// Las 7:00 del día siguiente no son "una hora antes" de la clase de ayer.
	ayer := time.Date(2026, time.August, 9, 0, 0, 0, 0, time.UTC)

	if CorrespondeRecordar(ayer, deOchoANue[0], deOchoANue[1], AntelacionDelRecordatorio, alas(7, 0)) {
		t.Error("una clase de ayer no se recuerda hoy")
	}
}

func TestCorrespondeLiberar(t *testing.T) {
	casos := []struct {
		nombre   string
		ahora    time.Time
		esperado bool
	}{
		{"antes de empezar", alas(7, 30), false},
		{"recién empezada", alas(8, 5), false},
		{"a los 39 minutos", alas(8, 39), false},
		{"a los 40 minutos exactos", alas(8, 40), true},
		{"a los 50 minutos", alas(8, 50), true},
		// La que ya terminó no se libera: el job de vencimiento la va a
		// marcar FINALIZADA, y liberar algo que nadie puede usar no sirve.
		{"ya terminada", alas(9, 10), false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := CorrespondeLiberar(diaDeClase, deOchoANue[0], deOchoANue[1], GraciaDeRetiroPorDefecto, c.ahora)
			if got != c.esperado {
				t.Errorf("CorrespondeLiberar = %v, esperaba %v", got, c.esperado)
			}
		})
	}
}

// TestCorrespondeLiberar_ClaseMasCortaQueLaGracia: con media hora de clase y
// cuarenta minutos de gracia, no se libera nunca.
func TestCorrespondeLiberar_ClaseMasCortaQueLaGracia(t *testing.T) {
	fin := 8*time.Hour + 30*time.Minute

	for _, ahora := range []time.Time{alas(8, 25), alas(8, 40), alas(9, 0)} {
		if CorrespondeLiberar(diaDeClase, 8*time.Hour, fin, GraciaDeRetiroPorDefecto, ahora) {
			t.Errorf("a las %s: una clase más corta que la gracia no se libera", ahora.Format("15:04"))
		}
	}
}

func TestCorrespondeLiberar_GraciaConfigurable(t *testing.T) {
	quince := 15 * time.Minute

	if CorrespondeLiberar(diaDeClase, deOchoANue[0], deOchoANue[1], quince, alas(8, 14)) {
		t.Error("con 15 minutos de gracia, a los 14 todavía no")
	}
	if !CorrespondeLiberar(diaDeClase, deOchoANue[0], deOchoANue[1], quince, alas(8, 15)) {
		t.Error("con 15 minutos de gracia, a los 15 sí")
	}
}

// TestCorrespondeAvisarEquipoNoDisponible_MaxDeteccionOInicioMenosUnaHora es
// la regla que se definió para el docente siguiente.
func TestCorrespondeAvisarEquipoNoDisponible_MaxDeteccionOInicioMenosUnaHora(t *testing.T) {
	casos := []struct {
		nombre   string
		ahora    time.Time
		esperado bool
	}{
		// Se detectó la demora a las 9:15 y su reserva es a las 11: el aviso
		// espera hasta las 10.
		{"falta más de una hora", alas(9, 15), false},
		{"justo una hora antes", alas(7, 0), true},
		// Reserva contigua o a menos de una hora: sale al detectarla.
		{"media hora antes", alas(7, 30), true},
		{"ya empezó", alas(8, 15), true},
		{"ya terminó", alas(9, 30), false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := CorrespondeAvisarEquipoNoDisponible(diaDeClase, deOchoANue[0], deOchoANue[1],
				AntelacionDelRecordatorio, c.ahora)
			if got != c.esperado {
				t.Errorf("CorrespondeAvisarEquipoNoDisponible = %v, esperaba %v", got, c.esperado)
			}
		})
	}
}

// ── Estados ─────────────────────────────────────────────────────────────

func TestEstadoReserva_NoRetirada(t *testing.T) {
	if _, err := ParseEstadoReserva("NO_RETIRADA"); err != nil {
		t.Errorf("NO_RETIRADA tiene que parsear: %v", err)
	}
	if !ReservaConfirmada.PuedeTransicionarA(ReservaNoRetirada) {
		t.Error("una confirmada tiene que poder liberarse")
	}
	// Liberar no es prohibir, pero tampoco se deshace: si el docente aparece a
	// los cincuenta minutos y las máquinas siguen ahí, se le entregan como
	// préstamo — la reserva no revive.
	if ReservaNoRetirada.PuedeTransicionarA(ReservaConfirmada) {
		t.Error("una liberada no vuelve a confirmarse")
	}
	for _, destino := range []EstadoReserva{ReservaCancelada, ReservaFinalizada} {
		if ReservaNoRetirada.PuedeTransicionarA(destino) {
			t.Errorf("NO_RETIRADA es terminal, no debería poder ir a %s", destino)
		}
	}
}

func TestEstadoReservaGrupo_NoRetirada(t *testing.T) {
	if _, err := ParseEstadoReservaGrupo("NO_RETIRADA"); err != nil {
		t.Errorf("NO_RETIRADA tiene que parsear: %v", err)
	}
	if !GrupoConfirmada.PuedeTransicionarA(GrupoNoRetirado) {
		t.Error("un grupo confirmado tiene que poder quedar no retirado")
	}
	if GrupoNoRetirado.PuedeTransicionarA(GrupoConfirmada) {
		t.Error("NO_RETIRADA es terminal también a nivel grupo")
	}
}
