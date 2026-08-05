package domain

import (
	"errors"
	"testing"
	"time"
)

func TestParseEstado_Validos(t *testing.T) {
	casos := map[string]Estado{"NO_LEIDA": NoLeida, "LEIDA": Leida}
	for entrada, esperado := range casos {
		got, err := ParseEstado(entrada)
		if err != nil || got != esperado {
			t.Errorf("ParseEstado(%q) = %q, %v", entrada, got, err)
		}
	}
}

func TestParseEstado_Invalido(t *testing.T) {
	casos := []string{"", "no_leida", "ARCHIVADA"}
	for _, c := range casos {
		_, err := ParseEstado(c)
		if !errors.Is(err, ErrEstadoInvalido) {
			t.Errorf("ParseEstado(%q): esperaba ErrEstadoInvalido, obtuve %v", c, err)
		}
	}
}

func TestNuevaNotificacion_OK(t *testing.T) {
	n, err := NuevaNotificacion("id1", "usuario1", "Tu reserva fue cancelada", TipoReservaCancelada, Referencias{}, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n.Estado != NoLeida {
		t.Errorf("estado inicial incorrecto: %s", n.Estado)
	}
}

func TestNuevaNotificacion_ConReservaID(t *testing.T) {
	reservaID := "reserva1"
	n, err := NuevaNotificacion("id1", "usuario1", "Tu reserva fue cancelada", TipoReservaCancelada, Referencias{ReservaID: &reservaID}, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n.ReservaID == nil || *n.ReservaID != "reserva1" {
		t.Errorf("ReservaID incorrecto: %v", n.ReservaID)
	}
}

func TestNuevaNotificacion_MensajeVacio_Error(t *testing.T) {
	_, err := NuevaNotificacion("id1", "usuario1", "", TipoGeneral, Referencias{}, time.Now())
	if !errors.Is(err, ErrMensajeVacio) {
		t.Fatalf("esperaba ErrMensajeVacio, obtuve %v", err)
	}
}

func TestMarcarLeida_OK(t *testing.T) {
	n, _ := NuevaNotificacion("id1", "usuario1", "mensaje", TipoGeneral, Referencias{}, time.Now())
	ahora := time.Now()

	err := n.MarcarLeida(ahora)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n.Estado != Leida {
		t.Errorf("estado incorrecto: %s", n.Estado)
	}
	if n.LeidaEn == nil || !n.LeidaEn.Equal(ahora) {
		t.Error("LeidaEn debería quedar seteada")
	}
}

func TestMarcarLeida_DosVeces_Error(t *testing.T) {
	n, _ := NuevaNotificacion("id1", "usuario1", "mensaje", TipoGeneral, Referencias{}, time.Now())
	_ = n.MarcarLeida(time.Now())

	err := n.MarcarLeida(time.Now())

	if !errors.Is(err, ErrYaLeida) {
		t.Fatalf("esperaba ErrYaLeida, obtuve %v", err)
	}
}

// Sin tipo explícito la notificación queda GENERAL, no con la cadena vacía:
// un valor fuera del enum no pasaría el CHECK de la base.
func TestNuevaNotificacion_SinTipo_QuedaGeneral(t *testing.T) {
	n, err := NuevaNotificacion("id1", "usuario1", "mensaje", "", Referencias{}, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n.Tipo != TipoGeneral {
		t.Errorf("tipo = %q, esperaba %q", n.Tipo, TipoGeneral)
	}
}

func TestParseTipo(t *testing.T) {
	for _, valido := range []string{"GENERAL", "DOCENTE_PENDIENTE", "RESERVA_CANCELADA"} {
		if _, err := ParseTipo(valido); err != nil {
			t.Errorf("ParseTipo(%q) falló: %v", valido, err)
		}
	}
	if _, err := ParseTipo("OTRA_COSA"); !errors.Is(err, ErrTipoInvalido) {
		t.Errorf("esperaba ErrTipoInvalido, obtuve %v", err)
	}
}
