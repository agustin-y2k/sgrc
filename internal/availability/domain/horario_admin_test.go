package domain

import (
	"errors"
	"testing"
	"time"
)

func TestParseDiaSemana_Validos(t *testing.T) {
	casos := map[string]DiaSemana{
		"LUNES": Lunes, "MARTES": Martes, "MIERCOLES": Miercoles,
		"JUEVES": Jueves, "VIERNES": Viernes,
	}
	for entrada, esperado := range casos {
		got, err := ParseDiaSemana(entrada)
		if err != nil || got != esperado {
			t.Errorf("ParseDiaSemana(%q) = %q, %v", entrada, got, err)
		}
	}
}

func TestParseDiaSemana_Invalido(t *testing.T) {
	casos := []string{"", "lunes", "DOMINGO"} // DOMINGO no es válido acá tampoco
	for _, c := range casos {
		_, err := ParseDiaSemana(c)
		if !errors.Is(err, ErrDiaSemanaInvalido) {
			t.Errorf("ParseDiaSemana(%q): esperaba ErrDiaSemanaInvalido, obtuve %v", c, err)
		}
	}
}

func TestNuevoBloqueHorario_OK(t *testing.T) {
	b, err := NuevoBloqueHorario("id1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if b.DiaSemana != Lunes || b.HoraInicio != 8*time.Hour || b.HoraFin != 12*time.Hour {
		t.Errorf("bloque construido incorrectamente: %+v", b)
	}
}

func TestNuevoBloqueHorario_RangoInvalido_Error(t *testing.T) {
	casos := []struct {
		nombre      string
		inicio, fin time.Duration
	}{
		{"fin antes que inicio", 12 * time.Hour, 8 * time.Hour},
		{"fin igual a inicio", 8 * time.Hour, 8 * time.Hour},
	}
	for _, c := range casos {
		_, err := NuevoBloqueHorario("id1", "admin1", Lunes, c.inicio, c.fin)
		if !errors.Is(err, ErrRangoHorarioInvalido) {
			t.Errorf("%s: esperaba ErrRangoHorarioInvalido, obtuve %v", c.nombre, err)
		}
	}
}

// ── Cubre — rango [HoraInicio, HoraFin), límites exactos ──────────────

func TestBloqueHorario_Cubre_DentroDelRango(t *testing.T) {
	b, _ := NuevoBloqueHorario("id1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)

	if !b.Cubre(Lunes, 10*time.Hour) {
		t.Error("10:00 debería estar cubierta por el bloque 08:00-12:00")
	}
}

func TestBloqueHorario_Cubre_LimiteInicioInclusive(t *testing.T) {
	b, _ := NuevoBloqueHorario("id1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)

	if !b.Cubre(Lunes, 8*time.Hour) {
		t.Error("la hora de inicio exacta debería estar cubierta (rango inclusive al inicio)")
	}
}

func TestBloqueHorario_Cubre_LimiteFinExclusive(t *testing.T) {
	b, _ := NuevoBloqueHorario("id1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)

	if b.Cubre(Lunes, 12*time.Hour) {
		t.Error("la hora de fin exacta NO debería estar cubierta (rango exclusive al fin)")
	}
}

func TestBloqueHorario_Cubre_FueraDelRango(t *testing.T) {
	b, _ := NuevoBloqueHorario("id1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)

	if b.Cubre(Lunes, 7*time.Hour) {
		t.Error("07:00 no debería estar cubierta por el bloque 08:00-12:00")
	}
	if b.Cubre(Lunes, 13*time.Hour) {
		t.Error("13:00 no debería estar cubierta por el bloque 08:00-12:00")
	}
}

func TestBloqueHorario_Cubre_DiaDistinto_NoAplica(t *testing.T) {
	b, _ := NuevoBloqueHorario("id1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)

	if b.Cubre(Martes, 10*time.Hour) {
		t.Error("un bloque de LUNES no debería cubrir MARTES a la misma hora")
	}
}

// ── DiaYHoraDe ──────────────────────────────────────────────────────────

func TestDiaYHoraDe_MapeaCorrectamenteCadaDiaHabil(t *testing.T) {
	casos := []struct {
		fecha    time.Time
		esperado DiaSemana
	}{
		{time.Date(2026, time.March, 9, 10, 30, 0, 0, time.UTC), Lunes}, // 9-mar-2026 es lunes
		{time.Date(2026, time.March, 10, 10, 30, 0, 0, time.UTC), Martes},
		{time.Date(2026, time.March, 11, 10, 30, 0, 0, time.UTC), Miercoles},
		{time.Date(2026, time.March, 12, 10, 30, 0, 0, time.UTC), Jueves},
		{time.Date(2026, time.March, 13, 10, 30, 0, 0, time.UTC), Viernes},
	}
	for _, c := range casos {
		dia, _ := DiaYHoraDe(c.fecha)
		if dia != c.esperado {
			t.Errorf("DiaYHoraDe(%v) día = %q, esperaba %q", c.fecha, dia, c.esperado)
		}
	}
}

func TestDiaYHoraDe_FinDeSemana_DiaSemanaVacia(t *testing.T) {
	// 14 y 15 de marzo de 2026 son sábado y domingo. Ninguno es día hábil
	// del sistema, así que no deben matchear ningún bloque cargado.
	casos := map[string]time.Time{
		"sábado":  time.Date(2026, time.March, 14, 10, 30, 0, 0, time.UTC),
		"domingo": time.Date(2026, time.March, 15, 10, 30, 0, 0, time.UTC),
	}

	for nombre, fecha := range casos {
		dia, _ := DiaYHoraDe(fecha)
		if dia != DiaSemana("") {
			t.Errorf("%s: esperaba DiaSemana vacía, obtuve %q", nombre, dia)
		}
	}
}

// El enum dejó de aceptar SABADO: availability era el último lugar donde
// entraba, y creaba filas que ni el frontend ni el resto del backend
// reconocen como un día del sistema.
func TestParseDiaSemana_Sabado_Invalido(t *testing.T) {
	if _, err := ParseDiaSemana("SABADO"); !errors.Is(err, ErrDiaSemanaInvalido) {
		t.Fatalf("esperaba ErrDiaSemanaInvalido, obtuve %v", err)
	}
}

func TestDiaYHoraDe_CalculaOffsetDesdeMedianoche(t *testing.T) {
	fecha := time.Date(2026, time.March, 9, 14, 35, 20, 0, time.UTC)

	_, hora := DiaYHoraDe(fecha)

	esperado := 14*time.Hour + 35*time.Minute + 20*time.Second
	if hora != esperado {
		t.Errorf("hora = %v, esperaba %v", hora, esperado)
	}
}

// ── FechaSolo ───────────────────────────────────────────────────────────

func TestFechaSolo_DescartaLaHora(t *testing.T) {
	conHora := time.Date(2026, time.March, 9, 23, 59, 59, 0, time.UTC)

	solo := FechaSolo(conHora)

	if solo.Hour() != 0 || solo.Minute() != 0 || solo.Second() != 0 {
		t.Errorf("FechaSolo debería descartar la hora, obtuve %v", solo)
	}
	if solo.Year() != 2026 || solo.Month() != time.March || solo.Day() != 9 {
		t.Errorf("FechaSolo alteró la fecha: %v", solo)
	}
}
