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
	// Etiqueta es cómo el docente reconoce el equipo: "PC 7", "Proyector
	// Epson". No es el UUID ni el número pelado — lo que se
	// reserva puede no tener número, y "PC 0" es lo que sale de formatear
	// uno que no existe.
	//
	// Vacía si no se pudo resolver, y el mensaje sale igual sin el detalle:
	// perder el aviso sería peor.
	Etiqueta string
	Fecha    time.Time
}

// CancelacionesDeUsuario junta TODO lo que se le canceló a una persona en
// una misma operación: un bloqueo administrativo sobre tres PCs de su clase
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
// Licencias de software
// ══════════════════════════════════════════════════════════════════

// LicenciaPorVencer es una licencia que necesita que alguien haga algo.
//
// Viaja con el nombre de la PC y del carro ya resueltos, no con sus UUID:
// el aviso lo lee una persona que tiene que ir hasta una máquina, y "PC 3
// del Carro 1" es lo que le sirve para encontrarla.
type LicenciaPorVencer struct {
	LicenciaID string
	Nombre     string
	// Etiqueta es cómo se nombra al equipo: "PC 3" o "Notebook chica".
	// Identificador va en 0 en un equipo suelto, así que un aviso
	// armado con él manda a buscar una "PC 0" que no existe.
	Etiqueta         string
	Identificador    int
	CarroNombre      string
	FechaVencimiento time.Time
	// DiasRestantes negativo significa que ya venció hace esos días.
	DiasRestantes int
}

// AvisoDeLicencias es TODO lo que encontró una barrida del job, en un solo
// evento.
//
// Uno y no un evento por licencia: bloquear el mismo AutoCAD en las ocho
// PCs de un carro daría ocho mails idénticos que nadie lee: es la misma
// lección que ya dejó CancelacionesDeUsuario. Los dos grupos van separados
// porque no piden lo mismo —una todavía se puede renovar a tiempo y la otra
// ya dejó de funcionar— y el correo los lista bajo títulos distintos.
type AvisoDeLicencias struct {
	PorVencer []LicenciaPorVencer
	Vencidas  []LicenciaPorVencer
}

