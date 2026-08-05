package domain

import (
	"errors"
	"testing"
	"time"
)

func fecha(anio int, mes time.Month, dia int) time.Time {
	return time.Date(anio, mes, dia, 0, 0, 0, 0, time.UTC)
}

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
	// La semana lectiva es de lunes a viernes: sábado y domingo no son días
	// sobre los que se pueda reservar.
	casos := []string{"", "lunes", "SABADO", "DOMINGO"}
	for _, c := range casos {
		_, err := ParseDiaSemana(c)
		if !errors.Is(err, ErrDiaSemanaInvalido) {
			t.Errorf("ParseDiaSemana(%q): esperaba ErrDiaSemanaInvalido, obtuve %v", c, err)
		}
	}
}

func TestEsDiaLectivo(t *testing.T) {
	casos := map[string]struct {
		fecha    time.Time
		esLabora bool
	}{
		"lunes":   {fecha(2026, time.August, 3), true},
		"viernes": {fecha(2026, time.August, 7), true},
		"sábado":  {fecha(2026, time.August, 8), false},
		"domingo": {fecha(2026, time.August, 9), false},
	}
	for nombre, caso := range casos {
		if got := EsDiaLectivo(caso.fecha); got != caso.esLabora {
			t.Errorf("EsDiaLectivo(%s) = %v, esperaba %v", nombre, got, caso.esLabora)
		}
	}
}

func TestNuevaReglaRecurrencia_RangoFechasInvalido_Error(t *testing.T) {
	_, err := NuevaReglaRecurrencia("id1", "materia1", "usuario1", Lunes, 8*time.Hour, 10*time.Hour,
		fecha(2026, time.March, 10), fecha(2026, time.March, 1)) // fin antes que inicio

	if !errors.Is(err, ErrRangoFechasInvalido) {
		t.Fatalf("esperaba ErrRangoFechasInvalido, obtuve %v", err)
	}
}

func TestNuevaReglaRecurrencia_RangoHorarioInvalido_Error(t *testing.T) {
	_, err := NuevaReglaRecurrencia("id1", "materia1", "usuario1", Lunes, 10*time.Hour, 8*time.Hour,
		fecha(2026, time.March, 1), fecha(2026, time.March, 10))

	if !errors.Is(err, ErrRangoHorarioInvalido) {
		t.Fatalf("esperaba ErrRangoHorarioInvalido, obtuve %v", err)
	}
}

// ── GenerarFechas — el algoritmo con más superficie para bugs sutiles ──

func TestGenerarFechas_UnMesCompleto_Lunes(t *testing.T) {
	// Marzo 2026: los lunes caen el 2, 9, 16, 23, 30.
	r, err := NuevaReglaRecurrencia("id1", "m1", "u1", Lunes, 8*time.Hour, 9*time.Hour,
		fecha(2026, time.March, 1), fecha(2026, time.March, 31))
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	fechas := r.GenerarFechas()

	esperadas := []int{2, 9, 16, 23, 30}
	if len(fechas) != len(esperadas) {
		t.Fatalf("esperaba %d fechas, obtuve %d: %v", len(esperadas), len(fechas), fechas)
	}
	for i, dia := range esperadas {
		if fechas[i].Day() != dia || fechas[i].Month() != time.March {
			t.Errorf("fecha[%d] = %v, esperaba día %d de marzo", i, fechas[i], dia)
		}
	}
}

func TestGenerarFechas_FechaInicioEsElDiaBuscado_Incluida(t *testing.T) {
	// Si FechaInicio ya es un lunes, debe incluirse (inclusive, no exclusive).
	r, _ := NuevaReglaRecurrencia("id1", "m1", "u1", Lunes, 8*time.Hour, 9*time.Hour,
		fecha(2026, time.March, 2), fecha(2026, time.March, 2)) // un solo día, y es lunes

	fechas := r.GenerarFechas()

	if len(fechas) != 1 || fechas[0].Day() != 2 {
		t.Fatalf("esperaba exactamente el 2 de marzo, obtuve %v", fechas)
	}
}

func TestGenerarFechas_FechaFinEsElDiaBuscado_Incluida(t *testing.T) {
	r, _ := NuevaReglaRecurrencia("id1", "m1", "u1", Lunes, 8*time.Hour, 9*time.Hour,
		fecha(2026, time.March, 1), fecha(2026, time.March, 2)) // termina justo el lunes

	fechas := r.GenerarFechas()

	if len(fechas) != 1 || fechas[0].Day() != 2 {
		t.Fatalf("esperaba exactamente el 2 de marzo, obtuve %v", fechas)
	}
}

func TestGenerarFechas_RangoSinNingunaOcurrencia_ListaVacia(t *testing.T) {
	// Un rango de una semana que no incluye ningún lunes.
	r, _ := NuevaReglaRecurrencia("id1", "m1", "u1", Lunes, 8*time.Hour, 9*time.Hour,
		fecha(2026, time.March, 3), fecha(2026, time.March, 8)) // martes a domingo

	fechas := r.GenerarFechas()

	if len(fechas) != 0 {
		t.Fatalf("esperaba 0 fechas, obtuve %v", fechas)
	}
}

func TestGenerarFechas_CruzaFinDeMes(t *testing.T) {
	// Confirma que el algoritmo no se rompe al cruzar límites de mes.
	r, _ := NuevaReglaRecurrencia("id1", "m1", "u1", Lunes, 8*time.Hour, 9*time.Hour,
		fecha(2026, time.March, 25), fecha(2026, time.April, 6))

	fechas := r.GenerarFechas()

	// Lunes en ese rango: 30 de marzo y 6 de abril.
	if len(fechas) != 2 {
		t.Fatalf("esperaba 2 fechas, obtuve %d: %v", len(fechas), fechas)
	}
	if fechas[0].Month() != time.March || fechas[0].Day() != 30 {
		t.Errorf("primera fecha incorrecta: %v", fechas[0])
	}
	if fechas[1].Month() != time.April || fechas[1].Day() != 6 {
		t.Errorf("segunda fecha incorrecta: %v", fechas[1])
	}
}

func TestGenerarFechas_CruzaFinDeAnio(t *testing.T) {
	r, _ := NuevaReglaRecurrencia("id1", "m1", "u1", Viernes, 8*time.Hour, 9*time.Hour,
		fecha(2026, time.December, 28), fecha(2027, time.January, 4))

	fechas := r.GenerarFechas()

	// Viernes en ese rango: 28-dic-2026 es lunes, así que el único
	// viernes hasta el 4-ene-2027 (inclusive) es el 1 de enero de 2027.
	if len(fechas) != 1 {
		t.Fatalf("esperaba 1 fecha, obtuve %d: %v", len(fechas), fechas)
	}
	if fechas[0].Year() != 2027 || fechas[0].Month() != time.January || fechas[0].Day() != 1 {
		t.Errorf("fecha incorrecta: %v", fechas[0])
	}
}

func TestGenerarFechas_AnioBisiesto_29DeFebrero(t *testing.T) {
	// 2028 es bisiesto. El 29 de febrero de 2028 cae martes.
	r, _ := NuevaReglaRecurrencia("id1", "m1", "u1", Martes, 8*time.Hour, 9*time.Hour,
		fecha(2028, time.February, 25), fecha(2028, time.March, 3))

	fechas := r.GenerarFechas()

	if len(fechas) != 1 || fechas[0].Day() != 29 || fechas[0].Month() != time.February {
		t.Fatalf("esperaba el 29 de febrero de 2028, obtuve %v", fechas)
	}
}
