package domain

import "time"

// DisponibleAhora es el cálculo central de RF-07.2 (ver también
// docs/07-modelo-datos.md §2, "Cálculo de ¿disponible ahora?"):
//
//  1. Si hay una excepción cargada para hoy, PISA por completo el patrón
//     semanal — no importa si algún bloque cubriría la hora actual.
//  2. Si no hay excepción, alcanza con que la hora actual caiga dentro de
//     alguno de los bloques del día de semana correspondiente (puede haber
//     más de uno).
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
