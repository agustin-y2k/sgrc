package domain

import (
	"errors"
	"testing"
	"time"
)

func TestParseEstadoReservaGrupo_Validos(t *testing.T) {
	casos := map[string]EstadoReservaGrupo{
		"CONFIRMADA":             GrupoConfirmada,
		"PARCIALMENTE_CANCELADA": GrupoParcialmenteCancelada,
		"CANCELADA":              GrupoCancelada,
		"FINALIZADA":             GrupoFinalizada,
	}
	for entrada, esperado := range casos {
		got, err := ParseEstadoReservaGrupo(entrada)
		if err != nil {
			t.Errorf("ParseEstadoReservaGrupo(%q) no debería fallar: %v", entrada, err)
		}
		if got != esperado {
			t.Errorf("ParseEstadoReservaGrupo(%q) = %q, esperaba %q", entrada, got, esperado)
		}
	}
}

func TestParseEstadoReservaGrupo_Invalido(t *testing.T) {
	_, err := ParseEstadoReservaGrupo("PENDIENTE")
	if !errors.Is(err, ErrEstadoGrupoInvalido) {
		t.Fatalf("esperaba ErrEstadoGrupoInvalido, obtuve %v", err)
	}
}

// TestGrupo_TodasLasCombinaciones prueba las 16 combinaciones (4x4)
// explícitamente.
func TestGrupo_TodasLasCombinaciones(t *testing.T) {
	estados := []EstadoReservaGrupo{GrupoConfirmada, GrupoParcialmenteCancelada, GrupoCancelada, GrupoFinalizada}

	permitidas := map[[2]EstadoReservaGrupo]bool{
		{GrupoConfirmada, GrupoParcialmenteCancelada}: true,
		{GrupoConfirmada, GrupoCancelada}:             true,
		{GrupoConfirmada, GrupoFinalizada}:            true,
		{GrupoParcialmenteCancelada, GrupoCancelada}:  true,
		{GrupoParcialmenteCancelada, GrupoFinalizada}: true,
	}

	for _, desde := range estados {
		for _, hacia := range estados {
			esperado := permitidas[[2]EstadoReservaGrupo{desde, hacia}]
			got := desde.PuedeTransicionarA(hacia)
			if got != esperado {
				t.Errorf("PuedeTransicionarA: %s -> %s = %v, esperaba %v", desde, hacia, got, esperado)
			}
		}
	}
}

func TestNuevoReservaGrupo_OK(t *testing.T) {
	g, err := NuevoReservaGrupo("id1", "materia1", nil, "Ada Lovelace", time.Now(), 8*time.Hour, 10*time.Hour, nil, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if g.Estado != GrupoConfirmada {
		t.Errorf("estado inicial incorrecto: %s", g.Estado)
	}
}

func TestNuevoReservaGrupo_RangoHorarioInvalido_Error(t *testing.T) {
	_, err := NuevoReservaGrupo("id1", "materia1", nil, "Ada", time.Now(), 10*time.Hour, 8*time.Hour, nil, time.Now())
	if !errors.Is(err, ErrRangoHorarioInvalido) {
		t.Fatalf("esperaba ErrRangoHorarioInvalido, obtuve %v", err)
	}
}

func TestNuevoReservaGrupo_HorasIguales_Error(t *testing.T) {
	_, err := NuevoReservaGrupo("id1", "materia1", nil, "Ada", time.Now(), 8*time.Hour, 8*time.Hour, nil, time.Now())
	if !errors.Is(err, ErrRangoHorarioInvalido) {
		t.Fatalf("esperaba ErrRangoHorarioInvalido con horas iguales, obtuve %v", err)
	}
}

func TestCambiarEstadoGrupo_DesdeCancelada_Rechazado(t *testing.T) {
	g, _ := NuevoReservaGrupo("id1", "materia1", nil, "Ada", time.Now(), 8*time.Hour, 10*time.Hour, nil, time.Now())
	g.Estado = GrupoCancelada

	err := g.CambiarEstado(GrupoFinalizada)

	if !errors.Is(err, ErrTransicionGrupoInvalida) {
		t.Fatalf("esperaba ErrTransicionGrupoInvalida, obtuve %v", err)
	}
}
