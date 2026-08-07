package application

import (
	"strings"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

func licenciaDelAviso(nombre string, pc int, carro string, diasRestantes int) eventbus.LicenciaPorVencer {
	return eventbus.LicenciaPorVencer{
		Nombre:           nombre,
		PCIdentificador:  pc,
		CarroNombre:      carro,
		FechaVencimiento: time.Date(2026, time.September, 3, 0, 0, 0, 0, time.UTC),
		DiasRestantes:    diasRestantes,
	}
}

// TestCuandoVence_SeLeeComoLoDiriaUnaPersona: "vence en 1 días" es lo que
// sale de formatear por número, y es exactamente lo que hace que un aviso
// automático se lea como un aviso automático.
func TestCuandoVence_SeLeeComoLoDiriaUnaPersona(t *testing.T) {
	casos := []struct {
		dias     int
		esperado string
	}{
		{7, "vence en 7 días (03/09/2026)"},
		{2, "vence en 2 días (03/09/2026)"},
		{1, "vence mañana (03/09/2026)"},
		{0, "vence hoy (03/09/2026)"},
		{-1, "venció ayer (03/09/2026)"},
		{-6, "venció hace 6 días (03/09/2026)"},
	}

	for _, c := range casos {
		if got := cuandoVence(licenciaDelAviso("AutoCAD 2027", 3, "Carro 1", c.dias)); got != c.esperado {
			t.Errorf("con %d días: %q, esperaba %q", c.dias, got, c.esperado)
		}
	}
}

func TestMensajeDeLicencias_UnaSola(t *testing.T) {
	// Con una sola, el aviso de la campana ya alcanza para resolverlo sin
	// abrir nada: dice cuál es, dónde está y cuándo vence.
	aviso := eventbus.AvisoDeLicencias{
		PorVencer: []eventbus.LicenciaPorVencer{licenciaDelAviso("AutoCAD 2027", 3, "Carro 1", 1)},
	}

	got := mensajeDeLicencias(aviso)

	esperado := "La licencia de AutoCAD 2027 de la PC 3 (Carro 1) vence mañana (03/09/2026)"
	if got != esperado {
		t.Errorf("mensaje = %q\nesperaba  = %q", got, esperado)
	}
}

func TestMensajeDeLicencias_UnaSolaVencida(t *testing.T) {
	aviso := eventbus.AvisoDeLicencias{
		Vencidas: []eventbus.LicenciaPorVencer{licenciaDelAviso("AutoCAD 2027", 3, "Carro 1", -1)},
	}

	got := mensajeDeLicencias(aviso)

	esperado := "La licencia de AutoCAD 2027 de la PC 3 (Carro 1) venció ayer (03/09/2026)"
	if got != esperado {
		t.Errorf("mensaje = %q\nesperaba  = %q", got, esperado)
	}
}

func TestMensajeDeLicencias_VariasResumeYNoEnumera(t *testing.T) {
	// Ocho renglones adentro de la campana no se leen: el detalle está en
	// la pantalla, que el tipo LICENCIA_POR_VENCER ya enlaza.
	casos := []struct {
		nombre    string
		porVencer int
		vencidas  int
		esperado  string
	}{
		{
			nombre: "solo por vencer", porVencer: 8, vencidas: 0,
			esperado: "8 licencias de software están por vencer",
		},
		{
			nombre: "solo vencidas", porVencer: 0, vencidas: 3,
			esperado: "3 licencias de software ya vencieron",
		},
		{
			nombre: "mezcla", porVencer: 2, vencidas: 3,
			esperado: "5 licencias de software necesitan que las renueven: 2 están por vencer y 3 ya vencieron",
		},
		{
			nombre: "mezcla con singulares", porVencer: 1, vencidas: 1,
			esperado: "2 licencias de software necesitan que las renueven: 1 está por vencer y 1 ya venció",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var aviso eventbus.AvisoDeLicencias
			for i := 0; i < c.porVencer; i++ {
				aviso.PorVencer = append(aviso.PorVencer, licenciaDelAviso("AutoCAD 2027", i+1, "Carro 1", 1))
			}
			for i := 0; i < c.vencidas; i++ {
				aviso.Vencidas = append(aviso.Vencidas, licenciaDelAviso("SolidWorks", i+1, "Carro 1", -2))
			}

			if got := mensajeDeLicencias(aviso); got != c.esperado {
				t.Errorf("mensaje = %q\nesperaba  = %q", got, c.esperado)
			}
		})
	}
}

