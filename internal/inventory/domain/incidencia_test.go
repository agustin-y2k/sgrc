package domain

import (
	"errors"
	"testing"
	"time"
)

func TestParseGravedad_Validos(t *testing.T) {
	casos := map[string]Gravedad{"LEVE": GravedadLeve, "MODERADA": GravedadModerada, "GRAVE": GravedadGrave}
	for entrada, esperado := range casos {
		got, err := ParseGravedad(entrada)
		if err != nil {
			t.Errorf("ParseGravedad(%q) no debería fallar: %v", entrada, err)
		}
		if got != esperado {
			t.Errorf("ParseGravedad(%q) = %q, esperaba %q", entrada, got, esperado)
		}
	}
}

func TestParseGravedad_Invalido(t *testing.T) {
	casos := []string{"", "leve", "CRITICA"}
	for _, c := range casos {
		_, err := ParseGravedad(c)
		if !errors.Is(err, ErrGravedadInvalida) {
			t.Errorf("ParseGravedad(%q): esperaba ErrGravedadInvalida, obtuve %v", c, err)
		}
	}
}

func TestParseEstadoIncidencia_Validos(t *testing.T) {
	casos := map[string]EstadoIncidencia{
		"ABIERTA":       IncidenciaAbierta,
		"EN_REPARACION": IncidenciaEnReparacion,
		"ENVIADA_DGE":   IncidenciaEnviadaDGE,
		"RESUELTA":      IncidenciaResuelta,
	}
	for entrada, esperado := range casos {
		got, err := ParseEstadoIncidencia(entrada)
		if err != nil {
			t.Errorf("ParseEstadoIncidencia(%q) no debería fallar: %v", entrada, err)
		}
		if got != esperado {
			t.Errorf("ParseEstadoIncidencia(%q) = %q, esperaba %q", entrada, got, esperado)
		}
	}
}

func TestParseEstadoIncidencia_Invalido(t *testing.T) {
	_, err := ParseEstadoIncidencia("CERRADA")
	if !errors.Is(err, ErrEstadoIncidenciaInvalido) {
		t.Fatalf("esperaba ErrEstadoIncidenciaInvalido, obtuve %v", err)
	}
}

func TestNuevaIncidencia_OK(t *testing.T) {
	i, err := NuevaIncidencia("id1", "pc1", "usuario1", "La pantalla no enciende", "pantalla", GravedadGrave, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if i.Estado != IncidenciaAbierta {
		t.Errorf("una incidencia nueva debería arrancar ABIERTA: %s", i.Estado)
	}
	if i.ReportadoPor == nil || *i.ReportadoPor != "usuario1" {
		t.Errorf("ReportadoPor incorrecto: %v", i.ReportadoPor)
	}
}

func TestNuevaIncidencia_DescripcionVacia_Error(t *testing.T) {
	casos := []string{"", "   "}
	for _, d := range casos {
		_, err := NuevaIncidencia("id1", "pc1", "usuario1", d, "", GravedadLeve, time.Now())
		if !errors.Is(err, ErrDescripcionVacia) {
			t.Errorf("descripción %q: esperaba ErrDescripcionVacia, obtuve %v", d, err)
		}
	}
}

func TestNuevaIncidencia_SinReportadoPor_QuedaNil(t *testing.T) {
	// Caso límite: si en algún flujo no hay un usuario identificado (no
	// debería pasar en la práctica, pero el dominio no debe panickear).
	i, err := NuevaIncidencia("id1", "pc1", "", "Algo raro", "", GravedadLeve, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if i.ReportadoPor != nil {
		t.Errorf("ReportadoPor debería quedar nil con string vacío, quedó: %v", *i.ReportadoPor)
	}
}

func TestMarcarEnviadaDGE_OK(t *testing.T) {
	i, _ := NuevaIncidencia("id1", "pc1", "usuario1", "Falla", "", GravedadGrave, time.Now())
	fecha := time.Now()

	i.MarcarEnviadaDGE(fecha)

	if !i.EnviadoDGE {
		t.Error("EnviadoDGE debería quedar true")
	}
	if i.FechaEnvioDGE == nil || !i.FechaEnvioDGE.Equal(fecha) {
		t.Error("FechaEnvioDGE debería quedar seteada")
	}
	if i.Estado != IncidenciaEnviadaDGE {
		t.Errorf("estado debería quedar ENVIADA_DGE: %s", i.Estado)
	}
}
