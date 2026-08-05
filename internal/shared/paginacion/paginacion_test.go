package paginacion

import (
	"errors"
	"testing"
)

func TestParsear_SinParametros_DaLaVentanaPorDefecto(t *testing.T) {
	p, err := Parsear("", "")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if p.Numero != 1 || p.Tamanio != TamanioPorDefecto {
		t.Errorf("obtuve %+v, esperaba la ventana por defecto", p)
	}
}

func TestParsear_Validos(t *testing.T) {
	casos := []struct {
		page, pageSize string
		numero, tam    int
	}{
		{"1", "10", 1, 10},
		{"3", "", 3, TamanioPorDefecto},
		{"", "200", 1, TamanioMaximo},
		{"7", "1", 7, 1},
	}
	for _, c := range casos {
		p, err := Parsear(c.page, c.pageSize)
		if err != nil {
			t.Errorf("page=%q pageSize=%q: %v", c.page, c.pageSize, err)
			continue
		}
		if p.Numero != c.numero || p.Tamanio != c.tam {
			t.Errorf("page=%q pageSize=%q: obtuve %+v", c.page, c.pageSize, p)
		}
	}
}

func TestParsear_Invalidos(t *testing.T) {
	casos := map[string]struct {
		page, pageSize string
		esperado       error
	}{
		"página cero":          {"0", "", ErrPaginaInvalida},
		"página negativa":      {"-2", "", ErrPaginaInvalida},
		"página no numérica":   {"abc", "", ErrPaginaInvalida},
		"tamaño cero":          {"", "0", ErrTamanioInvalido},
		"tamaño no numérico":   {"", "muchas", ErrTamanioInvalido},
		"tamaño sobre el tope": {"", "100000", ErrTamanioInvalido},
	}
	for nombre, c := range casos {
		_, err := Parsear(c.page, c.pageSize)
		if !errors.Is(err, c.esperado) {
			t.Errorf("%s: obtuve %v, esperaba %v", nombre, err, c.esperado)
		}
	}
}

func TestOffset(t *testing.T) {
	casos := []struct {
		p        Pagina
		esperado int
	}{
		{Pagina{Numero: 1, Tamanio: 50}, 0},
		{Pagina{Numero: 2, Tamanio: 50}, 50},
		{Pagina{Numero: 4, Tamanio: 25}, 75},
	}
	for _, c := range casos {
		if got := c.p.Offset(); got != c.esperado {
			t.Errorf("%+v: offset %d, esperaba %d", c.p, got, c.esperado)
		}
	}
}
