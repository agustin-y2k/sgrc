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
	// "PC 1, PC 2, PC 3" se lee como una tabla; esto es una frase. Y desde
	// la 015 lo que se reserva puede no ser una PC: la lista mezcla.
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
// liberar la reserva después se lee como que el sistema se la quitó de
// prepo. Por eso se repite en cada recordatorio y no se explica una sola vez.
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

// TestCorreo_Recordatorio_LaAdvertenciaVaAdentro: mandar dos correos por la
// misma clase es el bombardeo que se quiso evitar.
func TestCorreo_Recordatorio_LaAdvertenciaVaAdentro(t *testing.T) {
	bus, enviador := mensajeroDePrueba()
	aviso := recordatorioDePrueba()
	aviso.EquiposSinDevolver = []string{"PC 2"}

	bus.Publish(eventbus.Evento{Tipo: "reserva.recordatorio", Payload: aviso})

	if len(enviador.enviados) != 1 {
		t.Fatalf("tiene que ser UN solo mail, hubo %d", len(enviador.enviados))
	}
	if !strings.Contains(enviador.enviados[0].cuerpo, "PC 2 todavía no volvió") {
		t.Errorf("falta la advertencia:\n%s", enviador.enviados[0].cuerpo)
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

// TestCorreo_ReservaLiberada_DiceQueTodaviaLasPuedeUsar es la línea que
// evita que el docente que llegó tarde asuma que ya no puede usarlas y se
// vaya: liberar no es prohibir.
func TestCorreo_ReservaLiberada_DiceQueTodaviaLasPuedeUsar(t *testing.T) {
	bus, enviador := mensajeroDePrueba()

	bus.Publish(eventbus.Evento{Tipo: "reserva.no-retirada", Payload: eventbus.ReservasLiberadas{
		UsuarioID: "docente1", Email: "ada@escuela.edu.ar", Nombre: "Ada",
		MateriaNombre: "Matemáticas", HoraInicio: 8 * time.Hour,
		Equipos: []string{"PC 1", "PC 2"}, TodaLaReserva: true, MinutosDeGracia: 40,
	}})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 mail, hubo %d", len(enviador.enviados))
	}
	cuerpo := enviador.enviados[0].cuerpo
	if !strings.Contains(cuerpo, "no quiere decir que no las puedas usar") {
		t.Errorf("falta la aclaración de que liberar no es prohibir:\n%s", cuerpo)
	}
	if !strings.Contains(cuerpo, "40") {
		t.Errorf("tiene que decir por qué se liberó:\n%s", cuerpo)
	}
}

func TestMensajeDeReservasLiberadas_DistingueTodaLaReservaDeAlgunas(t *testing.T) {
	base := eventbus.ReservasLiberadas{
		MateriaNombre: "Matemáticas", HoraInicio: 8 * time.Hour,
		Equipos: []string{"PC 1", "PC 2"}, MinutosDeGracia: 40,
	}

	base.TodaLaReserva = true
	if !strings.Contains(mensajeDeReservasLiberadas(base), "Tu reserva de las 08:00") {
		t.Errorf("con toda la reserva: %q", mensajeDeReservasLiberadas(base))
	}

	base.TodaLaReserva = false
	completo := mensajeDeReservasLiberadas(base)
	if !strings.Contains(completo, "PC 1 y PC 2") {
		t.Errorf("con algunas tiene que nombrarlas: %q", completo)
	}
}

// ── Devoluciones demoradas ──────────────────────────────────────────────

func TestCorreo_Demora_VaALosAdminsYAQuienLaTiene(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar", "admin2@escuela.edu.ar")

	bus.Publish(eventbus.Evento{Tipo: "prestamo.demorado", Payload: eventbus.PrestamosDemorados{
		Prestamos: []eventbus.PrestamoDemorado{{
			PrestamoID: "pr1", Etiqueta: "PC 7", CarroNombre: "Carro 1",
			Quien: "Otro Docente", Email: "otro@escuela.edu.ar",
			DebioVolverA:    time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC),
			MinutosDeDemora: 25,
		}},
	}})

	// Dos Admin + la persona que la tiene.
	if len(enviador.enviados) != 3 {
		t.Fatalf("esperaba 3 mails, hubo %d", len(enviador.enviados))
	}
	var paraLaPersona *mailEnviado
	for i := range enviador.enviados {
		if enviador.enviados[i].para == "otro@escuela.edu.ar" {
			paraLaPersona = &enviador.enviados[i]
		}
	}
	if paraLaPersona == nil {
		t.Fatal("no le llegó a quien tiene la máquina")
	}
	// A quien la tiene se le habla como a un colega, no como a un deudor.
	if !strings.Contains(paraLaPersona.cuerpo, "Si ya la devolviste") {
		t.Errorf("el tono del recordatorio:\n%s", paraLaPersona.cuerpo)
	}
	if strings.Contains(paraLaPersona.cuerpo, "25 minutos") {
		t.Error("a quien la tiene no hace falta restregarle la demora")
	}
}

