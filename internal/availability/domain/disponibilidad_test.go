package domain

import (
	"testing"
	"time"
)

// Estos son los tests más importantes del paquete (ver
// docs/07-modelo-datos.md §2): la excepción de hoy, si existe, PISA por
// completo el patrón semanal — no importa si algún bloque cubriría la
// hora actual.

func TestDisponibleAhora_SinExcepcion_SinBloques_NoDisponible(t *testing.T) {
	disponible := DisponibleAhora(nil, nil, Lunes, 10*time.Hour)

	if disponible {
		t.Error("sin bloques ni excepción, nunca debería estar disponible")
	}
}

func TestDisponibleAhora_SinExcepcion_DentroDeUnBloque_Disponible(t *testing.T) {
	bloque, _ := NuevoBloqueHorario("b1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)

	disponible := DisponibleAhora([]*BloqueHorario{bloque}, nil, Lunes, 10*time.Hour)

	if !disponible {
		t.Error("10:00 un lunes debería estar cubierta por el bloque 08:00-12:00")
	}
}

func TestDisponibleAhora_SinExcepcion_FueraDeTodosLosBloques_NoDisponible(t *testing.T) {
	bloque, _ := NuevoBloqueHorario("b1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)

	disponible := DisponibleAhora([]*BloqueHorario{bloque}, nil, Lunes, 14*time.Hour)

	if disponible {
		t.Error("14:00 un lunes no debería estar cubierta por el bloque 08:00-12:00")
	}
}

func TestDisponibleAhora_SinExcepcion_VariosBloquesMismoDia_BastaUno(t *testing.T) {
	// Un admin puede tener varios bloques el mismo día (ej: mañana y tarde).
	manana, _ := NuevoBloqueHorario("b1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)
	tarde, _ := NuevoBloqueHorario("b2", "admin1", Lunes, 14*time.Hour, 18*time.Hour)
	bloques := []*BloqueHorario{manana, tarde}

	if !DisponibleAhora(bloques, nil, Lunes, 15*time.Hour) {
		t.Error("15:00 debería estar cubierta por el bloque de la tarde")
	}
	if DisponibleAhora(bloques, nil, Lunes, 13*time.Hour) {
		t.Error("13:00 (entre los dos bloques) no debería estar disponible")
	}
}

func TestDisponibleAhora_SinExcepcion_BloqueDeOtroDia_NoAplica(t *testing.T) {
	bloque, _ := NuevoBloqueHorario("b1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)

	disponible := DisponibleAhora([]*BloqueHorario{bloque}, nil, Martes, 10*time.Hour)

	if disponible {
		t.Error("un bloque de LUNES no debería aplicar un MARTES")
	}
}

// ── La excepción de hoy pisa el patrón semanal ─────────────────────────

func TestDisponibleAhora_ExcepcionNoDisponible_PisaBloqueQueLoCubriria(t *testing.T) {
	// El admin tiene un bloque LUNES 08-12 que normalmente lo cubriría a
	// las 10:00, pero cargó una excepción NO_DISPONIBLE para hoy (ej:
	// "marcarme no disponible ahora", RF-07.5).
	bloque, _ := NuevoBloqueHorario("b1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)
	excepcion, _ := NuevaExcepcion("e1", "admin1", time.Now(), NoDisponible, nil, nil, nil)

	disponible := DisponibleAhora([]*BloqueHorario{bloque}, excepcion, Lunes, 10*time.Hour)

	if disponible {
		t.Error("la excepción NO_DISPONIBLE de hoy debería pisar el bloque semanal")
	}
}

func TestDisponibleAhora_ExcepcionHorarioModificado_PisaAusenciaDelPatron(t *testing.T) {
	// El admin NO tiene ningún bloque semanal para este día — normalmente
	// no estaría disponible — pero cargó una excepción HORARIO_MODIFICADO
	// para hoy que sí lo cubre a esta hora.
	excepcion, _ := NuevaExcepcion("e1", "admin1", time.Now(), HorarioModificado, dur(9), dur(11), nil)

	disponible := DisponibleAhora(nil, excepcion, Lunes, 10*time.Hour)

	if !disponible {
		t.Error("la excepción HORARIO_MODIFICADO de hoy debería habilitar disponibilidad aunque no haya bloque semanal")
	}
}

func TestDisponibleAhora_ExcepcionHorarioModificado_PeroFueraDeSuRango_NoDisponible(t *testing.T) {
	// Hay excepción para hoy, pero la hora actual cae fuera del rango
	// modificado — sigue sin mirar el patrón semanal (la excepción pisa
	// del todo, no se combina).
	bloqueQueLoCubriria, _ := NuevoBloqueHorario("b1", "admin1", Lunes, 8*time.Hour, 20*time.Hour)
	excepcion, _ := NuevaExcepcion("e1", "admin1", time.Now(), HorarioModificado, dur(9), dur(11), nil)

	disponible := DisponibleAhora([]*BloqueHorario{bloqueQueLoCubriria}, excepcion, Lunes, 15*time.Hour)

	if disponible {
		t.Error("fuera del rango de la excepción debería ser no disponible, aunque el bloque semanal sí lo cubriría")
	}
}

func TestDisponibleAhora_ExcepcionDeOtroDia_NoAplicaria(t *testing.T) {
	// Caso de integración implícito: si el llamador solo pasa la
	// excepción correspondiente al día de hoy (como hace application/),
	// una excepción de otra fecha simplemente no debe llegar acá. Este
	// test documenta que, si SÍ llega una excepción, siempre se aplica
	// sin importar Fecha — la responsabilidad de filtrar por "hoy" es de
	// quien arma el argumento, no de esta función.
	bloque, _ := NuevoBloqueHorario("b1", "admin1", Lunes, 8*time.Hour, 12*time.Hour)
	excepcionDeAyer, _ := NuevaExcepcion("e1", "admin1", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), NoDisponible, nil, nil, nil)

	disponible := DisponibleAhora([]*BloqueHorario{bloque}, excepcionDeAyer, Lunes, 10*time.Hour)

	if disponible {
		t.Error("DisponibleAhora aplica CUALQUIER excepción no-nil pasada, sin mirar su Fecha")
	}
}
