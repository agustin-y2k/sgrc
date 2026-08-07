package application

import (
	"fmt"
	"strings"

	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// Los textos de los avisos de licencias — el de la campana y el del correo.
//
// Están juntos y aparte de los demás porque comparten la parte difícil:
// decir cuándo vence algo en castellano corriente. "Vence en 1 días" es lo
// que sale si uno formatea por número, y es exactamente lo que hace que un
// aviso automático se lea como un aviso automático.

// cuandoVence describe la fecha en relación a hoy, que es como la lee una
// persona. La fecha exacta va igual entre paréntesis: "mañana" alcanza para
// entender la urgencia, pero no para anotarla en ningún lado.
func cuandoVence(l eventbus.LicenciaPorVencer) string {
	fecha := formatearFecha(l.FechaVencimiento)
	switch d := l.DiasRestantes; {
	case d > 1:
		return fmt.Sprintf("vence en %d días (%s)", d, fecha)
	case d == 1:
		return fmt.Sprintf("vence mañana (%s)", fecha)
	case d == 0:
		return fmt.Sprintf("vence hoy (%s)", fecha)
	case d == -1:
		return fmt.Sprintf("venció ayer (%s)", fecha)
	default:
		return fmt.Sprintf("venció hace %d días (%s)", -d, fecha)
	}
}

// dondeEsta ubica la licencia: la PC y el carro. Es lo que convierte el
// aviso en algo que se puede ir a resolver.
func dondeEsta(l eventbus.LicenciaPorVencer) string {
	pc := nombrePC(l.PCIdentificador)
	if l.CarroNombre == "" {
		return pc
	}
	return fmt.Sprintf("%s (%s)", pc, l.CarroNombre)
}

// mensajeDeLicencias arma el aviso de la campana.
//
// Con una sola licencia dice cuál es y dónde está, porque eso ya alcanza
// para resolverlo sin abrir nada. Con varias resume y deja el detalle para
// la pantalla: un aviso con ocho renglones adentro de la campana no se lee,
// y el tipo LICENCIA_POR_VENCER ya la enlaza con la lista completa.
func mensajeDeLicencias(a eventbus.AvisoDeLicencias) string {
	if a.Total() == 1 {
		l := append(append([]eventbus.LicenciaPorVencer{}, a.PorVencer...), a.Vencidas...)[0]
		return fmt.Sprintf("La licencia de %s de la %s %s", l.Nombre, dondeEsta(l), cuandoVence(l))
	}

	switch {
	case len(a.Vencidas) == 0:
		return fmt.Sprintf("%s están por vencer", licencias(len(a.PorVencer)))
	case len(a.PorVencer) == 0:
		return fmt.Sprintf("%s ya vencieron", licencias(len(a.Vencidas)))
	default:
		return fmt.Sprintf("%s necesitan que las renueven: %d %s y %d ya %s",
			licencias(a.Total()),
			len(a.PorVencer), plural(len(a.PorVencer), "está por vencer", "están por vencer"),
			len(a.Vencidas), plural(len(a.Vencidas), "venció", "vencieron"))
	}
}

func licencias(n int) string {
	return fmt.Sprintf("%d %s de software", n, plural(n, "licencia", "licencias"))
}

func plural(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// ══════════════════════════════════════════════════════════════════
// El correo
// ══════════════════════════════════════════════════════════════════

func (m *Mensajero) textoDeLicencias(a eventbus.AvisoDeLicencias) (asunto, cuerpo string) {
	switch {
	case len(a.Vencidas) == 0:
		asunto = "Hay licencias de software por vencer"
	case len(a.PorVencer) == 0:
		asunto = "Hay licencias de software vencidas"
	default:
		asunto = "Hay licencias de software por vencer y vencidas"
	}

	var sb strings.Builder
	sb.WriteString("Estas licencias de software necesitan que alguien las renueve:\n")
	// A diferencia de la campana, acá va el detalle completo: el correo se
	// lee sin tener el sistema abierto, y muchas veces desde el celular
	// camino a la escuela. Resumir obligaría a entrar para saber a qué
	// máquina hay que ir.
	escribirGrupo(&sb, "Por vencer", a.PorVencer)
	escribirGrupo(&sb, "Ya vencidas", a.Vencidas)

	cuerpo = sb.String()
	cuerpo += m.enlace("Podés verlas y renovarlas desde:")
	cuerpo += firma
	return asunto, cuerpo
}

func escribirGrupo(sb *strings.Builder, titulo string, licencias []eventbus.LicenciaPorVencer) {
	if len(licencias) == 0 {
		return
	}
	fmt.Fprintf(sb, "\n%s:\n", titulo)
	for _, l := range licencias {
		fmt.Fprintf(sb, "  - %s en la %s: %s\n", l.Nombre, dondeEsta(l), cuandoVence(l))
	}
}
