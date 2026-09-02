package domain

import (
	"testing"
	"time"
)

// Los bloques que cruzan la medianoche son lo que hace posible una escuela
// nocturna.

func hs(h int) time.Duration { return time.Duration(h) * time.Hour }

func TestDuracionDe(t *testing.T) {
	casos := map[string]struct {
		inicio, fin time.Duration
		esperado    time.Duration
	}{
		"mismo día":              {hs(8), hs(12), 4 * time.Hour},
		"cruza la medianoche":    {hs(22), hs(1), 3 * time.Hour},
		"termina justo a las 00": {hs(22), 0, 2 * time.Hour},
		"empieza justo a las 00": {0, hs(3), 3 * time.Hour},
		"casi el día entero":     {hs(1), 0, 23 * time.Hour},
	}

	for nombre, caso := range casos {
		if got := DuracionDe(caso.inicio, caso.fin); got != caso.esperado {
			t.Errorf("%s: esperaba %v, obtuve %v", nombre, caso.esperado, got)
		}
	}
}

// El caso que más se paga si se equivoca: una clase nocturna dada por
// terminada apenas se crea.
func TestYaTermino_CruzandoLaMedianoche(t *testing.T) {
	lunes := time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC)
	enElLunes := func(h, m int) time.Time {
		return time.Date(2026, time.March, 2, h, m, 0, 0, time.UTC)
	}
	enElMartes := func(h, m int) time.Time {
		return time.Date(2026, time.March, 3, h, m, 0, 0, time.UTC)
	}

	casos := map[string]struct {
		ahora    time.Time
		esperado bool
	}{
		"antes de empezar":               {enElLunes(21, 0), false},
		"en plena clase":                 {enElLunes(23, 30), false},
		"pasada la medianoche, en clase": {enElMartes(0, 30), false},
		"justo a la hora de fin":         {enElMartes(1, 0), true},
		"al otro día":                    {enElMartes(9, 0), true},
	}

	for nombre, caso := range casos {
		got := YaTermino(lunes, hs(22), hs(1), caso.ahora)
		if got != caso.esperado {
			t.Errorf("%s (%s): esperaba %v, obtuve %v",
				nombre, caso.ahora.Format("Mon 15:04"), caso.esperado, got)
		}
	}
}

func TestFinDePared_SumaElDiaCuandoCruza(t *testing.T) {
	lunes := time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC)

	fin := FinDePared(lunes, hs(22), hs(1), time.UTC)
	esperado := time.Date(2026, time.March, 3, 1, 0, 0, 0, time.UTC)
	if !fin.Equal(esperado) {
		t.Errorf("nocturna: esperaba %v, obtuve %v", esperado, fin)
	}

	finNormal := FinDePared(lunes, hs(8), hs(12), time.UTC)
	esperadoNormal := time.Date(2026, time.March, 2, 12, 0, 0, 0, time.UTC)
	if !finNormal.Equal(esperadoNormal) {
		t.Errorf("diurna: esperaba %v, obtuve %v", esperadoNormal, finNormal)
	}
}

// Comparar hora_fin cruda daba 01:00 < casi todo y concluía que una clase
// nocturna no se pisa con nada — que es el bug que dejaría reservar encima de
// ella.
func TestSolapaCon_CruzandoLaMedianoche(t *testing.T) {
	nocturna := &Reserva{HoraInicio: hs(22), HoraFin: hs(1)}

	casos := map[string]struct {
		inicio, fin time.Duration
		esperado    bool
	}{
		"pisa el arranque":             {hs(21), hs(23), true},
		"contenida en la madrugada":    {hs(23), 0, true},
		"idéntica":                     {hs(22), hs(1), true},
		"termina justo cuando empieza": {hs(20), hs(22), false},
		"a la mañana siguiente":        {hs(8), hs(10), false},
		"otra nocturna más tarde":      {hs(1), hs(3), false},
	}

	for nombre, caso := range casos {
		if got := nocturna.SolapaCon(caso.inicio, caso.fin); got != caso.esperado {
			t.Errorf("%s: esperaba %v, obtuve %v", nombre, caso.esperado, got)
		}
	}
}

// El tope de duración se mide sobre la duración real.
func TestValidarVentanaTemporal_TopeDeDuracionConCruce(t *testing.T) {
	ahora := time.Date(2026, time.March, 2, 12, 0, 0, 0, time.UTC)
	manana := time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC)

	if err := ValidarVentanaTemporal(manana, hs(22), hs(1), ahora); err != nil {
		t.Errorf("tres horas cruzando la medianoche tendría que entrar: %v", err)
	}
	if err := ValidarVentanaTemporal(manana, hs(20), hs(6), ahora); err == nil {
		t.Error("diez horas cruzando la medianoche supera el tope: tendría que fallar")
	}
}

// La nocturna que cruza la medianoche tiene que poder liberarse igual.
//
// Antes esto lo cubría PuedeLlegarALiberarse, que se retiró con el aviso de no
// retiro (era su único llamador). La protección no se perdió: CorrespondeLiberar
// mide sobre la duración real, y una clase más corta que la gracia queda afuera
// por YaTermino —a los 40 minutos ya terminó— sin necesidad de una regla aparte.
func TestCorrespondeLiberar_ConCruceDeMedianoche(t *testing.T) {
	// Nocturna del lunes 22:00 a 01:00, y son las 22:45 del lunes: pasaron los
	// 40 de gracia y la clase sigue en curso.
	lunes := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	alas2245 := time.Date(2026, 8, 10, 22, 45, 0, 0, time.UTC)
	if !CorrespondeLiberar(lunes, hs(22), hs(1), 40*time.Minute, alas2245) {
		t.Error("una nocturna de tres horas ya pasó su gracia a las 22:45")
	}

	// Una clase de una hora con 90 minutos de gracia no se libera nunca: para
	// cuando el plazo se cumple, la clase ya terminó.
	alas0030 := time.Date(2026, 8, 11, 0, 30, 0, 0, time.UTC)
	if CorrespondeLiberar(lunes, hs(23), 0, 90*time.Minute, alas0030) {
		t.Error("una clase más corta que la gracia no se libera: ya terminó")
	}
}
