package eventbus

import "time"

// Payload tipado del evento `reserva.cancelada`.
//
// Vive acá y no en el `domain/` de reservation ni en el de notification por
// la misma razón que el resto de este paquete: es el contrato ENTRE los dos,
// y ninguno puede importar el dominio del otro (docs/06-arquitectura.md §3).
// Antes viajaba como `map[string]string`, que funcionaba mientras el evento
// tuviera tres campos de texto — con una lista adentro, el mapa deja de
// documentar nada y cada suscriptor tiene que adivinar las claves.
//
// Es un tipo y no un mapa además porque el compilador avisa: agregarle un
// campo al publicador sin actualizar al suscriptor deja de ser un `nil`
// silencioso en tiempo de ejecución.

// ReservaCancelada es una Reserva puntual que se canceló.
type ReservaCancelada struct {
	ReservaID string
	// PCIdentificador es el número visible de la PC ("PC 7"), no su UUID:
	// es lo que el docente puede reconocer. Va en 0 si no se pudo resolver,
	// y el mensaje sale igual sin el detalle — perder el aviso sería peor.
	PCIdentificador int
	Fecha           time.Time
}

// CancelacionesDeUsuario junta TODO lo que se le canceló a una persona en
// una misma operación: un bloqueo por evaluación sobre tres PCs de su clase
// es una sola noticia para ella, no tres.
//
// El motivo es uno solo por lote a propósito: agrupar cancelaciones con
// razones distintas obligaría a enumerarlas dentro del mensaje, y ahí el
// aviso deja de poder leerse de un vistazo.
type CancelacionesDeUsuario struct {
	UsuarioID string
	Motivo    string
	Reservas  []ReservaCancelada
}
