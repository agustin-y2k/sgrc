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

// TestCorreo_ReservaSinRetirar_OfreceLasSalidasQueQuedan: el aviso sale
// mientras todavía se puede hacer algo, así que tiene que decir qué.
func TestCorreo_ReservaSinRetirar_OfreceLasSalidasQueQuedan(t *testing.T) {
	bus, enviador := mensajeroDePrueba()

	bus.Publish(eventbus.Evento{Tipo: "reserva.sin-retirar", Payload: eventbus.ReservaSinRetirar{
		UsuarioID: "docente1", Email: "ada@escuela.edu.ar", Nombre: "Ada",
		MateriaNombre: "Matemáticas", HoraInicio: 8 * time.Hour,
		Equipos: []string{"PC 1", "PC 2"}, MinutosDeGracia: 40,
	}})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 mail, hubo %d", len(enviador.enviados))
	}
	cuerpo := enviador.enviados[0].cuerpo
	// El plazo es el dato accionable: dice cuánto tiempo le queda.
	if !strings.Contains(cuerpo, "40") {
		t.Errorf("tiene que decir a los cuántos minutos quedan libres:\n%s", cuerpo)
	}
	// Liberar no es prohibir: sin esta línea, el docente que llega tarde
	// asume que ya no puede usarlas y se va.
	if !strings.Contains(cuerpo, "te las entrega igual") {
		t.Errorf("falta la aclaración de que liberar no es prohibir:\n%s", cuerpo)
	}
	if !strings.Contains(cuerpo, "PC 1 y PC 2") {
		t.Errorf("tiene que nombrar las máquinas:\n%s", cuerpo)
	}
}

// El aviso NO puede estar escrito como si ya hubiera pasado: sale a los
// quince minutos justamente para que el docente todavía pueda ir, cambiar una
// máquina o cancelar.
func TestMensajeDeReservaSinRetirar_HablaEnFuturo(t *testing.T) {
	mensaje := mensajeDeReservaSinRetirar(eventbus.ReservaSinRetirar{
		MateriaNombre: "Matemáticas", HoraInicio: 8 * time.Hour,
		Equipos: []string{"PC 1", "PC 2"}, MinutosDeGracia: 40,
	})

	if !strings.Contains(mensaje, "Todavía no retiraste") {
		t.Errorf("el aviso avisa, no constata: %q", mensaje)
	}
	if strings.Contains(mensaje, "quedó libre") || strings.Contains(mensaje, "quedaron libres") {
		t.Errorf("todavía no se liberó nada: %q", mensaje)
	}
	if !strings.Contains(mensaje, "PC 1 y PC 2") {
		t.Errorf("tiene que nombrar las máquinas: %q", mensaje)
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
// para un trámite muchas veces no tiene cuenta.
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

	bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.EquiposSinDevolverAlCierre{
		Equipos: []eventbus.EquipoSinDevolverAlCierre{{
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

	bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.EquiposSinDevolverAlCierre{
		Equipos: []eventbus.EquipoSinDevolverAlCierre{{
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
	bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.EquiposSinDevolverAlCierre{}})

	if len(enviador.enviados) != 0 {
		t.Errorf("no debería mandar nada, mandó %d", len(enviador.enviados))
	}
}

// TestCorreo_Demora_DiceCuandoSalioYCuandoTeniaQueVolver: el correo a los
// Admin dice "la tiene X desde las 10:15, tenía que volver a las 11:30", y
// esas dos horas son datos distintos.
func TestCorreo_Demora_DiceCuandoSalioYCuandoTeniaQueVolver(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar")

	bus.Publish(eventbus.Evento{Tipo: "prestamo.demorado", Payload: eventbus.PrestamosDemorados{
		Prestamos: []eventbus.PrestamoDemorado{{
			Etiqueta:        "PC 7",
			Quien:           "Marta",
			EntregadoEn:     time.Date(2026, time.August, 10, 10, 15, 0, 0, time.UTC),
			DebioVolverA:    time.Date(2026, time.August, 10, 11, 30, 0, 0, time.UTC),
			MinutosDeDemora: 20,
		}},
	}})

	if len(enviador.enviados) != 1 {
		t.Fatalf("esperaba 1 mail, hubo %d", len(enviador.enviados))
	}
	cuerpo := enviador.enviados[0].cuerpo
	if !strings.Contains(cuerpo, "desde las 10:15") {
		t.Errorf("el correo no dice a qué hora salió la máquina:\n%s", cuerpo)
	}
	if !strings.Contains(cuerpo, "volver a las 11:30") {
		t.Errorf("el correo no dice a qué hora tenía que volver:\n%s", cuerpo)
	}
}

// TestBarrido_LasHorasDeUnPrestamoViajanEnLaZonaDeLaEscuela: quien arma el
// texto solo formatea (ver eventbus.PrestamoDemorado).
func TestBarrido_LasHorasDeUnPrestamoViajanEnLaZonaDeLaEscuela(t *testing.T) {
	escuela := time.FixedZone("prueba", -3*60*60)
	debioVolver := time.Date(2026, time.August, 10, 21, 12, 0, 0, time.UTC).In(escuela)

	msg := mensajeDePrestamosDemorados(eventbus.PrestamosDemorados{
		Prestamos: []eventbus.PrestamoDemorado{{
			Etiqueta: "PC 7", Quien: "Marta", DebioVolverA: debioVolver,
		}},
	})

	if !strings.Contains(msg, "18:12") {
		t.Errorf("esperaba la hora de la escuela (18:12), salió: %q", msg)
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
