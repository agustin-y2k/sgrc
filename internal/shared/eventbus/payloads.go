package eventbus

import "time"

// Payload tipado del evento `reserva.cancelada`.

// ReservaCancelada es una Reserva puntual que se canceló.
type ReservaCancelada struct {
	ReservaID string
	// Etiqueta es cómo el docente reconoce el equipo: "PC 7", "Proyector Epson".
	Etiqueta string
	Fecha    time.Time
}

// CancelacionesDeUsuario junta TODO lo que se le canceló a una persona en una
// misma operación: un bloqueo administrativo sobre tres PCs de su clase es
// una sola noticia para ella, no tres.
type CancelacionesDeUsuario struct {
	UsuarioID string
	Motivo    string
	Reservas  []ReservaCancelada
}

// ══════════════════════════════════════════════════════════════════
// Licencias de software
// ══════════════════════════════════════════════════════════════════

// LicenciaPorVencer es una licencia que necesita que alguien haga algo.
type LicenciaPorVencer struct {
	LicenciaID string
	Nombre     string
	// Etiqueta es cómo se nombra al equipo: "PC 3" o "Notebook chica".
	Etiqueta         string
	Identificador    int
	CarroNombre      string
	FechaVencimiento time.Time
	// DiasRestantes negativo significa que ya venció hace esos días.
	DiasRestantes int
}

// AvisoDeLicencias es TODO lo que encontró una barrida del job, en un solo
// evento.
type AvisoDeLicencias struct {
	PorVencer []LicenciaPorVencer
	Vencidas  []LicenciaPorVencer
}

// Total es la cantidad de licencias del aviso, para los textos que empiezan
// contándolas.
func (a AvisoDeLicencias) Total() int {
	return len(a.PorVencer) + len(a.Vencidas)
}

// ══════════════════════════════════════════════════════════════════ Correo
// ══════════════════════════════════════════════════════════════════ Los tres
// payloads de abajo existen porque el envío de correo no puede pasar dentro
// del request: InMemoryEventBus publica en la goroutine de quien publica, así
// que abrir una conexión SMTP adentro dejaría a un docente esperando a Gmail
// para terminar de registrarse.

// DatosDeRecuperacion es lo que hace falta para mandarle a alguien su código
// de recuperación (RF-01.10).
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
// que no tiene ninguna.
type CuentaSoloConGoogle struct {
	Email  string
	Nombre string
}

// CuentaAprobada avisa que un Admin aprobó una cuenta pendiente.
type CuentaAprobada struct {
	UsuarioID string
	Email     string
	Nombre    string
}

// ══════════════════════════════════════════════════════════════════ El
// barrido de reservas y entregas (RF-08.10 a RF-08.13)
// ══════════════════════════════════════════════════════════════════ Los
// cuatro payloads de abajo los publica un RELOJ, no una persona.

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
	// EquiposSinDevolver son los de esa misma reserva que en este momento están
	// afuera y pasados de hora.
	EquiposSinDevolver []string
	// MinutosDeGracia es cuánto se espera antes de liberar la reserva.
	MinutosDeGracia int
}

// EquipoNoDisponibleParaReserva es el aviso suelto al docente siguiente: una
// máquina de su reserva no volvió.
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
type PedidoDeLiberacion struct {
	// El dueño de la reserva, que es quien recibe el aviso.
	UsuarioID string
	Email     string
	Nombre    string

	// Quien pide.
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

	// Mensaje es lo que escribió quien pide, si escribió algo.
	Mensaje string
}

// ReservaSinRetirar: la clase ya empezó y nadie vino a buscar las máquinas.
type ReservaSinRetirar struct {
	UsuarioID     string
	Email         string
	Nombre        string
	MateriaNombre string
	Fecha         time.Time
	HoraInicio    time.Duration
	Equipos       []string
	// MinutosDeGracia es a los cuántos minutos quedan libres.
	MinutosDeGracia int
}

// PrestamoDemorado es una máquina que tenía que haber vuelto y no volvió.
type PrestamoDemorado struct {
	PrestamoID string
	// Etiqueta: "PC 7" o "Proyector Epson".
	Etiqueta    string
	CarroNombre string
	Quien       string
	// Email vacío si quien la tiene no tiene cuenta en el sistema.
	Email string
	// EntregadoEn y DebioVolverA vienen YA en la zona de la escuela, no en UTC:
	// los publica el barrido, que es el único que tiene el reloj de la
	// institución (ver reservation/application/vigilante.go).
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
	// Del docente de la PRÓXIMA reserva de esa PC, si la hay.
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

// ══════════════════════════════════════════════════════════════════ El buzón
// de sugerencias y los pedidos para dictar una materia
// ══════════════════════════════════════════════════════════════════

// SugerenciaNueva: alguien escribió en el buzón.
type SugerenciaNueva struct {
	SugerenciaID string
	// Quien es el nombre de quien escribió, ya resuelto.
	Quien string
	// Tipo es "SUGERENCIA" o "PROBLEMA": lo primero que quiere saber quien
	// lee es si hay algo roto.
	Tipo  string
	Texto string
	// Pantalla desde la que se escribió, si se pudo saber.
	Pantalla string
}

// SugerenciaRespondida: un Admin contestó. Le llega a quien escribió.
type SugerenciaRespondida struct {
	SugerenciaID string
	UsuarioID    string
	Email        string
	Nombre       string
	// TextoOriginal para que el aviso recuerde de qué mensaje se trata: entre
	// que se escribió y que llegó la respuesta pueden pasar días.
	TextoOriginal string
	Respuesta     string
}

// PedidoDeMateriaNuevo: un docente pidió dictar una materia.
type PedidoDeMateriaNuevo struct {
	PedidoID string
	// Quien pide.
	UsuarioID string
	Nombre    string
	// MateriaNombre es el nombre de la materia elegida, o el que escribió a
	// mano si todavía no existe. CursoNombre puede venir vacío.
	MateriaNombre string
	CursoNombre   string
	// EsMateriaNueva: la materia no existe y hay que crearla al aprobar.
	EsMateriaNueva bool
	Motivo         string
	// DocentesActuales son los que ya dictan esa materia, para avisarles.
	DocentesActuales []DocenteDeMateria
}

// DocenteDeMateria es quien ya dicta una materia, con lo necesario para
// avisarle.
type DocenteDeMateria struct {
	UsuarioID string
	Email     string
	Nombre    string
}

// PedidoDeMateriaResuelto: el Admin aprobó o rechazó. Le llega a quien pidió.
type PedidoDeMateriaResuelto struct {
	PedidoID      string
	UsuarioID     string
	Email         string
	Nombre        string
	MateriaNombre string
	Aprobado      bool
	// Respuesta es lo que escribió el Admin. En un rechazo es lo único que
	// explica el porqué, así que el texto del aviso la incluye siempre.
	Respuesta string
}
