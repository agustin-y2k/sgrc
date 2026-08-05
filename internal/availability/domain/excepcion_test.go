package domain

import (
	"errors"
	"testing"
	"time"
)

func dur(h int) *time.Duration {
	d := time.Duration(h) * time.Hour
	return &d
}

func TestParseTipoExcepcion_Validos(t *testing.T) {
	casos := map[string]TipoExcepcion{
		"NO_DISPONIBLE": NoDisponible, "HORARIO_MODIFICADO": HorarioModificado,
	}
	for entrada, esperado := range casos {
		got, err := ParseTipoExcepcion(entrada)
		if err != nil || got != esperado {
			t.Errorf("ParseTipoExcepcion(%q) = %q, %v", entrada, got, err)
		}
	}
}

func TestParseTipoExcepcion_Invalido(t *testing.T) {
	casos := []string{"", "no_disponible", "AUSENTE"}
	for _, c := range casos {
		_, err := ParseTipoExcepcion(c)
		if !errors.Is(err, ErrTipoExcepcionInvalido) {
			t.Errorf("ParseTipoExcepcion(%q): esperaba ErrTipoExcepcionInvalido, obtuve %v", c, err)
		}
	}
}

func TestNuevaExcepcion_NoDisponible_OK(t *testing.T) {
	fecha := time.Date(2026, time.March, 9, 15, 0, 0, 0, time.UTC)

	e, err := NuevaExcepcion("id1", "admin1", fecha, NoDisponible, nil, nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if e.Tipo != NoDisponible || e.HoraInicio != nil || e.HoraFin != nil {
		t.Errorf("excepción construida incorrectamente: %+v", e)
	}
	// La fecha se guarda sin la hora (RF-07.4/07.5 son por día, no por instante).
	if e.Fecha.Hour() != 0 {
		t.Errorf("Fecha debería quedar truncada a medianoche, obtuve %v", e.Fecha)
	}
}

func TestNuevaExcepcion_NoDisponible_ConHorario_Error(t *testing.T) {
	fecha := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)

	_, err := NuevaExcepcion("id1", "admin1", fecha, NoDisponible, dur(8), dur(12), nil)

	if !errors.Is(err, ErrExcepcionIncoherente) {
		t.Fatalf("esperaba ErrExcepcionIncoherente, obtuve %v", err)
	}
}

func TestNuevaExcepcion_HorarioModificado_OK(t *testing.T) {
	fecha := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)

	e, err := NuevaExcepcion("id1", "admin1", fecha, HorarioModificado, dur(9), dur(11), nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if e.HoraInicio == nil || *e.HoraInicio != 9*time.Hour {
		t.Errorf("HoraInicio incorrecta: %+v", e.HoraInicio)
	}
	if e.HoraFin == nil || *e.HoraFin != 11*time.Hour {
		t.Errorf("HoraFin incorrecta: %+v", e.HoraFin)
	}
}

func TestNuevaExcepcion_HorarioModificado_SinHorario_Error(t *testing.T) {
	fecha := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)

	casos := []struct {
		nombre      string
		inicio, fin *time.Duration
	}{
		{"sin ninguna de las dos", nil, nil},
		{"solo horaInicio", dur(9), nil},
		{"solo horaFin", nil, dur(11)},
	}
	for _, c := range casos {
		_, err := NuevaExcepcion("id1", "admin1", fecha, HorarioModificado, c.inicio, c.fin, nil)
		if !errors.Is(err, ErrExcepcionIncoherente) {
			t.Errorf("%s: esperaba ErrExcepcionIncoherente, obtuve %v", c.nombre, err)
		}
	}
}

func TestNuevaExcepcion_HorarioModificado_RangoInvalido_Error(t *testing.T) {
	fecha := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)

	_, err := NuevaExcepcion("id1", "admin1", fecha, HorarioModificado, dur(11), dur(9), nil)

	if !errors.Is(err, ErrRangoHorarioInvalido) {
		t.Fatalf("esperaba ErrRangoHorarioInvalido, obtuve %v", err)
	}
}

func TestNuevaExcepcion_ConMotivo(t *testing.T) {
	fecha := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)
	motivo := "llegué tarde"

	e, err := NuevaExcepcion("id1", "admin1", fecha, NoDisponible, nil, nil, &motivo)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if e.Motivo == nil || *e.Motivo != "llegué tarde" {
		t.Errorf("Motivo incorrecto: %+v", e.Motivo)
	}
}

// ── Excepcion.DisponibleAhora ──────────────────────────────────────────

func TestExcepcion_DisponibleAhora_NoDisponible_SiempreFalse(t *testing.T) {
	e, _ := NuevaExcepcion("id1", "admin1", time.Now(), NoDisponible, nil, nil, nil)

	if e.DisponibleAhora(10 * time.Hour) {
		t.Error("una excepción NO_DISPONIBLE nunca debería dar disponible")
	}
}

func TestExcepcion_DisponibleAhora_HorarioModificado_DentroDelRango(t *testing.T) {
	e, _ := NuevaExcepcion("id1", "admin1", time.Now(), HorarioModificado, dur(9), dur(11), nil)

	if !e.DisponibleAhora(10 * time.Hour) {
		t.Error("10:00 debería estar dentro del horario modificado 09:00-11:00")
	}
}

func TestExcepcion_DisponibleAhora_HorarioModificado_LimiteFinExclusive(t *testing.T) {
	e, _ := NuevaExcepcion("id1", "admin1", time.Now(), HorarioModificado, dur(9), dur(11), nil)

	if e.DisponibleAhora(11 * time.Hour) {
		t.Error("la hora de fin exacta no debería estar cubierta (rango exclusive al fin)")
	}
}

func TestExcepcion_DisponibleAhora_HorarioModificado_FueraDelRango(t *testing.T) {
	e, _ := NuevaExcepcion("id1", "admin1", time.Now(), HorarioModificado, dur(9), dur(11), nil)

	if e.DisponibleAhora(8 * time.Hour) {
		t.Error("08:00 no debería estar dentro del horario modificado 09:00-11:00")
	}
}
