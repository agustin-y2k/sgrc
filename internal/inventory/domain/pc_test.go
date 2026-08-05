package domain

import (
	"errors"
	"testing"
	"time"
)

func TestParseEstadoPC_Validos(t *testing.T) {
	casos := map[string]EstadoPC{
		"DISPONIBLE":        EstadoDisponible,
		"EN_MANTENIMIENTO":  EstadoEnMantenimiento,
		"FUERA_DE_SERVICIO": EstadoFueraDeServicio,
	}
	for entrada, esperado := range casos {
		got, err := ParseEstadoPC(entrada)
		if err != nil {
			t.Errorf("ParseEstadoPC(%q) no debería fallar: %v", entrada, err)
		}
		if got != esperado {
			t.Errorf("ParseEstadoPC(%q) = %q, esperaba %q", entrada, got, esperado)
		}
	}
}

func TestParseEstadoPC_Invalido(t *testing.T) {
	casos := []string{"", "disponible", "REPARANDO"}
	for _, c := range casos {
		_, err := ParseEstadoPC(c)
		if !errors.Is(err, ErrEstadoPCInvalido) {
			t.Errorf("ParseEstadoPC(%q): esperaba ErrEstadoPCInvalido, obtuve %v", c, err)
		}
	}
}

// TestPuedeTransicionarA_TodasLasCombinaciones prueba las 9 combinaciones
// (3 estados x 3 destinos) explícitamente, para que un cambio futuro no
// pueda abrir una transición no revisada sin que algún test lo note.
func TestPuedeTransicionarA_TodasLasCombinaciones(t *testing.T) {
	estados := []EstadoPC{EstadoDisponible, EstadoEnMantenimiento, EstadoFueraDeServicio}

	permitidas := map[[2]EstadoPC]bool{
		{EstadoDisponible, EstadoEnMantenimiento}:      true,
		{EstadoDisponible, EstadoFueraDeServicio}:      true,
		{EstadoEnMantenimiento, EstadoDisponible}:      true,
		{EstadoEnMantenimiento, EstadoFueraDeServicio}: true,
	}

	for _, desde := range estados {
		for _, hacia := range estados {
			esperado := permitidas[[2]EstadoPC{desde, hacia}]
			got := desde.PuedeTransicionarA(hacia)
			if got != esperado {
				t.Errorf("PuedeTransicionarA: %s -> %s = %v, esperaba %v", desde, hacia, got, esperado)
			}
		}
	}
}

func TestFueraDeServicio_EsTerminal(t *testing.T) {
	destinos := []EstadoPC{EstadoDisponible, EstadoEnMantenimiento, EstadoFueraDeServicio}
	for _, destino := range destinos {
		if EstadoFueraDeServicio.PuedeTransicionarA(destino) {
			t.Errorf("FUERA_DE_SERVICIO -> %s no debería estar permitido", destino)
		}
	}
}

func TestNuevaPC_OK(t *testing.T) {
	pc, err := NuevaPC("id1", "carro1", 27, 123456789, true, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if pc.Estado != EstadoDisponible {
		t.Errorf("una PC nueva debería arrancar DISPONIBLE: %s", pc.Estado)
	}
}

func TestNuevaPC_IdentificadorInvalido_Error(t *testing.T) {
	casos := []int{0, -1, -100}
	for _, id := range casos {
		_, err := NuevaPC("id1", "carro1", id, 123456789, false, time.Now())
		if !errors.Is(err, ErrIdentificadorInvalido) {
			t.Errorf("identificador %d: esperaba ErrIdentificadorInvalido, obtuve %v", id, err)
		}
	}
}

func TestNuevaPC_NumeroSerieInvalido_Error(t *testing.T) {
	casos := []int64{0, -1}
	for _, ns := range casos {
		_, err := NuevaPC("id1", "carro1", 1, ns, false, time.Now())
		if !errors.Is(err, ErrNumeroSerieInvalido) {
			t.Errorf("numeroSerie %d: esperaba ErrNumeroSerieInvalido, obtuve %v", ns, err)
		}
	}
}

func TestCambiarEstadoPC_TransicionValida_OK(t *testing.T) {
	pc, _ := NuevaPC("id1", "carro1", 1, 1, false, time.Now())

	err := pc.CambiarEstado(EstadoEnMantenimiento)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if pc.Estado != EstadoEnMantenimiento {
		t.Errorf("estado final incorrecto: %s", pc.Estado)
	}
}

func TestCambiarEstadoPC_DesdeFueraDeServicio_Rechazado(t *testing.T) {
	pc, _ := NuevaPC("id1", "carro1", 1, 1, false, time.Now())
	pc.Estado = EstadoFueraDeServicio

	err := pc.CambiarEstado(EstadoDisponible)

	if !errors.Is(err, ErrTransicionEstadoPCInvalida) {
		t.Fatalf("esperaba ErrTransicionEstadoPCInvalida, obtuve %v", err)
	}
	if pc.Estado != EstadoFueraDeServicio {
		t.Error("el estado no debería haber cambiado")
	}
}

func TestDarDeBaja_OK(t *testing.T) {
	pc, _ := NuevaPC("id1", "carro1", 1, 1, false, time.Now())
	ahora := time.Now()

	err := pc.DarDeBaja(ahora)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !pc.DadaDeBaja {
		t.Error("DadaDeBaja debería quedar true")
	}
	if pc.FechaBaja == nil || !pc.FechaBaja.Equal(ahora) {
		t.Error("FechaBaja debería quedar seteada")
	}
}

func TestDarDeBaja_DosVeces_Error(t *testing.T) {
	pc, _ := NuevaPC("id1", "carro1", 1, 1, false, time.Now())
	_ = pc.DarDeBaja(time.Now())

	err := pc.DarDeBaja(time.Now())

	if !errors.Is(err, ErrPCYaDadaDeBaja) {
		t.Fatalf("esperaba ErrPCYaDadaDeBaja, obtuve %v", err)
	}
}

func TestMoverACarro_OK(t *testing.T) {
	pc, _ := NuevaPC("id1", "carro1", 1, 1, false, time.Now())

	pc.MoverACarro("carro2")

	if pc.CarroID != "carro2" {
		t.Errorf("el carro no se actualizó: %s", pc.CarroID)
	}
}
