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
// "todavía no la declararon", no "la escuela está cerrada". Si se
// confundieran, instalar el sistema y no configurar nada dejaría a todos sin
// poder reservar, y el mensaje de error hablaría de una jornada que nadie
// cargó nunca.
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

// Turno mañana y turno noche, con el mediodía cerrado. La reserva tiene que
// entrar ENTERA en un tramo: una que cruce el hueco pediría el laboratorio
// durante horas en que la escuela está cerrada.
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

// Dos tramos contiguos describen un día abierto de punta a punta. Sin
// fusionarlos, una reserva que cruza la juntura no entraría entera en
// ninguno de los dos y se rechazaría sin que la persona pueda entender por
// qué: en la pantalla el día figura abierto de 7 a 18.
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

func TestNuevoBloqueJornada_RangoInvalido(t *testing.T) {
	for nombre, caso := range map[string]struct{ desde, hasta int }{
		"fin antes del inicio": {12, 8},
		"fin igual al inicio":  {8, 8},
	} {
		_, err := NuevoBloqueJornada("id1", Lunes,
			time.Duration(caso.desde)*time.Hour, time.Duration(caso.hasta)*time.Hour)
		if !errors.Is(err, ErrRangoHorarioInvalido) {
			t.Errorf("%s: esperaba ErrRangoHorarioInvalido, obtuve %v", nombre, err)
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
