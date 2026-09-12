package domain

import (
	"errors"
	"testing"
	"time"
)

func fecha(s string) *time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestFiltro_SinFechasEsValido(t *testing.T) {
	if err := (Filtro{Accion: "CURSO_ELIMINADO"}).Validar(); err != nil {
		t.Errorf("un filtro sin rango no tiene nada que validar: %v", err)
	}
}

// Una sola punta tampoco se valida: «desde el 1°» sin tope es una consulta
// legítima, y el tope de página ya acota cuánto vuelve.
func TestFiltro_UnaSolaPuntaEsValida(t *testing.T) {
	if err := (Filtro{Desde: fecha("2026-01-01")}).Validar(); err != nil {
		t.Errorf("sólo «desde» tendría que valer: %v", err)
	}
	if err := (Filtro{Hasta: fecha("2026-01-01")}).Validar(); err != nil {
		t.Errorf("sólo «hasta» tendría que valer: %v", err)
	}
}

func TestFiltro_RangoInvertido(t *testing.T) {
	f := Filtro{Desde: fecha("2026-09-11"), Hasta: fecha("2026-09-01")}
	if err := f.Validar(); !errors.Is(err, ErrRangoInvertido) {
		t.Errorf("esperaba ErrRangoInvertido, obtuve: %v", err)
	}
}

// El tope existe para que un año tipeado mal —«desde 1970»— no se lleve la
// tabla por delante.
func TestFiltro_RangoDemasiadoLargo(t *testing.T) {
	f := Filtro{Desde: fecha("2000-01-01"), Hasta: fecha("2026-01-01")}
	if err := f.Validar(); !errors.Is(err, ErrRangoLargo) {
		t.Errorf("esperaba ErrRangoLargo, obtuve: %v", err)
	}
}

func TestFiltro_UnAnioJustoEntra(t *testing.T) {
	// 2026 no es bisiesto: 365 días, dentro del tope de 366.
	f := Filtro{Desde: fecha("2026-01-01"), Hasta: fecha("2027-01-01")}
	if err := f.Validar(); err != nil {
		t.Errorf("un ciclo lectivo entero tiene que entrar: %v", err)
	}
}

// El mismo día en las dos puntas es la consulta más común de todas —«qué pasó
// hoy»— y no puede ser un error.
func TestFiltro_MismoDiaEnLasDosPuntas(t *testing.T) {
	f := Filtro{Desde: fecha("2026-09-11"), Hasta: fecha("2026-09-11")}
	if err := f.Validar(); err != nil {
		t.Errorf("«hoy» tiene que ser un rango válido: %v", err)
	}
}