// Total es la cantidad de licencias del aviso, para los textos que empiezan
// contándolas.
func (a AvisoDeLicencias) Total() int {
	return len(a.PorVencer) + len(a.Vencidas)
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
	// Codigo en claro. En la base solo está su hash.
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

// ══════════════════════════════════════════════════════════════════
// El barrido de reservas y entregas (RF-08.10 a RF-08.13)
// ══════════════════════════════════════════════════════════════════
//
// Los cuatro payloads de abajo los publica un RELOJ, no una persona. Eso
// cambia lo que hay que cuidar: como nadie está esperando el resultado,
// nadie se da cuenta si un aviso sale dos veces. La idempotencia la
// garantizan las marcas de cada fila, del lado de reservation (migración
// 014), no estos tipos.
//
// Todos llevan el contacto adentro por el mismo motivo que los de correo:
// quien los consume es notification, que no puede importar el domain de
// auth (docs/06-arquitectura.md §3).

// RecordatorioDeReserva: "en un rato tenés reserva". Uno por ReservaGrupo,
// no por PC — el docente vive la clase como una sola cosa.
type RecordatorioDeReserva struct {
	UsuarioID     string
	Email         string
	Nombre        string
	MateriaNombre string
	Fecha         time.Time
	HoraInicio    time.Duration
	// Equipos son las etiquetas ("PC 3", "Proyector Epson"), no los UUID
	// ni los números: lo reservable puede no tener número.
	Equipos []string
	// EquiposSinDevolver son los de esa misma reserva que en este momento
	// están afuera y pasados de hora. Van ADENTRO del recordatorio y no en
	// un aviso aparte: si el docente igual va a recibir un correo por esta
	// clase, mandarle dos es el bombardeo que se quiso evitar.
	EquiposSinDevolver []string
	// MinutosDeGracia es cuánto se espera antes de liberar la reserva. Va
	// en el payload para que el texto no tenga que conocer la
	// configuración del despliegue.
	MinutosDeGracia int
}

// EquipoNoDisponibleParaReserva es el aviso suelto al docente siguiente: una
// máquina de su reserva no volvió.
//
// Existe además del recordatorio porque la demora puede detectarse DESPUÉS
// de que el recordatorio ya salió, o cuando falta menos de una hora para su
// clase. Es la otra mitad de max(detección, inicio − 1 h).
type EquipoNoDisponibleParaReserva struct {
	UsuarioID     string
	Email         string
	Nombre        string
	MateriaNombre string
	Fecha         time.Time
	HoraInicio    time.Duration
	Equipos       []string
}

// PedidoDeLiberacion: un docente le pide a otro que le libere un equipo que
// tiene reservado (RF-04.12).
//
// Es el único aviso de esta familia que NO anuncia un cambio: la reserva de
// quien lo recibe sigue intacta y la decisión es suya. El texto tiene que
// dejarlo claro en la primera línea o se lee como una cancelación, que es lo
// que dicen todos los demás avisos sobre una reserva propia.
type PedidoDeLiberacion struct {
	// El dueño de la reserva, que es quien recibe el aviso.
	UsuarioID string
	Email     string
	Nombre    string

	// Quien pide. El ID va para poder registrar de quién habla el aviso
	// (sobre_usuario_id), que es lo que sostiene la regla de un pedido por
	// reserva, por solicitante y por día.
	SolicitanteID     string
	SolicitanteNombre string

	// La reserva pedida, dicha como la reconoce su dueño: qué máquina, para
	// qué materia y en qué franja.
	ReservaID     string
	Etiqueta      string
	MateriaNombre string
	Fecha         time.Time
	HoraInicio    time.Duration
	HoraFin       time.Duration

	// Mensaje es lo que escribió quien pide, si escribió algo. Viaja tal
	// cual: es la parte del pedido que explica para qué las necesita, y
	// reformularla sería ponerle palabras en la boca.
	Mensaje string
}

// ReservaSinRetirar: la clase ya empezó y nadie vino a buscar las máquinas.
//
// Es un aviso PREVIO a la liberación, no la constancia de una liberación ya
// hecha: sale a los quince minutos justamente para que el docente todavía
// pueda ir, cambiar la máquina o cancelar (RF-08.20). Liberar, después, no
// publica nada.
//
// Se agrupa por docente y clase, como las cancelaciones: una reserva de tres
// máquinas es una sola noticia.
type ReservaSinRetirar struct {
	UsuarioID     string
	Email         string
	Nombre        string
	MateriaNombre string
	Fecha         time.Time
	HoraInicio    time.Duration
	Equipos       []string
	// MinutosDeGracia es a los cuántos minutos quedan libres. Va en el
	// payload para que el texto no tenga que conocer la configuración del
	// despliegue, y porque es el dato accionable del aviso: dice cuánto
	// tiempo queda.
	MinutosDeGracia int
}

// PrestamoDemorado es una máquina que tenía que haber vuelto y no volvió.
type PrestamoDemorado struct {
	PrestamoID string
	// Etiqueta: "PC 7" o "Proyector Epson".
	Etiqueta    string
	CarroNombre string
	Quien       string
	// Email vacío si quien la tiene no tiene cuenta en el sistema. En ese
	// caso el reclamo le llega solo a los Admin, que es lo único que se
	// puede hacer.
	Email string
	// EntregadoEn y DebioVolverA vienen YA en la zona de la escuela, no en
	// UTC: los publica el barrido, que es el único que tiene el reloj de la
	// institución (ver reservation/application/vigilante.go). Quien arma el
	// texto solo los formatea — si tuviera que convertirlos, cada mensaje
	// nuevo sería otra oportunidad de olvidarse y mostrar tres horas de más.
	//
	// EntregadoEn es cuándo se la llevó, y existe porque el correo lo dice:
	// "la tiene X desde las 10:15". Sin este campo ese hueco se llenaba con
	// DebioVolverA, o sea que el correo afirmaba que se la había llevado
	// justo a la hora en que tenía que devolverla.
	EntregadoEn     time.Time
	DebioVolverA    time.Time
	MinutosDeDemora int
}

// PrestamosDemorados junta todo lo que está vencido en esta barrida: a los
// Admin les llega un solo correo con la lista, y a cada persona que tenga
// cuenta, uno suyo.
type PrestamosDemorados struct {
	Prestamos []PrestamoDemorado
}

// EquipoSinDevolverAlCierre es una máquina que quedó afuera al terminar la
// jornada, con el docente al que le va a faltar mañana.
type EquipoSinDevolverAlCierre struct {
	Etiqueta    string
	CarroNombre string
	Quien       string
	// DesdeCuando también viaja en la zona de la escuela (ver
	// PrestamoDemorado).
	DesdeCuando time.Time
	// Del docente de la PRÓXIMA reserva de esa PC, si la hay. Se avisa solo
	// al siguiente y no a todos los de la semana: es el único para quien el
	// aviso es accionable hoy, y si mañana la máquina sigue afuera, el corte
	// del día siguiente avisa al que siga.
	ProximoUsuarioID string
	ProximoEmail     string
	ProximoNombre    string
	ProximaFecha     time.Time
	ProximaHora      time.Duration
}

// EquiposSinDevolverAlCierre es el corte de fin de jornada.
type EquiposSinDevolverAlCierre struct {
	Equipos []EquipoSinDevolverAlCierre
}
