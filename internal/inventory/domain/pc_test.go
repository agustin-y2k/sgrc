package domain

import (
	"errors"
	"strings"
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
	pc, err := NuevaPC("id1", "carro1", 27, "5CD1234ABC", true, time.Now())
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
		_, err := NuevaPC("id1", "carro1", id, "5CD1234ABC", false, time.Now())
		if !errors.Is(err, ErrIdentificadorInvalido) {
			t.Errorf("identificador %d: esperaba ErrIdentificadorInvalido, obtuve %v", id, err)
		}
	}
}

// El número de serie es texto porque el de fábrica trae letras. Este es el
// caso que no entraba cuando la columna era BIGINT (ver migración 011), y
// es el primero que se prueba al cargar el inventario.
func TestNuevaPC_NumeroSerieConLetras_OK(t *testing.T) {
	pc, err := NuevaPC("id1", "carro1", 1, "5CD1234ABC", false, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if pc.NumeroSerie != "5CD1234ABC" {
		t.Errorf("número de serie incorrecto: %q", pc.NumeroSerie)
	}
}

// Se guarda la forma canónica y no lo que se tipeó: sin eso, la misma
// máquina cargada dos veces con distinta caja son dos filas para el UNIQUE.
func TestNuevaPC_NormalizaElNumeroDeSerie(t *testing.T) {
	pc, err := NuevaPC("id1", "carro1", 1, "  5cd1234abc  ", false, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if pc.NumeroSerie != "5CD1234ABC" {
		t.Errorf("esperaba la forma canónica en mayúsculas y sin espacios, obtuve %q", pc.NumeroSerie)
	}
}

func TestNuevaPC_NumeroSerieVacio_Error(t *testing.T) {
	// El de puros espacios importa aparte: normalizado queda vacío, y sin
	// normalizar ANTES de validar pasaría el chequeo y explotaría contra el
	// CHECK de la base con un 500.
	casos := []string{"", "   ", "\t\n"}
	for _, ns := range casos {
		_, err := NuevaPC("id1", "carro1", 1, ns, false, time.Now())
		if !errors.Is(err, ErrNumeroSerieInvalido) {
			t.Errorf("numeroSerie %q: esperaba ErrNumeroSerieInvalido, obtuve %v", ns, err)
		}
	}
}

func TestNuevaPC_NumeroSerieDemasiadoLargo_Error(t *testing.T) {
	largo := strings.Repeat("A", MaxLargoNumeroSerie+1)

	_, err := NuevaPC("id1", "carro1", 1, largo, false, time.Now())

	if !errors.Is(err, ErrNumeroSerieLargo) {
		t.Fatalf("esperaba ErrNumeroSerieLargo, obtuve %v", err)
	}
	// El tope exacto sí entra: es el VARCHAR(50) de la migración 011.
	if _, err := NuevaPC("id1", "carro1", 1, strings.Repeat("A", MaxLargoNumeroSerie), false, time.Now()); err != nil {
		t.Fatalf("el largo máximo debería entrar: %v", err)
	}
}

func TestCambiarEstadoPC_TransicionValida_OK(t *testing.T) {
	pc, _ := NuevaPC("id1", "carro1", 1, "5CD1234ABC", false, time.Now())

	err := pc.CambiarEstado(EstadoEnMantenimiento)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if pc.Estado != EstadoEnMantenimiento {
		t.Errorf("estado final incorrecto: %s", pc.Estado)
	}
}

func TestCambiarEstadoPC_DesdeFueraDeServicio_Rechazado(t *testing.T) {
	pc, _ := NuevaPC("id1", "carro1", 1, "5CD1234ABC", false, time.Now())
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
	pc, _ := NuevaPC("id1", "carro1", 1, "5CD1234ABC", false, time.Now())
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
	pc, _ := NuevaPC("id1", "carro1", 1, "5CD1234ABC", false, time.Now())
	_ = pc.DarDeBaja(time.Now())

	err := pc.DarDeBaja(time.Now())

	if !errors.Is(err, ErrPCYaDadaDeBaja) {
		t.Fatalf("esperaba ErrPCYaDadaDeBaja, obtuve %v", err)
	}
}

func TestMoverACarro_OK(t *testing.T) {
	pc, _ := NuevaPC("id1", "carro1", 1, "5CD1234ABC", false, time.Now())

	pc.MoverACarro("carro2")

	if pc.CarroID != "carro2" {
		t.Errorf("el carro no se actualizó: %s", pc.CarroID)
	}
}

// ── Equipos que no son PCs de un carro (RF-03.15) ───────────────────────

func TestNuevoEquipo_ProyectorSinCarroNiNumero(t *testing.T) {
	p, err := NuevoEquipo("eq-1", "PROYECTOR", "Proyector Epson", true, time.Now())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if p.EstaEnUnCarro() {
		t.Error("un proyector no está en ningún carro")
	}
	if p.Identificador != 0 || p.NumeroSerie != "" {
		t.Errorf("no tiene número ni serie: %+v", p)
	}
	if !p.Reservable {
		t.Error("el proyector se puede reservar")
	}
	if p.Estado != EstadoDisponible {
		t.Errorf("nace disponible, no %s", p.Estado)
	}
}

func TestNuevoEquipo_CargadorNoReservable(t *testing.T) {
	p, err := NuevoEquipo("eq-2", "CARGADOR", "Cargador 1", false, time.Now())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if p.Reservable {
		t.Error("un cargador se presta en el momento; nadie planifica con él")
	}
}

func TestNuevoEquipo_NormalizaSinTocarLaCaja(t *testing.T) {
	p, err := NuevoEquipo("eq-1", "  proyector  ", "  Proyector   Epson  ", true, time.Now())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	// Bordes recortados y espacios internos colapsados, pero la caja intacta:
	// se muestra tal como se escribió.
	if p.Tipo != "proyector" || p.Nombre != "Proyector Epson" {
		t.Errorf("tipo=%q nombre=%q", p.Tipo, p.Nombre)
	}
}

func TestNuevoEquipo_Invalidos(t *testing.T) {
	casos := []struct {
		caso     string
		tipo     string
		nombre   string
		esperado error
	}{
		{"sin tipo", "  ", "Proyector", ErrTipoEquipoVacio},
		{"sin nombre", "PROYECTOR", "   ", ErrNombreEquipoVacio},
		{"tipo larguísimo", strings.Repeat("a", MaxLargoTipoEquipo+1), "Proyector", ErrTipoEquipoLargo},
		{"nombre larguísimo", "PROYECTOR", strings.Repeat("a", MaxLargoNombreEquipo+1), ErrNombreEquipoLargo},
	}

	for _, c := range casos {
		t.Run(c.caso, func(t *testing.T) {
			if _, err := NuevoEquipo("eq-1", c.tipo, c.nombre, true, time.Now()); !errors.Is(err, c.esperado) {
				t.Errorf("esperaba %v, obtuve %v", c.esperado, err)
			}
		})
	}
}

// TestEtiqueta es lo que evita que un proyector aparezca rotulado "PC 0" en
// una pantalla o en un correo: un identificador que no existe, formateado.
func TestEtiqueta(t *testing.T) {
	deCarro, err := NuevaPC("pc-1", "carro-1", 3, "5CD1234ABC", false, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if deCarro.Etiqueta() != "PC 3" {
		t.Errorf("una PC de carro se llama por su número: %q", deCarro.Etiqueta())
	}

	suelto, err := NuevoEquipo("eq-1", "PROYECTOR", "Proyector Epson", true, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if suelto.Etiqueta() != "Proyector Epson" {
		t.Errorf("un equipo suelto se llama por su nombre: %q", suelto.Etiqueta())
	}
}

// Una PC de carro creada con NuevaPC tiene que quedar reservable y de tipo
// PC: es lo que hace que la 015 no cambie nada de lo que ya existía.
func TestNuevaPC_NaceReservableYDeTipoPC(t *testing.T) {
	p, err := NuevaPC("pc-1", "carro-1", 3, "5CD1234ABC", false, time.Now())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if p.Tipo != TipoPC || !p.Reservable || !p.EstaEnUnCarro() {
		t.Errorf("%+v", p)
	}
}
