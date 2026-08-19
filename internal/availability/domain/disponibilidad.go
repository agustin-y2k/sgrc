package domain

import "time"

// DisponibleAhora es el cálculo central de RF-07.2 (ver también
// docs/07-modelo-datos.md §2, "Cálculo de ¿disponible ahora?"): 1. Si hay una
// excepción cargada para hoy, PISA por completo el patrón semanal — no
// importa si algún bloque cubriría la hora actual.
func DisponibleAhora(bloques []*BloqueHorario, excepcionHoy *Excepcion, diaActual DiaSemana, horaActual time.Duration) bool {
	if excepcionHoy != nil {
		return excepcionHoy.DisponibleAhora(horaActual)
	}
	for _, b := range bloques {
		if b.Cubre(diaActual, horaActual) {
			return true
		}
	}
	return false
}
