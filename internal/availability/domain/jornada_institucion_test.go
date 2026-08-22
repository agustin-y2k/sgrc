package domain

import (
	"errors"
	"testing"
	"time"
)

func bloqueJornada(dia DiaSemana, desde, hasta int) *BloqueJornada {
	return &BloqueJornada{
		DiaSemana:  dia,
		HoraInicio: time.Duration(desde) * time.Hour,
		HoraFin:    time.Duration(hasta) * time.Hour,
	}
}

// La distinción que sostiene todo el diseño: una jornada vacía significa
// "todavía no la declararon", no "la escuela está cerrada".
func TestPermiteReserva_SinJornadaDeclarada_NoRestringe(t *testing.T) {
	for nombre, dia := range map[string]DiaSemana{
		"un martes":  Martes,
		"un sábado":  Sabado,
		"un domingo": Domingo,
	} {
		if !PermiteReserva(nil, dia, 3*time.Hour, 5*time.Hour) {
			t.Errorf("%s: sin jornada declarada no tendría que haber restricción", nombre)
		}
	}
}

// La otra mitad: con jornada declarada, un día sin tramos es un día en que
// la escuela no abre.
func TestPermiteReserva_DiaSinTramos_Rechaza(t *testing.T) {
	jornada := []*BloqueJornada{bloqueJornada(Lunes, 8, 12)}

	if PermiteReserva(jornada, Sabado, 9*time.Hour, 10*time.Hour) {
		t.Error("el sábado no tiene tramos declarados: tendría que rechazarse")
	}
}

func TestPermiteReserva_DentroDelTramo(t *testing.T) {
	jornada := []*BloqueJornada{bloqueJornada(Sabado, 8, 13)}

	casos := map[string]struct {
		desde, hasta int
		esperado     bool
	}{
		"entera adentro":  {9, 12, true},
		"pegada al borde": {8, 13, true},
		"empieza antes":   {7, 12, false},
		"termina después": {9, 14, false},
		"entera afuera":   {15, 17, false},
	}

	for nombre, caso := range casos {
		got := PermiteReserva(jornada, Sabado,
			time.Duration(caso.desde)*time.Hour, time.Duration(caso.hasta)*time.Hour)
		if got != caso.esperado {
			t.Errorf("%s (%d a %d): esperaba %v, obtuve %v", nombre, caso.desde, caso.hasta, caso.esperado, got)
		}
	}
}

// Turno mañana y turno noche, con el mediodía cerrado.
func TestPermiteReserva_DosTurnos_NoSePuedeCruzarElHueco(t *testing.T) {
	jornada := []*BloqueJornada{
		bloqueJornada(Miercoles, 7, 12),
		bloqueJornada(Miercoles, 18, 23),
	}

	if !PermiteReserva(jornada, Miercoles, 8*time.Hour, 11*time.Hour) {
		t.Error("de 8 a 11 cae dentro del turno mañana: tendría que permitirse")
	}
	if !PermiteReserva(jornada, Miercoles, 19*time.Hour, 22*time.Hour) {
		t.Error("de 19 a 22 cae dentro del turno noche: tendría que permitirse")
	}
	if PermiteReserva(jornada, Miercoles, 11*time.Hour, 19*time.Hour) {
		t.Error("de 11 a 19 cruza el mediodía cerrado: tendría que rechazarse")
	}
}

// Dos tramos contiguos describen un día abierto de punta a punta.
func TestPermiteReserva_TramosContiguos_SeFusionan(t *testing.T) {
	jornada := []*BloqueJornada{
		bloqueJornada(Jueves, 12, 18),
		bloqueJornada(Jueves, 7, 12), // desordenado a propósito
	}

	if !PermiteReserva(jornada, Jueves, 11*time.Hour, 13*time.Hour) {
		t.Error("07–12 y 12–18 son contiguos: de 11 a 13 tendría que permitirse")
	}
}

// Un día no puede opinar sobre otro: los tramos del lunes no habilitan nada
// el martes.
func TestPermiteReserva_NoMezclaDias(t *testing.T) {
	jornada := []*BloqueJornada{
		bloqueJornada(Lunes, 7, 22),
		bloqueJornada(Martes, 8, 10),
	}

	if PermiteReserva(jornada, Martes, 7*time.Hour, 22*time.Hour) {
		t.Error("el martes solo abre de 8 a 10: no tendría que heredar el tramo del lunes")
	}
}

