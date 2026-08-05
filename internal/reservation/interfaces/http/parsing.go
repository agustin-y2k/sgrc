package http

import (
	"fmt"
	"time"
)

// parseFecha interpreta "2026-03-09" como una fecha en UTC (medianoche) —
// domain.ReservaGrupo/Reserva.Fecha solo le importa el día, no la hora.
func parseFecha(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("fecha inválida (formato esperado AAAA-MM-DD): %q", s)
	}
	return t, nil
}

// parseHora interpreta "08:30" como offset desde medianoche —
// domain.Reserva/ReservaGrupo.HoraInicio/HoraFin son time.Duration, no
// time.Time (no hay zona horaria ni fecha involucrada, es un horario de
// aula).
func parseHora(s string) (time.Duration, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("hora inválida (formato esperado HH:MM): %q", s)
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}

// formatHora es el inverso de parseHora, para las respuestas JSON.
func formatHora(d time.Duration) string {
	horas := int(d.Hours())
	minutos := int(d.Minutes()) % 60
	return fmt.Sprintf("%02d:%02d", horas, minutos)
}
