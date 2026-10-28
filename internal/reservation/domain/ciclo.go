package domain

import (
	"errors"
	"fmt"
	"time"
)

var ErrReservaFueraDelCiclo = errors.New("la reserva debe quedar dentro del año del ciclo lectivo")

// ValidarReservaEnCiclo compara fechas civiles; no convierte una fecha sin
// horario a otra zona. El fin es exclusivo: terminar el 1 de enero a las 00:00
// no ocupa tiempo del año siguiente.
func ValidarReservaEnCiclo(anio int, fecha time.Time, inicio, fin time.Duration) error {
	dia := time.Date(fecha.Year(), fecha.Month(), fecha.Day(), 0, 0, 0, 0, time.UTC)
	limite := time.Date(anio+1, time.January, 1, 0, 0, 0, 0, time.UTC)
	termino := dia.Add(fin)
	if fin <= inicio {
		termino = termino.AddDate(0, 0, 1)
	}
	if fecha.Year() != anio || termino.After(limite) {
		return fmt.Errorf("%w: ciclo %d, del 01/01/%d al 31/12/%d", ErrReservaFueraDelCiclo, anio, anio, anio)
	}
	return nil
}
