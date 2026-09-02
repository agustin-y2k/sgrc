package application

import (
	"strings"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

func recordatorioDePrueba() eventbus.RecordatorioDeReserva {
	return eventbus.RecordatorioDeReserva{
		UsuarioID: "docente1", Email: "ada@escuela.edu.ar", Nombre: "Ada",
		MateriaNombre:   "Matemáticas",
		Fecha:           time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
		HoraInicio:      8 * time.Hour,
		Equipos:         []string{"PC 1", "PC 2", "PC 3"},
		MinutosDeGracia: 40,
	}
}

func TestListaDeEquipos(t *testing.T) {
	// "PC 1, PC 2, PC 3" se lee como una tabla; esto es una frase. Y lo que
	// se reserva puede no ser una computadora: la lista mezcla.
	casos := map[string][]string{
		"":                       {},
		"PC 1":                   {"PC 1"},
		"PC 1 y PC 2":            {"PC 1", "PC 2"},
		"PC 1, PC 2 y PC 3":      {"PC 1", "PC 2", "PC 3"},
		"PC 1 y Proyector Epson": {"PC 1", "Proyector Epson"},
	}
	for esperado, ids := range casos {
		if got := listaDeEquipos(ids); got != esperado {
			t.Errorf("con %v: %q, esperaba %q", ids, got, esperado)
		}
	}
}

// TestCorreo_Recordatorio_ExplicaLaReglaDeLosCuarentaMinutos: sin esa línea,
// liberar la reserva después se lee como que el sistema se la quitó de prepo.
func TestCorreo_Recordatorio_ExplicaLaReglaDeLosCuarentaMinutos(t *testing.T) {
	bus, enviador := mensajeroDePrueba()

	bus.Publish(eventbus.Evento{Tipo: "reserva.recordatorio", Payload: recordatorioDePrueba()})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 mail, hubo %d", len(enviador.enviados))
	}
	cuerpo := enviador.enviados[0].cuerpo
	for _, esperado := range []string{"08:00", "PC 1, PC 2 y PC 3", "Matemáticas", "40 minutos", "anulá"} {
		if !strings.Contains(cuerpo, esperado) {
			t.Errorf("el recordatorio no contiene %q:\n%s", esperado, cuerpo)
		}
	}
}

func TestCorreo_Recordatorio_SinEmailNoMandaNada(t *testing.T) {
	bus, enviador := mensajeroDePrueba()
	aviso := recordatorioDePrueba()
	aviso.Email = ""

	bus.Publish(eventbus.Evento{Tipo: "reserva.recordatorio", Payload: aviso})

	if len(enviador.enviados) != 0 {
		t.Errorf("sin dirección no hay a dónde mandarlo: %d mails", len(enviador.enviados))
	}
}

// ── Devoluciones demoradas ──────────────────────────────────────────────

// ── Corte de fin de jornada ─────────────────────────────────────────────

func TestCorreo_Cierre_DiceAQuienLeVaAFaltar(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar")

	bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.EquiposSinDevolverAlCierre{
		Equipos: []eventbus.EquipoSinDevolverAlCierre{{
			Etiqueta: "PC 3", CarroNombre: "Carro 1", Quien: "Marta",
			DesdeCuando:      time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC),
			ProximoUsuarioID: "docente2", ProximoEmail: "ada@escuela.edu.ar", ProximoNombre: "Ada",
			ProximaFecha: time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC),
			ProximaHora:  8 * time.Hour,
		}},
	}})

	// SOLO al Admin. Al docente de la próxima reserva no se le escribe acá: el
	// corte sale de noche, cuando ya no puede hacer nada, y su aviso de "tu PC
	// puede no estar" le llega una hora antes de la clase.
	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 mail (solo al Admin), hubo %d: %+v", len(enviador.enviados), enviador.enviados)
	}
	m := enviador.enviados[0]
	if m.para != "admin1@escuela.edu.ar" {
		t.Errorf("el único destinatario tiene que ser el Admin, fue %q", m.para)
	}
	// El dato accionable para el Admin es a quién le va a faltar: sin él la
	// lista es una constatación y con él es una tarea.
	if !strings.Contains(m.cuerpo, "Ada") || !strings.Contains(m.cuerpo, "11/08/2026") {
		t.Errorf("el aviso al Admin no dice a quién le va a faltar:\n%s", m.cuerpo)
	}
}

func TestCorreo_BarridoConListasVaciasNoMandaNada(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar")

	bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.EquiposSinDevolverAlCierre{}})

	if len(enviador.enviados) != 0 {
		t.Errorf("no debería mandar nada, mandó %d", len(enviador.enviados))
	}
}

// TestBarrido_LasHorasDeUnPrestamoViajanEnLaZonaDeLaEscuela: quien arma el
// texto solo formatea (ver eventbus.EquipoSinDevolverAlCierre).
func TestBarrido_LasHorasDeUnPrestamoViajanEnLaZonaDeLaEscuela(t *testing.T) {
	escuela := time.FixedZone("prueba", -3*60*60)
	salio := time.Date(2026, time.August, 10, 21, 12, 0, 0, time.UTC).In(escuela)

	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar")
	bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.EquiposSinDevolverAlCierre{
		Equipos: []eventbus.EquipoSinDevolverAlCierre{{
			Etiqueta: "PC 7", Quien: "Marta", DesdeCuando: salio,
		}},
	}})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 mail, hubo %d", len(enviador.enviados))
	}
	if !strings.Contains(enviador.enviados[0].cuerpo, "18:12") {
		t.Errorf("esperaba la hora de la escuela (18:12), salió:\n%s", enviador.enviados[0].cuerpo)
	}
}

// TestListaDeEquipos_FactorizaElCarro fija el equilibrio entre las dos cosas
// que este texto tiene que lograr a la vez: decir de qué carro es cada máquina
// —"PC 1" existe una vez por carro— sin volverse ilegible en el caso normal,
// que es una clase entera del mismo carro.
func TestListaDeEquipos_FactorizaElCarro(t *testing.T) {
	casos := []struct {
		nombre   string
		equipos  []string
		esperado string
	}{
		{
			"todos del mismo carro: se dice una vez al final",
			[]string{"PC 1 del Carro EDUTEC", "PC 2 del Carro EDUTEC", "PC 3 del Carro EDUTEC"},
			"PC 1, PC 2 y PC 3 del Carro EDUTEC",
		},
		{
			"mezclados: cada uno con el suyo, que ahí la repetición es el dato",
			[]string{"PC 1 del Carro 1", "PC 1 del Carro 2"},
			"PC 1 del Carro 1 y PC 1 del Carro 2",
		},
		{
			"un suelto entre medio corta la factorización",
			[]string{"PC 1 del Carro 1", "Proyector Epson"},
			"PC 1 del Carro 1 y Proyector Epson",
		},
		{
			"un carro con 'del' en el nombre no se parte por la mitad",
			[]string{"PC 1 del Carro del Fondo", "PC 2 del Carro del Fondo"},
			"PC 1 y PC 2 del Carro del Fondo",
		},
		{
			"un carro con acento no rompe el recorte",
			[]string{"PC 1 del Carro Ñandú", "PC 2 del Carro Ñandú"},
			"PC 1 y PC 2 del Carro Ñandú",
		},
		{"uno solo queda como está", []string{"PC 7 del Carro 2"}, "PC 7 del Carro 2"},
		{"ninguno", nil, ""},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if obtenido := listaDeEquipos(c.equipos); obtenido != c.esperado {
				t.Errorf("esperaba %q, obtuve %q", c.esperado, obtenido)
			}
		})
	}
}