func TestMensajeDeLicencias_SinCarroNoDejaParentesisVacio(t *testing.T) {
	// El nombre del carro sale de un JOIN; si por lo que sea viene vacío,
	// el aviso tiene que salir igual y no con un "(...)" colgado.
	aviso := eventbus.AvisoDeLicencias{
		PorVencer: []eventbus.LicenciaPorVencer{licenciaDelAviso("AutoCAD 2027", 3, "", 1)},
	}

	got := mensajeDeLicencias(aviso)

	if strings.Contains(got, "()") {
		t.Errorf("quedó un paréntesis vacío: %q", got)
	}
	if !strings.Contains(got, "PC 3") {
		t.Errorf("se perdió la PC: %q", got)
	}
}

// ── El correo ───────────────────────────────────────────────────────────

func TestCorreo_Licencias_LlegaATodosLosAdminsConElDetalle(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar", "admin2@escuela.edu.ar")

	bus.Publish(eventbus.Evento{
		Tipo: "licencia.por-vencer",
		Payload: eventbus.AvisoDeLicencias{
			PorVencer: []eventbus.LicenciaPorVencer{licenciaDelAviso("AutoCAD 2027", 3, "Carro 1", 1)},
			Vencidas:  []eventbus.LicenciaPorVencer{licenciaDelAviso("SolidWorks", 5, "Carro 2", -6)},
		},
	})

	if len(enviador.enviados) != 2 {
		t.Fatalf("esperaba 2 mails (uno por Admin), hubo %d", len(enviador.enviados))
	}

	mail := enviador.enviados[0]
	if !strings.Contains(mail.asunto, "por vencer y vencidas") {
		t.Errorf("el asunto no dice que hay de las dos: %q", mail.asunto)
	}
	// A diferencia de la campana, el correo sí enumera: se lee sin tener el
	// sistema abierto, muchas veces camino a la escuela.
	for _, esperado := range []string{
		"Por vencer", "AutoCAD 2027 en la PC 3 (Carro 1)", "vence mañana",
		"Ya vencidas", "SolidWorks en la PC 5 (Carro 2)", "venció hace 6 días",
		urlDePrueba,
	} {
		if !strings.Contains(mail.cuerpo, esperado) {
			t.Errorf("el cuerpo no contiene %q:\n%s", esperado, mail.cuerpo)
		}
	}
}

func TestCorreo_Licencias_AsuntoSegunElContenido(t *testing.T) {
	casos := []struct {
		nombre   string
		aviso    eventbus.AvisoDeLicencias
		esperado string
	}{
		{
			nombre:   "solo por vencer",
			aviso:    eventbus.AvisoDeLicencias{PorVencer: []eventbus.LicenciaPorVencer{licenciaDelAviso("AutoCAD", 1, "Carro 1", 1)}},
			esperado: "Hay licencias de software por vencer",
		},
		{
			nombre:   "solo vencidas",
			aviso:    eventbus.AvisoDeLicencias{Vencidas: []eventbus.LicenciaPorVencer{licenciaDelAviso("AutoCAD", 1, "Carro 1", -1)}},
			esperado: "Hay licencias de software vencidas",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar")
			bus.Publish(eventbus.Evento{Tipo: "licencia.por-vencer", Payload: c.aviso})

			if len(enviador.enviados) != 1 {
				t.Fatalf("esperaba 1 mail, hubo %d", len(enviador.enviados))
			}
			if enviador.enviados[0].asunto != c.esperado {
				t.Errorf("asunto = %q, esperaba %q", enviador.enviados[0].asunto, c.esperado)
			}
			// Un solo grupo: el título del otro no tiene que aparecer.
			cuerpo := enviador.enviados[0].cuerpo
			noEsperado := "Ya vencidas"
			if c.nombre == "solo vencidas" {
				noEsperado = "Por vencer"
			}
			if strings.Contains(cuerpo, noEsperado) {
				t.Errorf("apareció el título %q de un grupo vacío:\n%s", noEsperado, cuerpo)
			}
		})
	}
}

// TestCorreo_Licencias_AvisoVacioNoMandaNada: el job no publica si no hay
// nada, pero el handler no puede confiar en eso — un aviso vacío mandaría
// un mail que dice que hay licencias por vencer y no lista ninguna.
func TestCorreo_Licencias_AvisoVacioNoMandaNada(t *testing.T) {
	bus, enviador := mensajeroDePrueba("admin1@escuela.edu.ar")

	bus.Publish(eventbus.Evento{Tipo: "licencia.por-vencer", Payload: eventbus.AvisoDeLicencias{}})

	if len(enviador.enviados) != 0 {
		t.Errorf("no debería mandar nada, mandó %d", len(enviador.enviados))
	}
}
