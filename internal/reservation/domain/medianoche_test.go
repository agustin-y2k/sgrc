package domain

import (
	"testing"
	"time"
)

// Los bloques que cruzan la medianoche son lo que hace posible una escuela
// nocturna. Todo lo de este archivo existe porque antes el sistema exigía
// hora_fin > hora_inicio y una clase de 22:00 a 01:00 había que partirla en
// dos reservas sin relación entre sí.

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
// terminada apenas se crea. Con "fecha + hora_fin" a secas, una reserva del
// lunes 22:00–01:00 terminaba el lunes a la 01:00, o sea veintiuna horas
// ANTES de empezar.
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

// El tope de duración se mide sobre la duración real. Sin esto, una clase
// nocturna de tres horas daba una resta negativa, pasaba el tope sin
// problema y una de veintitrés horas también.
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

// PuedeLlegarALiberarse restaba las horas crudas, así que para una nocturna
// daba 01:00 > 22:40 = falso: el sistema concluía que la clase era más corta
// que el plazo de gracia y que nunca se libera.
func TestPuedeLlegarALiberarse_ConCruce(t *testing.T) {
	if !PuedeLlegarALiberarse(hs(22), hs(1), 40*time.Minute) {
		t.Error("una nocturna de tres horas es más larga que 40 minutos de gracia")
	}
	if PuedeLlegarALiberarse(hs(23), 0, 90*time.Minute) {
		t.Error("una clase de una hora es más corta que 90 minutos: no se libera nunca")
	}
}
