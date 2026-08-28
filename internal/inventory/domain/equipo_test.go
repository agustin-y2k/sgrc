package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseEstadoEquipo_Validos(t *testing.T) {
	casos := map[string]EstadoEquipo{
		"DISPONIBLE":        EstadoDisponible,
		"EN_MANTENIMIENTO":  EstadoEnMantenimiento,
		"FUERA_DE_SERVICIO": EstadoFueraDeServicio,
	}
	for entrada, esperado := range casos {
		got, err := ParseEstadoEquipo(entrada)
		if err != nil {
			t.Errorf("ParseEstadoEquipo(%q) no debería fallar: %v", entrada, err)
		}
		if got != esperado {
			t.Errorf("ParseEstadoEquipo(%q) = %q, esperaba %q", entrada, got, esperado)
		}
	}
}

func TestParseEstadoEquipo_Invalido(t *testing.T) {
	casos := []string{"", "disponible", "REPARANDO"}
	for _, c := range casos {
		_, err := ParseEstadoEquipo(c)
		if !errors.Is(err, ErrEstadoEquipoInvalido) {
			t.Errorf("ParseEstadoEquipo(%q): esperaba ErrEstadoEquipoInvalido, obtuve %v", c, err)
		}
	}
}

// TestPuedeTransicionarA_TodasLasCombinaciones prueba las 9 combinaciones (3
// estados x 3 destinos) explícitamente, para que un cambio futuro no pueda
// abrir una transición no revisada sin que algún test lo note.
func TestPuedeTransicionarA_TodasLasCombinaciones(t *testing.T) {
	estados := []EstadoEquipo{EstadoDisponible, EstadoEnMantenimiento, EstadoFueraDeServicio}

	permitidas := map[[2]EstadoEquipo]bool{
		{EstadoDisponible, EstadoEnMantenimiento}:      true,
		{EstadoDisponible, EstadoFueraDeServicio}:      true,
		{EstadoEnMantenimiento, EstadoDisponible}:      true,
		{EstadoEnMantenimiento, EstadoFueraDeServicio}: true,
		{EstadoFueraDeServicio, EstadoDisponible}:      true,
		{EstadoFueraDeServicio, EstadoEnMantenimiento}: true,
	}

	for _, desde := range estados {
		for _, hacia := range estados {
			esperado := permitidas[[2]EstadoEquipo{desde, hacia}]
			got := desde.PuedeTransicionarA(hacia)
			if got != esperado {
				t.Errorf("PuedeTransicionarA: %s -> %s = %v, esperaba %v", desde, hacia, got, esperado)
			}
		}
	}
}

// El caso real que lo destapó: un equipo pasó a FUERA_DE_SERVICIO porque no
// tenía batería, apareció una batería, y no había forma de devolverlo a
// circulación — el botón existía y el servidor lo rechazaba siempre. Lo
// irreversible es dar de baja, no este estado.
func TestFueraDeServicio_VuelveACirculacion(t *testing.T) {
	for _, destino := range []EstadoEquipo{EstadoDisponible, EstadoEnMantenimiento} {
		if !EstadoFueraDeServicio.PuedeTransicionarA(destino) {
			t.Errorf("FUERA_DE_SERVICIO -> %s debería estar permitido: un equipo se arregla", destino)
		}
	}
	// Lo único que sigue sin ser una transición es quedarse donde está.
	if EstadoFueraDeServicio.PuedeTransicionarA(EstadoFueraDeServicio) {
		t.Error("FUERA_DE_SERVICIO -> FUERA_DE_SERVICIO no es un cambio de estado")
	}
}

