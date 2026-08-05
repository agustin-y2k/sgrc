package application

import "errors"

// ErrIDInvalido: mismo criterio que en el resto del proyecto — un ID sin
// formato UUID válido se mapea a 400, no a 500.
var ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")

// ErrRangoFechasInvalido: RF-06.1 permite filtrar por rango de fechas; un
// "hasta" anterior al "desde" es un error del cliente, no del servidor.
var ErrRangoFechasInvalido = errors.New("la fecha hasta no puede ser anterior a la fecha desde")
