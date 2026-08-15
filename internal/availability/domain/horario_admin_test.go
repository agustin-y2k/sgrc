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
	// Ojo con qué se elige de ejemplo inválido: hasta que los siete días
	// entraron al enum, este caso usaba "DOMINGO", que hoy es válido. Un
	// nombre de día real no sirve para probar el rechazo.
	casos := []string{"", "lunes", "FERIADO"}
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

func TestDiaYHoraDe_FinDeSemana_SeTraduceComoCualquierDia(t *testing.T) {
	// 14 y 15 de marzo de 2026 son sábado y domingo. Los dos tienen que
	// traducirse como cualquier otro día: si el fin de semana no se mapeara,
	// DisponibleAhora respondería "no hay nadie" un sábado aunque el Admin
	// hubiera cargado su horario, y no habría forma de corregirlo desde la
	// aplicación. Que un sábado no haya nadie tiene que salir de que no hay
	// bloques cargados, no de que el día no exista.
	casos := map[string]struct {
		fecha    time.Time
		esperado DiaSemana
	}{
		"sábado":  {time.Date(2026, time.March, 14, 10, 30, 0, 0, time.UTC), Sabado},
		"domingo": {time.Date(2026, time.March, 15, 10, 30, 0, 0, time.UTC), Domingo},
	}

	for nombre, caso := range casos {
		dia, _ := DiaYHoraDe(caso.fecha)
		if dia != caso.esperado {
			t.Errorf("%s: esperaba %q, obtuve %q", nombre, caso.esperado, dia)
		}
	}
}

// Los siete días entran al enum. Las escuelas de jornada extendida o
// albergue dictan el fin de semana, y antes un Admin de una de ellas no
// podía siquiera declarar que ese día está.
func TestParseDiaSemana_FinDeSemana_Valido(t *testing.T) {
	for _, dia := range []string{"SABADO", "DOMINGO"} {
		if _, err := ParseDiaSemana(dia); err != nil {
			t.Errorf("ParseDiaSemana(%q): esperaba válido, obtuve %v", dia, err)
		}
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

// ── Solapamiento entre bloques ──────────────────────────────────────────

func TestSeSolapaCon(t *testing.T) {
	bloque := func(dia DiaSemana, desde, hasta time.Duration) *BloqueHorario {
		return &BloqueHorario{DiaSemana: dia, HoraInicio: desde, HoraFin: hasta}
	}
	h := func(n int) time.Duration { return time.Duration(n) * time.Hour }

	casos := []struct {
		nombre   string
		a, b     *BloqueHorario
		esperado bool
	}{
		{"se pisan por el medio", bloque(Lunes, h(8), h(12)), bloque(Lunes, h(10), h(14)), true},
		{"uno contiene al otro", bloque(Lunes, h(8), h(18)), bloque(Lunes, h(10), h(12)), true},
		{"idénticos", bloque(Lunes, h(8), h(12)), bloque(Lunes, h(8), h(12)), true},
		// El caso que NO hay que rechazar: mañana y tarde de corrido es lo
		// más común que carga un Admin, y con rangos cerrados se prohibiría.
		{"se tocan en el borde", bloque(Lunes, h(8), h(12)), bloque(Lunes, h(12), h(18)), false},
		{"separados el mismo día", bloque(Lunes, h(8), h(10)), bloque(Lunes, h(14), h(18)), false},
		{"mismo horario, día distinto", bloque(Lunes, h(8), h(12)), bloque(Martes, h(8), h(12)), false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := c.a.SeSolapaCon(c.b); got != c.esperado {
				t.Errorf("a.SeSolapaCon(b) = %v, esperaba %v", got, c.esperado)
			}
			// La relación es simétrica: da igual cuál se pregunta.
			if got := c.b.SeSolapaCon(c.a); got != c.esperado {
				t.Errorf("b.SeSolapaCon(a) = %v, esperaba %v — no es simétrico", got, c.esperado)
			}
		})
	}
}

// Editar un bloque sin moverlo no puede chocar contra su propia versión
// guardada: sin ignorarse por ID, guardar dos veces lo mismo daría error.
func TestPrimeroQueSeSolapa_SeIgnoraASiMismo(t *testing.T) {
	guardado := &BloqueHorario{ID: "b1", DiaSemana: Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour}
	mismoEditado := &BloqueHorario{ID: "b1", DiaSemana: Lunes, HoraInicio: 8 * time.Hour, HoraFin: 13 * time.Hour}

	if choca := mismoEditado.PrimeroQueSeSolapa([]*BloqueHorario{guardado}); choca != nil {
		t.Errorf("no debería chocar consigo mismo, chocó con %+v", choca)
	}
}

func TestPrimeroQueSeSolapa_DevuelveElQueEstorba(t *testing.T) {
	manana := &BloqueHorario{ID: "b1", DiaSemana: Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour}
	tarde := &BloqueHorario{ID: "b2", DiaSemana: Lunes, HoraInicio: 14 * time.Hour, HoraFin: 18 * time.Hour}
	nuevo := &BloqueHorario{ID: "b3", DiaSemana: Lunes, HoraInicio: 16 * time.Hour, HoraFin: 20 * time.Hour}

	choca := nuevo.PrimeroQueSeSolapa([]*BloqueHorario{manana, tarde})
	if choca == nil {
		t.Fatal("esperaba que chocara con el de la tarde")
	}
	if choca.ID != "b2" {
		t.Errorf("esperaba el bloque de la tarde, obtuve %s", choca.ID)
	}
}