func TestNuevaEquipo_OK(t *testing.T) {
	equipo, err := NuevoEquipoDeCarro("id1", "carro1", 27, "5CD1234ABC", true, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if equipo.Estado != EstadoDisponible {
		t.Errorf("una PC nueva debería arrancar DISPONIBLE: %s", equipo.Estado)
	}
}

func TestNuevaEquipo_IdentificadorInvalido_Error(t *testing.T) {
	casos := []int{0, -1, -100}
	for _, id := range casos {
		_, err := NuevoEquipoDeCarro("id1", "carro1", id, "5CD1234ABC", false, time.Now())
		if !errors.Is(err, ErrIdentificadorInvalido) {
			t.Errorf("identificador %d: esperaba ErrIdentificadorInvalido, obtuve %v", id, err)
		}
	}
}

// El número de serie es texto porque el de fábrica trae letras.
func TestNuevaEquipo_NumeroSerieConLetras_OK(t *testing.T) {
	equipo, err := NuevoEquipoDeCarro("id1", "carro1", 1, "5CD1234ABC", false, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if equipo.NumeroSerie != "5CD1234ABC" {
		t.Errorf("número de serie incorrecto: %q", equipo.NumeroSerie)
	}
}

// Se guarda la forma canónica y no lo que se tipeó: sin eso, la misma
// máquina cargada dos veces con distinta caja son dos filas para el UNIQUE.
func TestNuevaEquipo_NormalizaElNumeroDeSerie(t *testing.T) {
	equipo, err := NuevoEquipoDeCarro("id1", "carro1", 1, "  5cd1234abc  ", false, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if equipo.NumeroSerie != "5CD1234ABC" {
		t.Errorf("esperaba la forma canónica en mayúsculas y sin espacios, obtuve %q", equipo.NumeroSerie)
	}
}

func TestNuevaEquipo_NumeroSerieVacio_Error(t *testing.T) {
	// El de puros espacios importa aparte: normalizado queda vacío, y sin
	// normalizar ANTES de validar pasaría el chequeo y explotaría contra el
	// CHECK de la base con un 500.
	casos := []string{"", "   ", "\t\n"}
	for _, ns := range casos {
		_, err := NuevoEquipoDeCarro("id1", "carro1", 1, ns, false, time.Now())
		if !errors.Is(err, ErrNumeroSerieInvalido) {
			t.Errorf("numeroSerie %q: esperaba ErrNumeroSerieInvalido, obtuve %v", ns, err)
		}
	}
}

func TestNuevaEquipo_NumeroSerieDemasiadoLargo_Error(t *testing.T) {
	largo := strings.Repeat("A", MaxLargoNumeroSerie+1)

	_, err := NuevoEquipoDeCarro("id1", "carro1", 1, largo, false, time.Now())

	if !errors.Is(err, ErrNumeroSerieLargo) {
		t.Fatalf("esperaba ErrNumeroSerieLargo, obtuve %v", err)
	}
	// El tope exacto sí entra: es el VARCHAR(50) de la columna.
	if _, err := NuevoEquipoDeCarro("id1", "carro1", 1, strings.Repeat("A", MaxLargoNumeroSerie), false, time.Now()); err != nil {
		t.Fatalf("el largo máximo debería entrar: %v", err)
	}
}

func TestCambiarEstadoEquipo_TransicionValida_OK(t *testing.T) {
	equipo, _ := NuevoEquipoDeCarro("id1", "carro1", 1, "5CD1234ABC", false, time.Now())

	err := equipo.CambiarEstado(EstadoEnMantenimiento)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if equipo.Estado != EstadoEnMantenimiento {
		t.Errorf("estado final incorrecto: %s", equipo.Estado)
	}
}

// Un equipo fuera de servicio que se arregla vuelve a circulación: apareció el
// repuesto y la máquina anda. Antes esto devolvía
// ErrTransicionEstadoEquipoInvalida y la única salida era darla de baja y
// cargarla de nuevo, perdiendo su historial.
func TestCambiarEstadoEquipo_DesdeFueraDeServicio_Vuelve(t *testing.T) {
	equipo, _ := NuevoEquipoDeCarro("id1", "carro1", 1, "5CD1234ABC", false, time.Now())
	equipo.Estado = EstadoFueraDeServicio

	if err := equipo.CambiarEstado(EstadoDisponible); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if equipo.Estado != EstadoDisponible {
		t.Errorf("estado final = %s, esperaba DISPONIBLE", equipo.Estado)
	}
}

// Repetir el estado que ya se tiene sigue siendo un error, en los tres: no es
// un cambio, y dejarlo pasar convertiría un doble clic en una cascada de
// cancelaciones repetida.
func TestCambiarEstadoEquipo_AlMismoEstado_Rechazado(t *testing.T) {
	for _, estado := range []EstadoEquipo{EstadoDisponible, EstadoEnMantenimiento, EstadoFueraDeServicio} {
		equipo, _ := NuevoEquipoDeCarro("id1", "carro1", 1, "5CD1234ABC", false, time.Now())
		equipo.Estado = estado

		err := equipo.CambiarEstado(estado)

		if !errors.Is(err, ErrTransicionEstadoEquipoInvalida) {
			t.Errorf("%s -> %s: esperaba ErrTransicionEstadoEquipoInvalida, obtuve %v", estado, estado, err)
		}
	}
}

func TestDarDeBaja_OK(t *testing.T) {
	equipo, _ := NuevoEquipoDeCarro("id1", "carro1", 1, "5CD1234ABC", false, time.Now())
	ahora := time.Now()

	err := equipo.DarDeBaja(ahora)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !equipo.DadoDeBaja {
		t.Error("DadoDeBaja debería quedar true")
	}
	if equipo.FechaBaja == nil || !equipo.FechaBaja.Equal(ahora) {
		t.Error("FechaBaja debería quedar seteada")
	}
}

func TestDarDeBaja_DosVeces_Error(t *testing.T) {
	equipo, _ := NuevoEquipoDeCarro("id1", "carro1", 1, "5CD1234ABC", false, time.Now())
	_ = equipo.DarDeBaja(time.Now())

	err := equipo.DarDeBaja(time.Now())

	if !errors.Is(err, ErrEquipoYaDadoDeBaja) {
		t.Fatalf("esperaba ErrEquipoYaDadoDeBaja, obtuve %v", err)
	}
}

func TestMoverACarro_OK(t *testing.T) {
	equipo, _ := NuevoEquipoDeCarro("id1", "carro1", 1, "5CD1234ABC", false, time.Now())

	equipo.MoverACarro("carro2")

	if equipo.CarroID != "carro2" {
		t.Errorf("el carro no se actualizó: %s", equipo.CarroID)
	}
}

// ── Equipos que no son PCs de un carro (RF-03.15) ───────────────────────

func TestNuevoEquipo_ProyectorSinCarroNiNumero(t *testing.T) {
	p, err := NuevoEquipoSuelto("eq-1", "PROYECTOR", "Proyector Epson", "", true, time.Now())

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
	p, err := NuevoEquipoSuelto("eq-2", "CARGADOR", "Cargador 1", "", false, time.Now())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if p.Reservable {
		t.Error("un cargador se presta en el momento; nadie planifica con él")
	}
}

func TestNuevoEquipo_NormalizaSinTocarLaCaja(t *testing.T) {
	p, err := NuevoEquipoSuelto("eq-1", "  proyector  ", "  Proyector   Epson  ", "", true, time.Now())

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
			if _, err := NuevoEquipoSuelto("eq-1", c.tipo, c.nombre, "", true, time.Now()); !errors.Is(err, c.esperado) {
				t.Errorf("esperaba %v, obtuve %v", c.esperado, err)
			}
		})
	}
}