// "Fin antes del inicio" dejó de ser un error: significa que el tramo cruza
// la medianoche, que es como abre una escuela nocturna.
func TestNuevoBloqueJornada_RangoInvalido(t *testing.T) {
	if _, err := NuevoBloqueJornada("id1", Lunes, 8*time.Hour, 8*time.Hour); !errors.Is(err, ErrRangoHorarioInvalido) {
		t.Errorf("fin igual al inicio: esperaba ErrRangoHorarioInvalido, obtuve %v", err)
	}
	if _, err := NuevoBloqueJornada("id2", Lunes, 20*time.Hour, 1*time.Hour); err != nil {
		t.Errorf("20:00–01:00 es una jornada nocturna válida, obtuve %v", err)
	}
}

// Una nocturna abre de 20:00 a 01:00 y dicta de 22:00 a 00:30. Todo se mide
// desde la misma medianoche: la clase es [22h, 24.5h) y la jornada [20h, 25h).
func TestPermiteReserva_JornadaNocturna(t *testing.T) {
	jornada := []*BloqueJornada{{DiaSemana: Martes, HoraInicio: 20 * time.Hour, HoraFin: 1 * time.Hour}}

	casos := map[string]struct {
		desde, hasta time.Duration
		esperado     bool
	}{
		"clase que cruza la medianoche":     {22 * time.Hour, 30 * time.Minute, true},
		"clase que termina justo al cierre": {23 * time.Hour, 1 * time.Hour, true},
		"clase entera antes de medianoche":  {20 * time.Hour, 22 * time.Hour, true},
		"empieza antes de abrir":            {19 * time.Hour, 23 * time.Hour, false},
		"se pasa del cierre":                {23 * time.Hour, 2 * time.Hour, false},
	}

	for nombre, caso := range casos {
		if got := PermiteReserva(jornada, Martes, caso.desde, caso.hasta); got != caso.esperado {
			t.Errorf("%s: esperaba %v, obtuve %v", nombre, caso.esperado, got)
		}
	}
}

// Tocarse no es pisarse: es exactamente el caso de una escuela que declara
// sus dos turnos sin hueco entre ellos, y rechazarlo la obligaría a mentir
// poniendo 11:59.
func TestSolapaCon(t *testing.T) {
	base := bloqueJornada(Viernes, 8, 12)

	casos := map[string]struct {
		otro     *BloqueJornada
		esperado bool
	}{
		"contiguo por derecha":   {bloqueJornada(Viernes, 12, 18), false},
		"contiguo por izquierda": {bloqueJornada(Viernes, 6, 8), false},
		"se pisa parcialmente":   {bloqueJornada(Viernes, 11, 13), true},
		"contenido":              {bloqueJornada(Viernes, 9, 10), true},
		"otro día":               {bloqueJornada(Sabado, 8, 12), false},
	}

	for nombre, caso := range casos {
		if got := base.SolapaCon(caso.otro); got != caso.esperado {
			t.Errorf("%s: esperaba %v, obtuve %v", nombre, caso.esperado, got)
		}
	}
}

// ── CierreDe ────────────────────────────────────────────────────────────

func TestCierreDe_UnSoloTramo(t *testing.T) {
	jornada := []*BloqueJornada{
		{ID: "1", DiaSemana: Lunes, HoraInicio: 8 * time.Hour, HoraFin: 18 * time.Hour},
	}

	fin, abre := CierreDe(jornada, Lunes)

	if !abre || fin != 18*time.Hour {
		t.Errorf("esperaba cerrar a las 18h, obtuve %v (abre=%v)", fin, abre)
	}
}

// Turno mañana y turno noche: la escuela cierra cuando termina la noche, no
// cuando termina la mañana.
func TestCierreDe_ConVariosTramosEsElUltimo(t *testing.T) {
	jornada := []*BloqueJornada{
		{ID: "1", DiaSemana: Lunes, HoraInicio: 18 * time.Hour, HoraFin: 23 * time.Hour},
		{ID: "2", DiaSemana: Lunes, HoraInicio: 7 * time.Hour, HoraFin: 12 * time.Hour},
	}

	fin, abre := CierreDe(jornada, Lunes)

	if !abre || fin != 23*time.Hour {
		t.Errorf("esperaba cerrar a las 23h, obtuve %v (abre=%v)", fin, abre)
	}
}

