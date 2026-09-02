package domain

import (
	"errors"
	"testing"
)

func TestParseCategoriaEmail_LasQueExistenYNadaMas(t *testing.T) {
	for _, c := range CategoriasDeEmail() {
		parseada, err := ParseCategoriaEmail(string(c))
		if err != nil {
			t.Errorf("%s debería ser válida: %v", c, err)
		}
		if parseada != c {
			t.Errorf("esperaba %s, obtuve %s", c, parseada)
		}
	}

	// Las dos listas se parecen y algunos nombres coinciden a propósito
	// (RESERVA_CANCELADA es un Tipo y también una categoría), pero no son la
	// misma: hay Tipos que no tienen correo, y categorías que no tienen Tipo.
	for _, invalida := range []string{"", "GENERAL", "DOCENTE_PENDIENTE", "cuenta_pendiente"} {
		if _, err := ParseCategoriaEmail(invalida); !errors.Is(err, ErrCategoriaEmailInvalida) {
			t.Errorf("%q debería ser inválida, obtuve %v", invalida, err)
		}
	}
}

// Qué ve cada rol. Un docente no recibe los avisos que van a todos los Admin,
// así que tampoco tiene una casilla para ellos — pero sí ve los de su cuenta.
func TestCategoriasPara_ElDocenteVeLasSuyasYLasDeLaCuenta(t *testing.T) {
	delDocente := CategoriasPara(false)
	for _, c := range delDocente {
		if c.Grupo() == GrupoAdministracion {
			t.Errorf("%s no debería estar en el panel de un docente", c)
		}
	}
	// Contadas y no un número fijo: retirar o agregar una categoría de
	// administración no tiene por qué romper este test, que afirma otra cosa.
	deAdministracion := 0
	for _, c := range CategoriasDeEmail() {
		if c.Grupo() == GrupoAdministracion {
			deAdministracion++
		}
	}
	if len(delDocente) != len(CategoriasDeEmail())-deAdministracion {
		t.Errorf("esperaba las de cuenta y las personales, obtuve %v", delDocente)
	}

	delAdmin := CategoriasPara(true)
	if len(delAdmin) != len(CategoriasDeEmail()) {
		t.Errorf("el Admin debería ver todas: esperaba %d, obtuve %d",
			len(CategoriasDeEmail()), len(delAdmin))
	}
	if len(delDocente) >= len(delAdmin) {
		t.Errorf("el docente debería ver menos casillas que el Admin: %d vs %d",
			len(delDocente), len(delAdmin))
	}
}

// El default no es un detalle de la pantalla: decide qué le llega a quien
// nunca abrió el panel, que es el estado de casi todos.
//
// Desde la 1.18.0 la respuesta es "casi nada": los cuatro correos fijos y una
// sola categoría más. Todo lo demás se enciende si esa persona lo pide.
func TestActivaPorDefecto_SoloLasFijasYCuentasPendientes(t *testing.T) {
	encendidas := map[CategoriaEmail]bool{
		CatRecuperacionDeCuenta: true,
		CatCuentaAprobada:       true,
		CatSoporte:              true,
		CatSoporteRespondido:    true,
		// La única excepción, y no por quien la recibe sino por quien la
		// sufre: un docente que no puede entrar hasta que un Admin lo mire.
		CatCuentaPendiente: true,
	}

	for _, c := range CategoriasDeEmail() {
		if c.ActivaPorDefecto() != encendidas[c] {
			t.Errorf("%s: por defecto debería estar %v", c, encendidas[c])
		}
	}
}

// Ninguna categoría que se pueda apagar viene encendida, salvo la de las
// cuentas pendientes. Es la misma regla dicha al revés, y está aparte porque
// es la que hay que releer cuando alguien agregue una categoría nueva: el
// default de lo nuevo es APAGADO, salvo que haya un tercero esperando.
func TestActivaPorDefecto_NingunaConfigurableVieneEncendida(t *testing.T) {
	for _, c := range CategoriasDeEmail() {
		if c.EsFija() || c == CatCuentaPendiente {
			continue
		}
		if c.ActivaPorDefecto() {
			t.Errorf("%s no debería arrancar encendida: nadie la pidió", c)
		}
	}
}

