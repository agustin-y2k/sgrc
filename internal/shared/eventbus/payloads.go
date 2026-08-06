package eventbus

import "time"

// Payload tipado del evento `reserva.cancelada`.
//
// Vive acá y no en el `domain/` de reservation ni en el de notification por
// la misma razón que el resto de este paquete: es el contrato ENTRE los dos,
// y ninguno puede importar el dominio del otro (docs/06-arquitectura.md §3).
// Es un tipo y no un `map[string]string` como los eventos de tres campos de
// texto: con una lista adentro el mapa deja de documentar nada y cada
// suscriptor tiene que adivinar las claves. Y con un tipo el compilador
// avisa — agregarle un campo al publicador sin actualizar al suscriptor
// deja de ser un `nil` silencioso en tiempo de ejecución.

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

// ══════════════════════════════════════════════════════════════════
// Correo
// ══════════════════════════════════════════════════════════════════
// Los tres payloads de abajo existen porque el envío de correo no puede
// pasar dentro del request: InMemoryEventBus publica en la goroutine de
// quien publica, así que abrir una conexión SMTP adentro dejaría a un
// docente esperando a Gmail para terminar de registrarse. Los manda
// internal/notification, que entrega en su propia goroutine y con timeout.
//
// Viaja el dato que el correo necesita (nombre, email) y no solo un ID: el
// suscriptor tendría que releerlo de la base, y el paquete que lo manda no
// puede importar el domain de auth.

// DatosDeRecuperacion es lo que hace falta para mandarle a alguien su
// código de recuperación (RF-01.10).
//
// CUIDADO con los logs: el código en claro solo existe entre que se genera
// y que se manda, y un `%+v` sobre este struct lo imprimiría.
type DatosDeRecuperacion struct {
	Email  string
	Nombre string
	// Codigo en claro. En la base solo está su hash (migración 009).
	Codigo string
	// MinutosDeVigencia viaja armado para que el texto del correo no tenga
	// que conocer la constante del dominio de auth.
	MinutosDeVigencia int
}

// CuentaSoloConGoogle: alguien pidió recuperar la contraseña de una cuenta
// que no tiene ninguna. El correo sale igual y se lo explica — lo recibe la
// dueña de la casilla, así que no se le revela nada a un tercero, y sin él
// el síntoma sería "pedí el código y nunca llegó".
type CuentaSoloConGoogle struct {
	Email  string
	Nombre string
}

// CuentaAprobada avisa que un Admin aprobó una cuenta pendiente.
//
// Es un evento aparte de `cuenta.pendiente.resuelta` y no un campo suyo:
// ese se publica igual al aprobar que al rechazar (su trabajo es cerrarle
// el aviso al Admin), así que reusarlo felicitaría a quien fue rechazado.
type CuentaAprobada struct {
	UsuarioID string
	Email     string
	Nombre    string
}