// La nocturna cierra pasada la medianoche, y el valor lo dice: 25h es la
// 01:00 del día siguiente. Devolverlo así permite sumarle la gracia sin tener
// que saber de qué día se habla.
func TestCierreDe_CuandoCruzaLaMedianochePasaDe24(t *testing.T) {
	jornada := []*BloqueJornada{
		{ID: "1", DiaSemana: Lunes, HoraInicio: 20 * time.Hour, HoraFin: 1 * time.Hour},
	}

	fin, abre := CierreDe(jornada, Lunes)

	if !abre || fin != 25*time.Hour {
		t.Errorf("esperaba 25h (01:00 del martes), obtuve %v (abre=%v)", fin, abre)
	}
}

func TestCierreDe_UnDiaSinTramosNoAbre(t *testing.T) {
	jornada := []*BloqueJornada{
		{ID: "1", DiaSemana: Lunes, HoraInicio: 8 * time.Hour, HoraFin: 18 * time.Hour},
	}

	if _, abre := CierreDe(jornada, Sabado); abre {
		t.Error("el sábado no tiene tramos: no abre")
	}
}

// Sin jornada declarada no hay de dónde deducir un cierre. Es distinto de
// "no abre": quien pregunta tiene que poder caer a su valor por defecto.
func TestCierreDe_SinJornadaDeclaradaNoAbre(t *testing.T) {
	if _, abre := CierreDe(nil, Lunes); abre {
		t.Error("sin jornada no hay cierre que deducir")
	}
}

// ── MomentoDentroDeLaJornada ────────────────────────────────────────────

func TestMomentoDentroDeLaJornada(t *testing.T) {
	jornada := []*BloqueJornada{
		{ID: "1", DiaSemana: Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour},
		{ID: "2", DiaSemana: Lunes, HoraInicio: 18 * time.Hour, HoraFin: 23 * time.Hour},
	}

	casos := []struct {
		nombre   string
		dia      DiaSemana
		hora     time.Duration
		esperado bool
	}{
		{"en pleno turno mañana", Lunes, 10 * time.Hour, true},
		{"en el mediodía cerrado", Lunes, 15 * time.Hour, false},
		{"justo al abrir", Lunes, 8 * time.Hour, true},
		// Devolver exactamente a la hora de cierre es devolver en horario.
		{"justo al cerrar", Lunes, 23 * time.Hour, true},
		{"antes de abrir", Lunes, 7 * time.Hour, false},
		{"un día que no abre", Sabado, 10 * time.Hour, false},
	}

	for _, c := range casos {
		if got := MomentoDentroDeLaJornada(jornada, c.dia, c.hora); got != c.esperado {
			t.Errorf("%s: esperaba %v, obtuve %v", c.nombre, c.esperado, got)
		}
	}
}

// El tramo que cruza la medianoche se mide desde la medianoche de SU día, así
// que la 01:00 del martes es 25h del lunes.
func TestMomentoDentroDeLaJornada_CuandoElTramoCruza(t *testing.T) {
	nocturna := []*BloqueJornada{
		{ID: "1", DiaSemana: Lunes, HoraInicio: 20 * time.Hour, HoraFin: 1 * time.Hour},
	}

	if !MomentoDentroDeLaJornada(nocturna, Lunes, 24*time.Hour+30*time.Minute) {
		t.Error("las 00:30 del martes caen dentro del lunes de la nocturna")
	}
	if MomentoDentroDeLaJornada(nocturna, Lunes, 19*time.Hour) {
		t.Error("las 19:00 son antes de que abra")
	}
}

// Sin jornada declarada no hay restricción, igual que en PermiteReserva: un
// préstamo no puede quedar "fuera" de un horario que nadie declaró.
func TestMomentoDentroDeLaJornada_SinJornadaNoHayRestriccion(t *testing.T) {
	if !MomentoDentroDeLaJornada(nil, Domingo, 3*time.Hour) {
		t.Error("sin tramos declarados todo momento está adentro")
	}
}
