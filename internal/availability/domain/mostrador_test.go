package domain

import (
	"testing"
	"time"
)

func hs(h int) time.Duration { return time.Duration(h) * time.Hour }

func bloque(dia DiaSemana, desde, hasta int) *BloqueHorario {
	return &BloqueHorario{ID: "b", UsuarioID: "u", DiaSemana: dia, HoraInicio: hs(desde), HoraFin: hs(hasta)}
}

func TestTramosDelDia_SaleDelPatronSemanal(t *testing.T) {
	bloques := []*BloqueHorario{
		bloque(Lunes, 8, 12),
		bloque(Lunes, 14, 18),
		bloque(Martes, 8, 12),
	}

	tramos := TramosDelDia(bloques, nil, Lunes)

	if len(tramos) != 2 {
		t.Fatalf("esperaba los dos tramos del lunes, obtuve %+v", tramos)
	}
	if !CubreLaHora(tramos, hs(9)) {
		t.Error("las 9 caen en el primer tramo")
	}
	if CubreLaHora(tramos, hs(13)) {
		t.Error("las 13 son el hueco del mediodía: no hay nadie")
	}
	if !CubreLaHora(tramos, hs(15)) {
		t.Error("las 15 caen en el segundo tramo")
	}
}

// El límite exacto de fin NO cuenta, igual que en el resto del sistema.
func TestCubreLaHora_ElFinNoEstaIncluido(t *testing.T) {
	tramos := TramosDelDia([]*BloqueHorario{bloque(Lunes, 8, 12)}, nil, Lunes)

	if !CubreLaHora(tramos, hs(8)) {
		t.Error("el inicio sí cuenta")
	}
	if CubreLaHora(tramos, hs(12)) {
		t.Error("el fin no cuenta: a las 12 en punto ya se fue")
	}
}

// Declarar la ausencia es el caso que motivó todo esto: el Admin sabe que
// falta y lo deja anotado, y ese día el sistema no concluye nada.
func TestTramosDelDia_LaAusenciaDeclaradaPisaElPatron(t *testing.T) {
	bloques := []*BloqueHorario{bloque(Lunes, 8, 18)}
	ausente := &Excepcion{Tipo: NoDisponible}

	if tramos := TramosDelDia(bloques, ausente, Lunes); len(tramos) != 0 {
		t.Errorf("declaró que no viene: no atiende nada ese día, obtuve %+v", tramos)
	}
}

// Y el horario modificado también pisa: el Admin que ese día entra más tarde.
func TestTramosDelDia_ElHorarioModificadoPisaElPatron(t *testing.T) {
	bloques := []*BloqueHorario{bloque(Lunes, 8, 18)}
	desde, hasta := hs(14), hs(18)
	modificado := &Excepcion{Tipo: HorarioModificado, HoraInicio: &desde, HoraFin: &hasta}

	tramos := TramosDelDia(bloques, modificado, Lunes)

	if CubreLaHora(tramos, hs(9)) {
		t.Error("ese día entra a las 14: a las 9 no está, aunque su patrón diga que sí")
	}
	if !CubreLaHora(tramos, hs(15)) {
		t.Error("a las 15 sí está")
	}
}

// DeclaroHorario separa "no hay nadie ahora" de "acá nadie declaró nunca
// nada". Sin esa distinción, desplegar esta versión apagaría el barrido solo
// y en silencio en cualquier instalación sin horarios cargados.
func TestDeclaroHorario_DistingueElSinConfigurarDelSinNadie(t *testing.T) {
	if DeclaroHorario(nil) {
		t.Error("sin bloques no hay ninguna declaración")
	}
	// Un bloque de otro día igual es una declaración: esa persona cargó su
	// horario, y que hoy no le toque es una respuesta, no un silencio.
	if !DeclaroHorario([]*BloqueHorario{bloque(Martes, 8, 12)}) {
		t.Error("un bloque de cualquier día ya es haber declarado")
	}
}
