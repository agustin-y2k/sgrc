package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseEstadoReserva_Validos(t *testing.T) {
	casos := map[string]EstadoReserva{"CONFIRMADA": ReservaConfirmada, "CANCELADA": ReservaCancelada, "FINALIZADA": ReservaFinalizada}
	for entrada, esperado := range casos {
		got, err := ParseEstadoReserva(entrada)
		if err != nil {
			t.Errorf("no debería fallar: %v", err)
		}
		if got != esperado {
			t.Errorf("= %q, esperaba %q", got, esperado)
		}
	}
}

func TestParseEstadoReserva_Invalido(t *testing.T) {
	_, err := ParseEstadoReserva("PENDIENTE")
	if !errors.Is(err, ErrEstadoReservaInvalido) {
		t.Fatalf("esperaba ErrEstadoReservaInvalido, obtuve %v", err)
	}
}

func TestReserva_TodasLasCombinaciones(t *testing.T) {
	estados := []EstadoReserva{ReservaConfirmada, ReservaCancelada, ReservaFinalizada}
	permitidas := map[[2]EstadoReserva]bool{
		{ReservaConfirmada, ReservaCancelada}:  true,
		{ReservaConfirmada, ReservaFinalizada}: true,
	}
	for _, desde := range estados {
		for _, hacia := range estados {
			esperado := permitidas[[2]EstadoReserva{desde, hacia}]
			got := desde.PuedeTransicionarA(hacia)
			if got != esperado {
				t.Errorf("%s -> %s = %v, esperaba %v", desde, hacia, got, esperado)
			}
		}
	}
}

func TestParseTipoReserva_Validos(t *testing.T) {
	casos := map[string]TipoReserva{"NORMAL": TipoNormal, "BLOQUEO": TipoBloqueo}
	for entrada, esperado := range casos {
		got, err := ParseTipoReserva(entrada)
		if err != nil || got != esperado {
			t.Errorf("ParseTipoReserva(%q) = %q, %v", entrada, got, err)
		}
	}
}

func TestParseTipoReserva_Invalido(t *testing.T) {
	_, err := ParseTipoReserva("RECREO")
	if !errors.Is(err, ErrTipoReservaInvalido) {
		t.Fatalf("esperaba ErrTipoReservaInvalido, obtuve %v", err)
	}
}

