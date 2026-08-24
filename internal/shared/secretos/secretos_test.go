package secretos

import (
	"errors"
	"strings"
	"testing"
)

const secretoDePrueba = "un-secreto-de-prueba-bastante-largo"

func cifradorDePrueba(t *testing.T) *Cifrador {
	t.Helper()
	c, err := Nuevo(secretoDePrueba)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !c.Disponible() {
		t.Fatal("con secreto configurado tendría que estar disponible")
	}
	return c
}

func TestCifrarYDescifrar_VuelveElOriginal(t *testing.T) {
	c := cifradorDePrueba(t)

	// Con acentos y símbolos: las contraseñas de la escuela no son ASCII puro.
	for _, original := range []string{"", "hola", "Contraseña#2026", "ñÑáé 空白"} {
		guardado, err := c.Cifrar(original)
		if err != nil {
			t.Fatalf("cifrando %q: %v", original, err)
		}
		vuelto, err := c.Descifrar(guardado)
		if err != nil {
			t.Fatalf("descifrando %q: %v", original, err)
		}
		if vuelto != original {
			t.Fatalf("esperaba %q, obtuve %q", original, vuelto)
		}
	}
}

// El texto guardado no puede contener la contraseña a la vista: es el punto
// entero de cifrarla antes de que entre al volcado de `make backup`.
func TestCifrar_NoDejaElTextoALaVista(t *testing.T) {
	c := cifradorDePrueba(t)

	guardado, err := c.Cifrar("Contraseña#2026")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if strings.Contains(guardado, "Contraseña") {
		t.Fatalf("la contraseña quedó legible en %q", guardado)
	}
}

// Dos filas con la misma contraseña tienen que verse distintas en la base. Si
// no, quien mire el volcado sabe qué máquinas comparten clave sin descifrar
// nada.
func TestCifrar_DosVecesLoMismoDaTextosDistintos(t *testing.T) {
	c := cifradorDePrueba(t)

	uno, _ := c.Cifrar("la-misma")
	otro, _ := c.Cifrar("la-misma")

	if uno == otro {
		t.Fatal("dos cifrados de la misma contraseña no deberían coincidir")
	}
}

// GCM autentica además de cifrar: un valor alterado en la base falla en vez de
// descifrar a basura que la pantalla mostraría como si fuera la contraseña.
func TestDescifrar_TextoAlterado_Falla(t *testing.T) {
	c := cifradorDePrueba(t)
	guardado, _ := c.Cifrar("Contraseña#2026")

	// Se cambia el último carácter del base64.
	alterado := guardado[:len(guardado)-2] + "AA"

	if _, err := c.Descifrar(alterado); !errors.Is(err, ErrNoSePudoDescifrar) {
		t.Fatalf("esperaba ErrNoSePudoDescifrar, obtuve %v", err)
	}
}

func TestDescifrar_ConOtraClave_Falla(t *testing.T) {
	c := cifradorDePrueba(t)
	guardado, _ := c.Cifrar("Contraseña#2026")

	otro, _ := Nuevo("otro-secreto-completamente-distinto")

	// El caso de haber rotado CUENTAS_SECRET sin migrar lo guardado: las
	// contraseñas viejas no se recuperan, y el sistema lo dice en vez de
	// devolver basura.
	if _, err := otro.Descifrar(guardado); !errors.Is(err, ErrNoSePudoDescifrar) {
		t.Fatalf("esperaba ErrNoSePudoDescifrar, obtuve %v", err)
	}
}

func TestDescifrar_TextoQueNoEsBase64_Falla(t *testing.T) {
	c := cifradorDePrueba(t)

	if _, err := c.Descifrar("esto no es base64 ni ahí"); !errors.Is(err, ErrNoSePudoDescifrar) {
		t.Fatalf("esperaba ErrNoSePudoDescifrar, obtuve %v", err)
	}
}

// Sin CUENTAS_SECRET el sistema arranca igual: es una función de menos, no un
// arranque roto. Y el nil no puede explotar al usarse.
func TestSinSecreto_NoEstaDisponibleYNoRompe(t *testing.T) {
	c, err := Nuevo("")
	if err != nil {
		t.Fatalf("un secreto vacío no es un error de arranque: %v", err)
	}
	if c.Disponible() {
		t.Fatal("sin secreto no tendría que estar disponible")
	}

	if _, err := c.Cifrar("algo"); !errors.Is(err, ErrSinClave) {
		t.Fatalf("Cifrar: esperaba ErrSinClave, obtuve %v", err)
	}
	if _, err := c.Descifrar("algo"); !errors.Is(err, ErrSinClave) {
		t.Fatalf("Descifrar: esperaba ErrSinClave, obtuve %v", err)
	}
}
