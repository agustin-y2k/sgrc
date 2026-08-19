package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var ahoraDePrueba = time.Date(2026, time.August, 18, 10, 0, 0, 0, time.UTC)

func TestNueva_ExigeTexto(t *testing.T) {
	if _, err := Nueva("s1", "u1", TipoProblema, "   ", "/reservas", "1.9.0", ahoraDePrueba); !errors.Is(err, ErrTextoVacio) {
		t.Errorf("esperaba ErrTextoVacio, obtuve %v", err)
	}
}

func TestNueva_TopeDeLargo(t *testing.T) {
	largo := strings.Repeat("a", MaxTexto+1)
	if _, err := Nueva("s1", "u1", TipoSugerencia, largo, "", "", ahoraDePrueba); !errors.Is(err, ErrTextoLargo) {
		t.Errorf("esperaba ErrTextoLargo, obtuve %v", err)
	}
}

// La pantalla y la versión las completa la aplicación: sin ellas, un "no
// anda" obliga a ir a buscar a quien lo escribió para preguntarle qué estaba
// haciendo.
func TestNueva_GuardaDesdeDondeSeEscribio(t *testing.T) {
	s, err := Nueva("s1", "u1", TipoProblema, " no me deja reservar ", " /reservas/nueva ", "1.9.0", ahoraDePrueba)
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if s.Texto != "no me deja reservar" || s.Pantalla != "/reservas/nueva" {
		t.Errorf("no se limpiaron los espacios: %+v", s)
	}
	if s.Estado != Abierta {
		t.Errorf("una sugerencia nace abierta, no %q", s.Estado)
	}
}

// Responder y cerrar son la misma acción: una respuesta que no cierra deja
// el mensaje en la lista de pendientes para siempre, y cerrar sin responder
// es lo que hace que la próxima vez nadie escriba.
func TestResponder_CierraYRegistra(t *testing.T) {
	s := abierta(t)
	if err := s.Responder(" ya lo arreglamos ", "admin1", ahoraDePrueba); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if s.Estado != Resuelta || s.Respuesta != "ya lo arreglamos" {
		t.Errorf("esperaba resuelta con la respuesta limpia: %+v", s)
	}
	if s.RespondidaPor == nil || *s.RespondidaPor != "admin1" || s.RespondidaEn == nil {
		t.Error("quién contestó y cuándo tienen que quedar registrados")
	}
}

func TestResponder_ExigeTexto(t *testing.T) {
	s := abierta(t)
	if err := s.Responder("   ", "admin1", ahoraDePrueba); !errors.Is(err, ErrRespuestaVacia) {
		t.Errorf("esperaba ErrRespuestaVacia, obtuve %v", err)
	}
	if s.Estado != Abierta {
		t.Error("una respuesta inválida no puede cerrar el mensaje")
	}
}

// Dos Admin mirando la lista al mismo tiempo.
func TestResponder_UnaSolaVez(t *testing.T) {
	s := abierta(t)
	if err := s.Responder("listo", "admin1", ahoraDePrueba); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if err := s.Responder("otra cosa", "admin2", ahoraDePrueba); !errors.Is(err, ErrYaResuelta) {
		t.Errorf("esperaba ErrYaResuelta, obtuve %v", err)
	}
	if s.Respuesta != "listo" {
		t.Error("la primera respuesta es la que vale")
	}
}

func TestParseTipo(t *testing.T) {
	for _, valido := range []string{"SUGERENCIA", "PROBLEMA"} {
		if _, err := ParseTipo(valido); err != nil {
			t.Errorf("%q debería ser válido: %v", valido, err)
		}
	}
	if _, err := ParseTipo("QUEJA"); !errors.Is(err, ErrTipoInvalido) {
		t.Errorf("esperaba ErrTipoInvalido, obtuve %v", err)
	}
}

func abierta(t *testing.T) *Sugerencia {
	t.Helper()
	s, err := Nueva("s1", "u1", TipoProblema, "algo no anda", "/reservas", "1.9.0", ahoraDePrueba)
	if err != nil {
		t.Fatalf("armando la sugerencia de prueba: %v", err)
	}
	return s
}
