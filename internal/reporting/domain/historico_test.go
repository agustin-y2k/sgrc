package domain

import (
	"errors"
	"testing"
)

func TestNuevoHistoricoUsoEquipo_OK(t *testing.T) {
	h, err := NuevoHistoricoUsoEquipo("id1", 2026, "pc1", "PC 27", 27, "Carro 1", 900, 12)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if h.CantidadReservas != 12 || h.MinutosReservados != 900 || h.IdentificadorSnapshot != 27 {
		t.Errorf("valores incorrectos: %+v", h)
	}
}

func TestNuevoHistoricoUsoEquipo_MinutosNegativos_Error(t *testing.T) {
	_, err := NuevoHistoricoUsoEquipo("id1", 2026, "pc1", "PC 27", 27, "Carro 1", -30, 5)
	if !errors.Is(err, ErrValorNegativo) {
		t.Fatalf("esperaba ErrValorNegativo, obtuve %v", err)
	}
}

func TestNuevoHistoricoUsoEquipo_CantidadNegativa_Error(t *testing.T) {
	_, err := NuevoHistoricoUsoEquipo("id1", 2026, "pc1", "PC 27", 27, "Carro 1", 30, -1)
	if !errors.Is(err, ErrValorNegativo) {
		t.Fatalf("esperaba ErrValorNegativo, obtuve %v", err)
	}
}

func TestNuevoHistoricoUsoEquipo_Cero_OK(t *testing.T) {
	// Caso límite: una PC que existía pero nunca se reservó en todo el
	// año — cantidad y minutos en cero, no debería ser un error.
	h, err := NuevoHistoricoUsoEquipo("id1", 2026, "pc1", "PC 27", 27, "Carro 1", 0, 0)
	if err != nil {
		t.Fatalf("cero no debería ser un error: %v", err)
	}
	if h.CantidadReservas != 0 || h.MinutosReservados != 0 {
		t.Errorf("valores incorrectos: %+v", h)
	}
}

func TestNuevoHistoricoUsoDocente_OK(t *testing.T) {
	usuarioID := "usuario1"
	h, err := NuevoHistoricoUsoDocente("id1", 2026, &usuarioID, "Ada Lovelace", 8, 720)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if h.UsuarioID == nil || *h.UsuarioID != "usuario1" {
		t.Errorf("UsuarioID incorrecto: %v", h.UsuarioID)
	}
	if h.NombreDocenteSnapshot != "Ada Lovelace" {
		t.Errorf("nombre incorrecto: %s", h.NombreDocenteSnapshot)
	}
}

func TestNuevoHistoricoUsoDocente_SinUsuarioID_QuedaNil(t *testing.T) {
	// Caso legítimo: el docente ya fue eliminado definitivamente
	// (SET NULL) — el histórico se conserva solo con el snapshot del
	// nombre.
	h, err := NuevoHistoricoUsoDocente("id1", 2026, nil, "Ada Lovelace", 8, 720)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if h.UsuarioID != nil {
		t.Errorf("esperaba UsuarioID nil, obtuve %v", *h.UsuarioID)
	}
}

func TestNuevoHistoricoUsoDocente_CantidadNegativa_Error(t *testing.T) {
	_, err := NuevoHistoricoUsoDocente("id1", 2026, nil, "Ada", -3, 100)
	if !errors.Is(err, ErrValorNegativo) {
		t.Fatalf("esperaba ErrValorNegativo, obtuve %v", err)
	}
}

func TestNuevoHistoricoUsoDocente_MinutosNegativos_Error(t *testing.T) {
	_, err := NuevoHistoricoUsoDocente("id1", 2026, nil, "Ada", 3, -100)
	if !errors.Is(err, ErrValorNegativo) {
		t.Fatalf("esperaba ErrValorNegativo, obtuve %v", err)
	}
}