func TestNuevaReservaNormal_OK(t *testing.T) {
	creadoPor := "usuario1"
	r, err := NuevaReservaNormal("id1", "grupo1", "pc1", "materia1", "Ada Lovelace", &creadoPor, time.Now(), 8*time.Hour, 9*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if r.Tipo != TipoNormal || r.Estado != ReservaConfirmada {
		t.Errorf("valores iniciales incorrectos: %+v", r)
	}
	if r.ReservaGrupoID == nil || *r.ReservaGrupoID != "grupo1" {
		t.Error("ReservaGrupoID debería estar seteado para una reserva normal")
	}
}

func TestNuevaReservaNormal_RangoInvalido_Error(t *testing.T) {
	_, err := NuevaReservaNormal("id1", "grupo1", "pc1", "materia1", "Ada", nil, time.Now(), 9*time.Hour, 8*time.Hour, time.Now())
	if !errors.Is(err, ErrRangoHorarioInvalido) {
		t.Fatalf("esperaba ErrRangoHorarioInvalido, obtuve %v", err)
	}
}

func TestNuevaReservaBloqueo_OK(t *testing.T) {
	creadoPor := "admin1"
	r, err := NuevaReservaBloqueo("id1", "pc1", &creadoPor, time.Now(), 10*time.Hour, 12*time.Hour, "Jornada docente", time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if r.Tipo != TipoBloqueo {
		t.Errorf("tipo incorrecto: %s", r.Tipo)
	}
	if r.ReservaGrupoID != nil || r.MateriaID != nil {
		t.Error("un bloqueo de evaluación no debería tener ReservaGrupoID ni MateriaID")
	}
}

func TestCancelar_OK(t *testing.T) {
	r, _ := NuevaReservaBloqueo("id1", "pc1", nil, time.Now(), 10*time.Hour, 12*time.Hour, "Jornada docente", time.Now())
	canceladoPor := "admin1"
	ahora := time.Now()

	err := r.Cancelar(&canceladoPor, "PC en mantenimiento", ahora)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if r.Estado != ReservaCancelada {
		t.Errorf("estado incorrecto: %s", r.Estado)
	}
	if r.MotivoCancelacion == nil || *r.MotivoCancelacion != "PC en mantenimiento" {
		t.Error("motivo de cancelación no se guardó correctamente")
	}
	if r.CanceladaEn == nil || !r.CanceladaEn.Equal(ahora) {
		t.Error("CanceladaEn no se seteó correctamente")
	}
}

func TestCancelar_YaCancelada_Error(t *testing.T) {
	r, _ := NuevaReservaBloqueo("id1", "pc1", nil, time.Now(), 10*time.Hour, 12*time.Hour, "Jornada docente", time.Now())
	_ = r.Cancelar(nil, "primera", time.Now())

	err := r.Cancelar(nil, "segunda", time.Now())

	if !errors.Is(err, ErrTransicionReservaInvalida) {
		t.Fatalf("esperaba ErrTransicionReservaInvalida, obtuve %v", err)
	}
}

func TestFinalizar_OK(t *testing.T) {
	r, _ := NuevaReservaBloqueo("id1", "pc1", nil, time.Now(), 10*time.Hour, 12*time.Hour, "Jornada docente", time.Now())

	err := r.Finalizar()

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if r.Estado != ReservaFinalizada {
		t.Errorf("estado incorrecto: %s", r.Estado)
	}
}

func TestFinalizar_DesdeCancelada_Error(t *testing.T) {
	r, _ := NuevaReservaBloqueo("id1", "pc1", nil, time.Now(), 10*time.Hour, 12*time.Hour, "Jornada docente", time.Now())
	_ = r.Cancelar(nil, "motivo", time.Now())

	err := r.Finalizar()

	if !errors.Is(err, ErrTransicionReservaInvalida) {
		t.Fatalf("esperaba ErrTransicionReservaInvalida, obtuve %v", err)
	}
}

// ── SolapaCon — casos límite críticos (esto respalda la lógica que en la
// base implementa la constraint EXCLUDE) ──────────────────────────────

func TestSolapaCon_Solapamientos(t *testing.T) {
	r := &Reserva{HoraInicio: 8 * time.Hour, HoraFin: 10 * time.Hour}

	casos := []struct {
		nombre      string
		inicio, fin time.Duration
		esperado    bool
	}{
		{"idéntico", 8 * time.Hour, 10 * time.Hour, true},
		{"solapa al principio", 7 * time.Hour, 9 * time.Hour, true},
		{"solapa al final", 9 * time.Hour, 11 * time.Hour, true},
		{"contenido adentro", 8*time.Hour + 30*time.Minute, 9 * time.Hour, true},
		{"contiene por completo", 7 * time.Hour, 11 * time.Hour, true},
		{"justo antes, sin tocar", 6 * time.Hour, 8 * time.Hour, false},
		{"justo después, sin tocar", 10 * time.Hour, 12 * time.Hour, false},
		{"completamente antes", 5 * time.Hour, 6 * time.Hour, false},
		{"completamente después", 14 * time.Hour, 16 * time.Hour, false},
	}

	for _, c := range casos {
		got := r.SolapaCon(c.inicio, c.fin)
		if got != c.esperado {
			t.Errorf("%s: SolapaCon(%v, %v) = %v, esperaba %v", c.nombre, c.inicio, c.fin, got, c.esperado)
		}
	}
}

// ── ValidarVentanaTemporal / YaTermino ─────────────────────────────────

func TestValidarVentanaTemporal(t *testing.T) {
	// Fecha llega como medianoche UTC (parseFecha), "ahora" en la zona de la
	// escuela (UTC-3): la comparación tiene que ser de hora de pared, no de
	// instantes, o el límite del día se corre tres horas.
	escuela := time.FixedZone("ART", -3*60*60)
	ahora := time.Date(2026, 3, 9, 12, 0, 0, 0, escuela) // lunes al mediodía
	dia := func(d int) time.Time { return time.Date(2026, 3, d, 0, 0, 0, 0, time.UTC) }

	casos := []struct {
		nombre      string
		fecha       time.Time
		inicio, fin time.Duration
		esperado    error
	}{
		{"mañana", dia(10), 8 * time.Hour, 10 * time.Hour, nil},
		{"hoy más tarde", dia(9), 14 * time.Hour, 16 * time.Hour, nil},
		{"hoy, en curso ahora mismo", dia(9), 11 * time.Hour, 13 * time.Hour, nil},
		{"hoy pero ya terminó", dia(9), 8 * time.Hour, 10 * time.Hour, ErrReservaEnElPasado},
		{"termina justo ahora", dia(9), 10 * time.Hour, 12 * time.Hour, ErrReservaEnElPasado},
		{"ayer", dia(8), 14 * time.Hour, 16 * time.Hour, ErrReservaEnElPasado},
		{"años atrás", dia(1), 8 * time.Hour, 10 * time.Hour, ErrReservaEnElPasado},
		{"fin antes que inicio", dia(10), 10 * time.Hour, 8 * time.Hour, ErrRangoHorarioInvalido},
		{"duración justa en el tope", dia(10), 8 * time.Hour, 16 * time.Hour, nil},
		{"un minuto pasado el tope", dia(10), 8 * time.Hour, 16*time.Hour + time.Minute, ErrDuracionExcesiva},
		{"el día entero", dia(10), 0, 23*time.Hour + 59*time.Minute, ErrDuracionExcesiva},
	}

	for _, c := range casos {
		err := ValidarVentanaTemporal(c.fecha, c.inicio, c.fin, ahora)
		if !errors.Is(err, c.esperado) {
			t.Errorf("%s: obtuve %v, esperaba %v", c.nombre, err, c.esperado)
		}
	}
}

// ── El motivo del bloqueo ─────────────────────────────────────────

// Un bloqueo cancela las clases de otros, así que el porqué no es opcional.
// Sin motivo, quien mira el calendario y encuentra el rato ocupado no tiene
// dónde averiguar por qué, y el docente cancelado recibe un aviso hueco.
func TestNuevaReservaBloqueo_SinMotivo_Error(t *testing.T) {
	_, err := NuevaReservaBloqueo("id1", "pc1", nil, time.Now(), 10*time.Hour, 12*time.Hour, "", time.Now())
	if !errors.Is(err, ErrMotivoBloqueoVacio) {
		t.Errorf("err = %v, esperaba ErrMotivoBloqueoVacio", err)
	}
}

// Los espacios no alcanzan para pasar: si no, el CHECK de la base lo
// rechazaría como un 500 en vez de como un mensaje que se puede leer.
func TestNuevaReservaBloqueo_MotivoDeEspacios_Error(t *testing.T) {
	_, err := NuevaReservaBloqueo("id1", "pc1", nil, time.Now(), 10*time.Hour, 12*time.Hour, "   ", time.Now())
	if !errors.Is(err, ErrMotivoBloqueoVacio) {
		t.Errorf("err = %v, esperaba ErrMotivoBloqueoVacio", err)
	}
}

func TestNuevaReservaBloqueo_MotivoLargo_Error(t *testing.T) {
	largo := strings.Repeat("a", MaxLargoMotivoBloqueo+1)
	_, err := NuevaReservaBloqueo("id1", "pc1", nil, time.Now(), 10*time.Hour, 12*time.Hour, largo, time.Now())
	if !errors.Is(err, ErrMotivoBloqueoLargo) {
		t.Errorf("err = %v, esperaba ErrMotivoBloqueoLargo", err)
	}
}

// El motivo se guarda recortado y tal cual lo escribió el Admin: no se lo
// envuelve en ninguna categoría, porque el sistema no sabe de qué clase de
// cosa se trata — puede ser una evaluación, una jornada docente o una obra.
func TestNuevaReservaBloqueo_GuardaElMotivoRecortado(t *testing.T) {
	r, err := NuevaReservaBloqueo("id1", "pc1", nil, time.Now(), 10*time.Hour, 12*time.Hour,
		"  Jornada docente  ", time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if r.MotivoBloqueo != "Jornada docente" {
		t.Errorf("motivo = %q, esperaba el texto recortado", r.MotivoBloqueo)
	}
	if r.Tipo != TipoBloqueo {
		t.Errorf("tipo = %q, esperaba BLOQUEO", r.Tipo)
	}
}

// Una reserva normal NO lleva motivo: ya dice para qué es por su materia, y
// un segundo lugar donde escribir lo mismo se desincroniza solo.
func TestNuevaReservaNormal_NoLlevaMotivoDeBloqueo(t *testing.T) {
	creadoPor := "docente1"
	r, err := NuevaReservaNormal("id1", "grupo1", "pc1", "materia1", "Ada Lovelace", &creadoPor,
		time.Now(), 10*time.Hour, 12*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if r.MotivoBloqueo != "" {
		t.Errorf("motivo = %q, esperaba vacío", r.MotivoBloqueo)
	}
}
