package infrastructure

import (
	"strings"
	"testing"
)

// TestEtiquetaConCarroSQL fija la forma del fragmento, que es lo único que se
// puede probar sin base: el comportamiento contra Postgres lo cubren los tests
// de integración.
//
// Vale la pena igual porque el error que arregló es de los que no se ven: la
// consulta corre, devuelve una etiqueta, y lo que falta —el carro— solo se
// nota leyendo un aviso y no sabiendo a qué carro ir.
func TestEtiquetaConCarroSQL(t *testing.T) {
	sql := etiquetaConCarroSQL("eq", "car")

	// El nombre primero: un proyector no tiene número ni carro.
	if !strings.Contains(sql, "COALESCE(eq.nombre,") {
		t.Errorf("el nombre tiene que mandar cuando existe: %s", sql)
	}
	// Y el carro pegado al número, no en una columna aparte.
	if !strings.Contains(sql, "'PC ' || eq.identificador") || !strings.Contains(sql, "' del ' || car.nombre") {
		t.Errorf("falta el carro junto al número: %s", sql)
	}
	// El COALESCE de adentro es lo que deja pasar a los equipos sueltos: sin
	// él, concatenar con un NULL daría NULL y la etiqueta saldría vacía.
	if !strings.Contains(sql, "COALESCE(' del '") {
		t.Errorf("sin el COALESCE interno un equipo suelto queda sin etiqueta: %s", sql)
	}
}