// TestEtiqueta es lo que evita que un proyector aparezca rotulado "PC 0" en
// una pantalla o en un correo: un identificador que no existe, formateado.
func TestEtiqueta(t *testing.T) {
	deCarro, err := NuevoEquipoDeCarro("equipo-1", "carro-1", 3, "5CD1234ABC", false, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if deCarro.Etiqueta() != "PC 3" {
		t.Errorf("una PC de carro se llama por su número: %q", deCarro.Etiqueta())
	}

	suelto, err := NuevoEquipoSuelto("eq-1", "PROYECTOR", "Proyector Epson", "", true, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if suelto.Etiqueta() != "Proyector Epson" {
		t.Errorf("un equipo suelto se llama por su nombre: %q", suelto.Etiqueta())
	}
}

// Una PC de carro creada con NuevoEquipoDeCarro tiene que quedar reservable y de tipo
// PC: es lo que hace que sumar equipos sueltos no cambie nada de lo que ya existía.
func TestNuevaEquipo_NaceReservableYDeTipoEquipo(t *testing.T) {
	p, err := NuevoEquipoDeCarro("equipo-1", "carro-1", 3, "5CD1234ABC", false, time.Now())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if p.Tipo != TipoPC || !p.Reservable || !p.EstaEnUnCarro() {
		t.Errorf("%+v", p)
	}
}

// ── El número de serie opcional de un equipo suelto ──────────────────────
//
// Vale para cualquier tipo, no solo para las notebooks: un proyector tiene
// serie y se extravía, un cargador no tiene ninguna. Por eso es un campo que
// se llena o no, y no dos categorías de equipo.

func TestNuevoEquipoSuelto_ConNumeroDeSerie_LoGuardaNormalizado(t *testing.T) {
	p, err := NuevoEquipoSuelto("eq-1", "NOTEBOOK", "Notebook Dirección", "  abc-123x  ", true, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Misma forma canónica que en una computadora de carro —mayúsculas y sin
	// bordes—, para que la misma máquina no pueda entrar dos veces con
	// distinta caja y esquivar el UNIQUE de la columna.
	if p.NumeroSerie != "ABC-123X" {
		t.Fatalf("esperaba ABC-123X, obtuve %q", p.NumeroSerie)
	}
}

func TestNuevoEquipoSuelto_SinNumeroDeSerie_NoEsError(t *testing.T) {
	// El caso del cargador: no tiene serie y no hay ninguna que inventar.
	for _, entrada := range []string{"", "   "} {
		p, err := NuevoEquipoSuelto("eq-1", "CARGADOR", "Cargador 1", entrada, false, time.Now())
		if err != nil {
			t.Fatalf("con %q no debería fallar: %v", entrada, err)
		}
		// Vacío y no espacios: el repositorio lo guarda como NULL, y la columna
		// tiene un CHECK que rechaza la cadena vacía.
		if p.NumeroSerie != "" {
			t.Fatalf("con %q esperaba vacío, obtuve %q", entrada, p.NumeroSerie)
		}
	}
}

func TestNuevoEquipoSuelto_NumeroDeSerieDemasiadoLargo(t *testing.T) {
	largo := strings.Repeat("A", MaxLargoNumeroSerie+1)

	if _, err := NuevoEquipoSuelto("eq-1", "NOTEBOOK", "Notebook 1", largo, true, time.Now()); !errors.Is(err, ErrNumeroSerieLargo) {
		t.Fatalf("esperaba ErrNumeroSerieLargo, obtuve %v", err)
	}
}

// El equipo de carro sigue exigiéndolo: que sea opcional afuera no relaja la
// regla adentro del laboratorio.
func TestNuevoEquipoDeCarro_SigueExigiendoNumeroDeSerie(t *testing.T) {
	if _, err := NuevoEquipoDeCarro("eq-1", "carro-1", 3, "", false, time.Now()); !errors.Is(err, ErrNumeroSerieInvalido) {
		t.Fatalf("esperaba ErrNumeroSerieInvalido, obtuve %v", err)
	}
}

// TestEstadoEquipo_Legible fija los textos que ve un docente. El valor crudo
// del enum llegaba tal cual al buzón y al correo: "el equipo pasó a
// FUERA_DE_SERVICIO" en un aviso que además trae una mala noticia.
func TestEstadoEquipo_Legible(t *testing.T) {
	casos := map[EstadoEquipo]string{
		EstadoDisponible:      "disponible",
		EstadoEnMantenimiento: "en mantenimiento",
		EstadoFueraDeServicio: "fuera de servicio",
	}
	for estado, esperado := range casos {
		if obtenido := estado.Legible(); obtenido != esperado {
			t.Errorf("%s: esperaba %q, obtuve %q", estado, esperado, obtenido)
		}
	}

	// La frase donde se usa tiene que poder leerse con los tres. "pasó a en
	// mantenimiento" es lo que obliga a que el verbo sea "quedó".
	for estado := range casos {
		frase := "el equipo quedó " + estado.Legible()
		if strings.Contains(frase, "_") || strings.ToUpper(frase) == frase {
			t.Errorf("la frase quedó con el enum adentro: %q", frase)
		}
	}
}
