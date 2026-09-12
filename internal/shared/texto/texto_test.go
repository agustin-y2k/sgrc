package texto

import "testing"

func TestCanonizar(t *testing.T) {
	casos := []struct{ entrada, esperado, porque string }{
		{"Carro 1", "Carro 1", "lo que ya está bien no se toca"},
		{"  Carro 1  ", "Carro 1", "recorta los bordes"},
		{"Carro  1", "Carro 1", "colapsa el doble espacio"},
		{"Carro   de   Informática", "Carro de Informática", "colapsa varios"},
		// El espacio duro viaja al copiar desde una página web y se imprime
		// igual que uno común: es el caso que nadie ve.
		{"Carro 1", "Carro 1", "unifica el espacio duro"},
		{"Carro  1", "Carro 1", "espacio duro pegado a uno común"},
		{"   ", "", "sólo espacios queda vacío"},
		{"", "", "vacío sigue vacío"},
		// No toca la caja ni las tildes: es el texto que se va a MOSTRAR.
		{"Carro EDUTEC", "Carro EDUTEC", "no baja a minúsculas"},
		{"Informática", "Informática", "no saca tildes"},
	}
	for _, c := range casos {
		if obtenido := Canonizar(c.entrada); obtenido != c.esperado {
			t.Errorf("Canonizar(%q) = %q; esperaba %q (%s)", c.entrada, obtenido, c.esperado, c.porque)
		}
	}
}

func TestClave_LoQueEsLoMismoDaLoMismo(t *testing.T) {
	referencia := Clave("Educación Física")
	mismas := []string{
		"Educación Física",
		"EDUCACION FISICA",
		"educación física",
		"Educacion Fisica",
		"  Educación   Física  ",
		"Educación Física",
		"EdUcAcIóN  fÍsIcA",
	}
	for _, v := range mismas {
		if Clave(v) != referencia {
			t.Errorf("Clave(%q) = %q; esperaba %q", v, Clave(v), referencia)
		}
		if !SonElMismo(v, "Educación Física") {
			t.Errorf("SonElMismo(%q, «Educación Física») dio false", v)
		}
	}
}

func TestClave_DistingueLoQueDeVerdadEsOtro(t *testing.T) {
	distintas := [][2]string{
		{"Educación Física", "Educación Física I"},
		{"Carro 1", "Carro 2"},
		{"Lengua", "Lenguas"},
		// La ñ no se confunde con la n de al lado de otra palabra: se traduce
		// igual que en la base, así que «año» y «ano» SÍ son el mismo texto —
		// se deja escrito para que el día que alguien lo mire no lo tome por
		// un error.
		{"Diseño", "Diseñado"},
	}
	for _, par := range distintas {
		if SonElMismo(par[0], par[1]) {
			t.Errorf("%q y %q no son la misma cosa", par[0], par[1])
		}
	}
}

// El contrato que sostiene todo: Clave() tiene que dar lo mismo que la función
// `clave_texto()` de la migración 011. Acá se fija el resultado esperado en
// texto literal; el test de integración de infrastructure compara contra la
// base de verdad.
func TestClave_FormaCanonica(t *testing.T) {
	casos := map[string]string{
		"Educación  Física":            "educacion fisica",
		"  CARRO   EDUTEC  ":           "carro edutec",
		"Tecnología de la Fabricación": "tecnologia de la fabricacion",
		"Ciencias Sociales: Geografía": "ciencias sociales: geografia",
		"Müller Ñandú":                 "muller nandu",
		"5CD 1234 ABC":                 "5cd 1234 abc",
	}
	for entrada, esperado := range casos {
		if obtenido := Clave(entrada); obtenido != esperado {
			t.Errorf("Clave(%q) = %q; esperaba %q", entrada, obtenido, esperado)
		}
	}
}