func TestEfectivasPara_LoElegidoGanaSobreElDefault(t *testing.T) {
	// Sin haber elegido nada, un docente recibe SOLO las tres fijas que le
	// corresponden: los dos correos de su cuenta y la respuesta de soporte.
	activas := EfectivasPara(nil, false)
	if len(activas) != 3 {
		t.Fatalf("esperaba solo las tres fijas del docente, obtuve %v", activas)
	}
	for _, c := range activas {
		if !c.EsFija() {
			t.Errorf("%s no es fija y no debería salir sin que nadie la pida", c)
		}
	}

	// Y encender una tiene que poder encenderla de verdad.
	elegidas := map[CategoriaEmail]bool{
		CatReservaCancelada:      true,
		CatRecordatorioDeReserva: true,
	}
	activas = EfectivasPara(elegidas, false)
	if len(activas) != 5 {
		t.Fatalf("esperaba las tres fijas más las dos elegidas, obtuve %v", activas)
	}
}

// Destildar todo no apaga las fijas —los de la cuenta y los de soporte—: no
// hay forma de pedir eso y el sistema no la ofrece.
func TestEfectivasPara_LasFijasSobrevivenATodoApagado(t *testing.T) {
	apagadas := map[CategoriaEmail]bool{}
	for _, c := range CategoriasDeEmail() {
		apagadas[c] = false
	}

	activas := EfectivasPara(apagadas, true)
	if len(activas) != 4 {
		t.Fatalf("esperaba solo las cuatro fijas, obtuve %v", activas)
	}
	for _, c := range activas {
		if !c.EsFija() {
			t.Errorf("%s no es fija y quedó encendida", c)
		}
	}
}

// Un docente ve las suyas y punto: aunque la base tuviera una categoría de
// administración encendida a su nombre, no se le muestra ni se le cuenta.
func TestEfectivasPara_NoDevuelveLasDeAdminAUnDocente(t *testing.T) {
	activas := EfectivasPara(map[CategoriaEmail]bool{CatSugerencia: true}, false)
	for _, c := range activas {
		if c == CatSugerencia {
			t.Fatalf("le devolvió una categoría de administración: %v", activas)
		}
	}
}

func TestDecisiones_SonExplicitasYSoloSobreLoQueLeToca(t *testing.T) {
	// Un docente que tilda una sola cosa se pronuncia sobre las ocho suyas...
	decisiones := Decisiones([]CategoriaEmail{CatRecordatorioDeReserva, CatRecordatorioDeReserva}, false)
	if len(decisiones) != len(Configurables(false)) {
		t.Fatalf("esperaba una decisión por categoría configurable, obtuve %v", decisiones)
	}
	if !decisiones[CatRecordatorioDeReserva] {
		t.Error("la que tildó tendría que quedar encendida")
	}
	if decisiones[CatReservaCancelada] != false {
		t.Error("la que no tildó tendría que quedar apagada, no ausente")
	}

	// ...y sobre ninguna de administración: el día que lo asciendan, esas
	// tienen que empezar con su valor por defecto y no en false. Tampoco sobre
	// las de la cuenta, que no tienen nada que decidir.
	for c := range decisiones {
		if c.Grupo() == GrupoAdministracion {
			t.Errorf("le guardó una decisión que no es suya: %s", c)
		}
		if c.EsFija() {
			t.Errorf("guardó una decisión sobre un correo que sale siempre: %s", c)
		}
	}
}

// Nadie puede elegir sobre las fijas, ni siquiera un Admin. Las de soporte
// están acá por una razón distinta a las de la cuenta —del otro lado hay
// alguien esperando— pero el resultado es el mismo.
func TestPuedeElegir_LasFijasNoSeTocan(t *testing.T) {
	for _, c := range []CategoriaEmail{
		CatRecuperacionDeCuenta, CatCuentaAprobada, CatSoporte, CatSoporteRespondido,
	} {
		for _, esAdmin := range []bool{true, false} {
			if c.PuedeElegir(esAdmin) {
				t.Errorf("%s no debería poder elegirse (esAdmin=%t)", c, esAdmin)
			}
		}
	}
}