// TestCorreo_Demora_SinCuentaSoloAvisaALosAdmins: quien se llevó una máquina
// para un trámite muchas veces no tiene cuenta. No hay a dónde mandarle
// nada, y por eso el aviso a los Admin es el que no puede fallar.
func TestCorreo_Demora_SinCuentaSoloAvisaALosAdmins(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar")

	bus.Publish(eventbus.Evento{Tipo: "prestamo.demorado", Payload: eventbus.PrestamosDemorados{
		Prestamos: []eventbus.PrestamoDemorado{{
			PrestamoID: "pr1", Etiqueta: "PC 7", Quien: "Marta (secretaría)",
			DebioVolverA: time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC), MinutosDeDemora: 30,
		}},
	}})

	if len(enviador.enviados) != 1 || enviador.enviados[0].para != "admin1@escuela.edu.ar" {
		t.Errorf("esperaba solo el mail al Admin: %+v", enviador.enviados)
	}
	if !strings.Contains(enviador.enviados[0].cuerpo, "Marta (secretaría)") {
		t.Errorf("el Admin tiene que saber a quién buscar:\n%s", enviador.enviados[0].cuerpo)
	}
}

// ── Corte de fin de jornada ─────────────────────────────────────────────

func TestCorreo_Cierre_DiceAQuienLeVaAFaltar(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar")

	bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.PCsSinDevolverAlCierre{
		PCs: []eventbus.PCSinDevolverAlCierre{{
			Etiqueta: "PC 3", CarroNombre: "Carro 1", Quien: "Marta",
			DesdeCuando:      time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC),
			ProximoUsuarioID: "docente2", ProximoEmail: "ada@escuela.edu.ar", ProximoNombre: "Ada",
			ProximaFecha: time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC),
			ProximaHora:  8 * time.Hour,
		}},
	}})

	// Al Admin y al docente que la tiene reservada mañana.
	if len(enviador.enviados) != 2 {
		t.Fatalf("esperaba 2 mails, hubo %d", len(enviador.enviados))
	}
	var paraAdmin, paraDocente string
	for _, m := range enviador.enviados {
		if m.para == "admin1@escuela.edu.ar" {
			paraAdmin = m.cuerpo
		} else {
			paraDocente = m.cuerpo
		}
	}
	// El dato accionable para el Admin es a quién le va a faltar: sin él la
	// lista es una constatación y con él es una tarea.
	if !strings.Contains(paraAdmin, "Ada") || !strings.Contains(paraAdmin, "11/08/2026") {
		t.Errorf("el aviso al Admin no dice a quién le va a faltar:\n%s", paraAdmin)
	}
	if !strings.Contains(paraDocente, "PC 3") || !strings.Contains(paraDocente, "cambiarla por otra") {
		t.Errorf("el aviso al docente:\n%s", paraDocente)
	}
}

func TestCorreo_Cierre_SinProximaReservaSoloAvisaALosAdmins(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar")

	bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.PCsSinDevolverAlCierre{
		PCs: []eventbus.PCSinDevolverAlCierre{{
			Etiqueta: "PC 3", Quien: "Marta",
			DesdeCuando: time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC),
		}},
	}})

	if len(enviador.enviados) != 1 {
		t.Errorf("sin próxima reserva solo va a los Admin: %d mails", len(enviador.enviados))
	}
}

func TestCorreo_BarridoConListasVaciasNoMandaNada(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar")

	bus.Publish(eventbus.Evento{Tipo: "prestamo.demorado", Payload: eventbus.PrestamosDemorados{}})
	bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.PCsSinDevolverAlCierre{}})

	if len(enviador.enviados) != 0 {
		t.Errorf("no debería mandar nada, mandó %d", len(enviador.enviados))
	}
}
