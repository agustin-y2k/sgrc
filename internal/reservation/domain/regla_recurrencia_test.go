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
	// Los siete días son válidos, así que lo que se rechaza es lo que no es
	// un día: vacío, minúsculas y cualquier otra palabra. SABADO y DOMINGO
	// estaban acá cuando la semana terminaba el viernes.
	casos := []string{"", "lunes", "FERIADO"}
	for _, c := range casos {
		_, err := ParseDiaSemana(c)
		if !errors.Is(err, ErrDiaSemanaInvalido) {
			t.Errorf("ParseDiaSemana(%q): esperaba ErrDiaSemanaInvalido, obtuve %v", c, err)
		}
	}
}

// Una regla recurrente puede caer en sábado o en domingo. Antes el enum
// frenaba en viernes, así que una escuela albergue no podía siquiera
// expresar "todos los sábados de 9 a 11".
//
// El 1 y el 2 de agosto de 2026 son sábado y domingo, y agosto de 2026
// arranca justo un sábado, así que el mes tiene cinco de cada uno.
func TestGenerarFechas_FinDeSemana(t *testing.T) {
	casos := map[string]struct {
		dia      DiaSemana
		esperado int
	}{
		"sábados":  {Sabado, 5},
		"domingos": {Domingo, 5},
	}

	for nombre, caso := range casos {
		regla, err := NuevaReglaRecurrencia("id1", "materia1", "usuario1", caso.dia,
			9*time.Hour, 11*time.Hour,
			fecha(2026, time.August, 1), fecha(2026, time.August, 31))
		if err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}

		fechas := regla.GenerarFechas()
		if len(fechas) != caso.esperado {
			t.Errorf("%s: esperaba %d ocurrencias, obtuve %d", nombre, caso.esperado, len(fechas))
		}
		for _, f := range fechas {
			if got := diaSemanaAGoWeekday[caso.dia]; f.Weekday() != got {
				t.Errorf("%s: %s cayó en %s", nombre, f.Format("2006-01-02"), f.Weekday())
			}
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
	// Solo la igualdad. Una serie nocturna —"todos los lunes de 22 a 1"— es
	// válida y tiene su propio test más abajo.
	_, err := NuevaReglaRecurrencia("id1", "materia1", "usuario1", Lunes, 8*time.Hour, 8*time.Hour,
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

// Una serie recurrente nocturna: todos los lunes de 22:00 a 01:00. La regla
// se guarda con el día en que EMPIEZA la clase, así que las ocurrencias
// siguen cayendo en lunes aunque cada una termine un martes.
func TestGenerarFechas_SerieNocturna(t *testing.T) {
	regla, err := NuevaReglaRecurrencia("id1", "materia1", "usuario1", Lunes,
		22*time.Hour, 1*time.Hour,
		fecha(2026, time.March, 2), fecha(2026, time.March, 23))
	if err != nil {
		t.Fatalf("una serie nocturna es válida: %v", err)
	}

	fechas := regla.GenerarFechas()
	if len(fechas) != 4 {
		t.Fatalf("esperaba 4 lunes, obtuve %d", len(fechas))
	}
	for _, f := range fechas {
		if f.Weekday() != time.Monday {
			t.Errorf("%s cayó en %s", f.Format("2006-01-02"), f.Weekday())
		}
	}
}
