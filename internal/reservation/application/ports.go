package application

import (
	"context"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// Repo es el único contrato que este paquete necesita de infrastructure/.
type Repo interface {
	// EnTransaccion corre fn de forma atómica: o se aplican todas las escrituras
	// que haga sobre el Repo que recibe, o no queda ninguna.
	EnTransaccion(ctx context.Context, fn func(Repo) error) error

	CrearReservaGrupo(ctx context.Context, g *domain.ReservaGrupo) error
	BuscarReservaGrupoPorID(ctx context.Context, id string) (*domain.ReservaGrupo, error)
	GuardarReservaGrupo(ctx context.Context, g *domain.ReservaGrupo) error

	CrearReserva(ctx context.Context, r *domain.Reserva) error
	BuscarReservaPorID(ctx context.Context, id string) (*domain.Reserva, error)
	GuardarReserva(ctx context.Context, r *domain.Reserva) error
	ListarReservasPorGrupo(ctx context.Context, reservaGrupoID string) ([]*domain.Reserva, error)

	// ListarReservasFuturasDeEquipo: usado tanto por la cascada de inventory
	// (cambio de estado / baja de un equipo) como por el bloqueo administrativo
	// (RF-04.7) — todas las Reserva CONFIRMADA de un equipo a partir de cierta
	// fecha/hora.
	ListarReservasFuturasDeEquipo(ctx context.Context, equipoID string, desde time.Time) ([]*domain.Reserva, error)

	// BuscarSolapamientos resuelve el pre-chequeo de TODO el lote —todos los
	// equipos contra todas las fechas— en una sola consulta, y devuelve qué
	// chocó contra qué.
	BuscarSolapamientos(ctx context.Context, equipoIDs []string, fechas []time.Time, horaInicio, horaFin time.Duration) ([]Solapamiento, error)

	// ListarReservasFuturasDeMateria: usado por la cascada de auth (dar de baja
	// al único docente de una materia, RF-02.8) — todas las Reserva CONFIRMADA
	// vinculadas a esa materia a partir de cierta fecha/hora.
	ListarReservasFuturasDeMateria(ctx context.Context, materiaID string, desde time.Time) ([]*domain.Reserva, error)

	// EliminarReservasYGruposDeCiclo: usado por la cascada de archivado de
	// academic (RF-02.4) — borra FÍSICAMENTE (no cancela) toda
	// Reserva/ReservaGrupo vinculada a materias de ese ciclo lectivo, sin
	// importar su estado.
	EliminarReservasYGruposDeCiclo(ctx context.Context, cicloID string) (gruposEliminados int, reservasEliminadas int, err error)

	// ListarReservasConfirmadasVencidas: para el job RF-04.9 — las Reserva
	// CONFIRMADA cuya fecha+horaFin ya pasó respecto a "ahora", de la más vieja
	// a la más nueva y como mucho `limite` filas.
	ListarReservasConfirmadasVencidas(ctx context.Context, ahora time.Time, limite int) ([]*domain.Reserva, error)

	// ── Préstamos (custodia física de una PC) ─────────────────────── Viven en
	// este paquete y no en inventory porque casi todas sus reglas son sobre
	// reservas: contra qué reserva se entregó, si volvió antes de que empiece la
	// siguiente, quién es el próximo que la tiene reservada.
	CrearPrestamo(ctx context.Context, p *domain.Prestamo) error
	BuscarPrestamoPorID(ctx context.Context, id string) (*domain.Prestamo, error)
	GuardarPrestamo(ctx context.Context, p *domain.Prestamo) error
	// BuscarPrestamoAbiertoDeEquipo devuelve el préstamo sin devolver de esa PC,
	// o ErrPrestamoNoEncontrado si la máquina está en el laboratorio.
	BuscarPrestamoAbiertoDeEquipo(ctx context.Context, equipoID string) (*domain.Prestamo, error)
	// ListarPrestamosAbiertos es "qué hay afuera ahora mismo": la pantalla que
	// reemplaza al papel.
	ListarPrestamosAbiertos(ctx context.Context) ([]*PrestamoDetallado, error)
	// ListarPrestamosDeEquipo es el historial de una máquina, de lo más reciente
	// a lo más viejo.
	ListarPrestamosDeEquipo(ctx context.Context, equipoID string, limite int) ([]*PrestamoDetallado, error)

	// ── El barrido (RF-08.10 a RF-08.13) ────────────────────────────
	// ReservasAVigilar trae las reservas CONFIRMADA de hoy y mañana, con el
	// contacto del docente y el estado de custodia de cada PC ya resueltos.
	ReservasAVigilar(ctx context.Context, hoy time.Time) ([]ReservaParaVigilar, error)
	// PrestamosAVigilar son todos los abiertos, con ubicación y contacto.
	PrestamosAVigilar(ctx context.Context) ([]PrestamoParaVigilar, error)

	// ProximaReservaDeEquipo es a quién le va a faltar esa máquina, con el
	// contacto resuelto.
	ProximaReservaDeEquipo(ctx context.Context, equipoID string, desde time.Time) (*ProximaReserva, error)

	// Las cinco marcas de idempotencia.
	MarcarRecordatorioEnviado(ctx context.Context, grupoID string, ahora time.Time) error
	MarcarAvisoSinRetirarEnviado(ctx context.Context, grupoID string, ahora time.Time) error
	MarcarAvisoEquipoNoDisponible(ctx context.Context, reservaID string, ahora time.Time) error
	MarcarDemoraAvisada(ctx context.Context, prestamoID string, ahora time.Time) error
	MarcarCierreAvisado(ctx context.Context, prestamoID string, jornada time.Time) error

	// ListarReservas devuelve las reservas que matcheen el filtro, con los
	// nombres de PC, carro, materia y curso ya resueltos.
	ListarReservas(ctx context.Context, f FiltroReservas) ([]ReservaDetallada, int, error)

	// CalendarioDeEquipo implementa RF-04.4 — los bloques ocupados de una PC en
	// un rango de fechas, con el nombre del docente y de la materia para poder
	// mostrarlos.
	CalendarioDeEquipo(ctx context.Context, equipoID string, desde, hasta time.Time) ([]BloqueCalendario, error)

	// ListarEquiposDisponiblesEn implementa el "tildar casillas" de RF-04.2: qué
	// equipos están libres para un día y franja horaria concretos.
	ListarEquiposDisponiblesEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration, materiaID string) ([]EquipoDisponible, error)

	// ListarEquiposOcupadosEn es la otra mitad de la misma pregunta (RF-04.11):
	// qué equipos de ese universo ya tiene alguien en esa franja, y quién.
	ListarEquiposOcupadosEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) ([]EquipoOcupado, error)

	// ListarEquiposLibresEnLaSerie: los equipos libres en TODAS las fechas que
	// le quedan a la serie de ese grupo, de esa fecha en adelante (RF-08.14).
	ListarEquiposLibresEnLaSerie(ctx context.Context, grupoID string) ([]EquipoDisponible, error)

	// ReservasDeLaSerieDesde: la misma máquina, en todas las ocurrencias que le
	// quedan a la serie a partir de esta (RF-08.14).
	ReservasDeLaSerieDesde(ctx context.Context, reservaID string) ([]*domain.Reserva, error)

	// DatosParaPedirLiberacion trae, en una sola consulta, todo lo que el pedido
	// de RF-04.12 necesita decidir y decir: en qué estado está la reserva, de
	// quién es, con qué contacto, qué máquina y de qué franja.
	DatosParaPedirLiberacion(ctx context.Context, reservaID string) (*ReservaParaPedido, error)

	// YaPidioLiberacionHoy sostiene la regla de un pedido por reserva, por
	// solicitante y por día (RF-04.12).
	YaPidioLiberacionHoy(ctx context.Context, reservaID, solicitanteID string, dia time.Time) (bool, error)

	CrearReglaRecurrencia(ctx context.Context, regla *domain.ReglaRecurrencia) error
	// ListarGruposFuturosDeRegla: los ReservaGrupo de una regla recurrente con
	// fecha posterior a la indicada — usado por "cancelar esta y las siguientes"
	// (RF-04.6).
	ListarGruposFuturosDeRegla(ctx context.Context, reglaID string, desde time.Time) ([]*domain.ReservaGrupo, error)
}

// FiltroReservas acota qué reservas devuelve ListarReservas.
type FiltroReservas struct {
	// CreadoPor: las reservas de un docente puntual. Es el filtro que usa
	// "mis reservas".
	CreadoPor *string
	EquipoID  *string
	MateriaID *string
	// Desde/Hasta acotan por fecha (inclusive ambos extremos).
	Desde *time.Time
	Hasta *time.Time
	// IncluirCanceladas: por defecto las canceladas no se listan, porque
	// lo que casi siempre se quiere ver es lo que sigue vigente.
	IncluirCanceladas bool
	// Pagina acota cuántas filas vuelven.
	Pagina paginacion.Pagina
}

// BloqueCalendario es una reserva vista desde el calendario de una PC
// (RF-04.4): además del horario, lleva el nombre del docente y de la materia
// ya resueltos, que es lo que se muestra en pantalla.
type BloqueCalendario struct {
	Reserva       *domain.Reserva
	MateriaNombre string
	CursoNombre   string
}

// ReservaDetallada es una Reserva con los nombres ya resueltos para mostrarla
// en pantalla: de qué PC y carro se trata, y de qué materia y curso.
type ReservaDetallada struct {
	Reserva *domain.Reserva
	// Identificador va en 0 y CarroNombre vacío en un equipo suelto: un
	// proyector no está en ningún carro. Lo que se muestra es Etiqueta.
	Identificador int
	CarroNombre   string
	// Etiqueta es cómo se nombra al equipo en pantalla: "PC 3" o "Proyector
	// Epson".
	Etiqueta      string
	MateriaNombre string
	CursoNombre   string
	// ReglaRecurrenciaID viene del ReservaGrupo, no de la Reserva.
	ReglaRecurrenciaID *string
}

// EquipoDisponible es una PC libre en la franja consultada, con los datos que
// RF-03.7 dice que el docente necesita para elegir (software instalado,
// freezado) sin tener que pedirlos a inventory por separado.
type EquipoDisponible struct {
	EquipoID string
	// Identificador va en 0 para un equipo suelto (un proyector no es
	// "PC 3"). Lo que se muestra es Etiqueta.
	Identificador int
	// Etiqueta es cómo se lo nombra: "PC 3" o "Proyector Epson".
	Etiqueta string
	// Tipo distingue una PC de un proyector. Texto libre.
	Tipo string
	// CarroID y CarroNombre vacíos en un equipo suelto.
	CarroID           string
	CarroNombre       string
	Freezado          bool
	SoftwareInstalado string

	// Tramo es en qué grupo cae este equipo para la materia que se está
	// reservando (RF-03.21). Es lo que parte la lista en tres bloques.
	Tramo TramoPreferencia
	// PreferenciaMateria, PreferenciaAnio y PreferenciaDivision describen la
	// marca que puso al equipo en su tramo — la de la materia propia si es
	// preferente, la ajena más fuerte si no.
	PreferenciaMateria  string
	PreferenciaAnio     int
	PreferenciaDivision string
}

// TramoPreferencia agrupa los equipos libres según qué materia los prefiere
// (RF-03.21).
type TramoPreferencia string

const (
	// TramoPreferente: la marca del equipo apunta a la materia que se está
	// reservando. Van primero.
	TramoPreferente TramoPreferencia = "PREFERENTE"
	// TramoNeutral: el equipo no es preferente de nadie. El orden de siempre.
	TramoNeutral TramoPreferencia = "NEUTRAL"
	// TramoDeOtraMateria: lo prefiere otra materia. Van al final, y cuanto
	// más fuerte el reclamo ajeno, más abajo.
	TramoDeOtraMateria TramoPreferencia = "DE_OTRA_MATERIA"
)

// MotivoDePreferencia arma el texto que explica por qué el equipo está donde
// está: "Preferente para Matemática de 3°B".
func (e EquipoDisponible) MotivoDePreferencia() string {
	if e.PreferenciaMateria == "" {
		return ""
	}
	motivo := "Preferente para " + e.PreferenciaMateria
	if e.PreferenciaAnio == 0 {
		return motivo
	}
	motivo += fmt.Sprintf(" de %d°", e.PreferenciaAnio)
	return motivo + e.PreferenciaDivision
}

// EquipoOcupado es un equipo que ya tiene dueño en la franja consultada
// (RF-04.11).
type EquipoOcupado struct {
	EquipoID    string
	Etiqueta    string
	CarroNombre string
	// ReservaID es la fila que lo ocupa: es lo que después recibe el pedido
	// de liberación.
	ReservaID string
	// EsBloqueo: lo tomó un Admin (RF-04.7). No tiene docente detrás, así
	// que no hay a quién pedirle nada — lo que se muestra es el motivo.
	EsBloqueo bool
	// DocenteID es de quién es la reserva. Sirve para no ofrecerle a alguien
	// pedirse a sí mismo. nil en un bloqueo, o si la cuenta se eliminó.
	DocenteID     *string
	DocenteNombre string
	MateriaNombre string
	Motivo        string
	// HoraInicio y HoraFin son las de la reserva que lo ocupa, que pueden no
	// coincidir con la franja consultada: alguien que necesita el equipo de 10 a
	// 12 tiene que poder ver que quien lo tiene lo usa de 8 a 11.
	HoraInicio time.Duration
	HoraFin    time.Duration
	// PuedePedirse lo decide el servidor para que la pantalla no tenga que
	// replicar la regla: false en un bloqueo, en una reserva propia y si esa
	// franja ya empezó.
	PuedePedirse bool
}

// ReservaParaPedido es lo que hace falta para resolver un pedido de
// liberación (RF-04.12): las cuatro condiciones que pueden rechazarlo y los
// datos con los que se arma el aviso.
type ReservaParaPedido struct {
	Estado    domain.EstadoReserva
	EsBloqueo bool
	// DuenoID nil en un bloqueo administrativo, o si la cuenta del docente
	// se eliminó. En los dos casos no hay a quién pedirle.
	DuenoID     *string
	DuenoNombre string
	DuenoEmail  string

	Etiqueta      string
	MateriaNombre string
	Fecha         time.Time
	HoraInicio    time.Duration
	HoraFin       time.Duration
}

// ValidadorMateria es el puerto hacia academic — confirma que un docente está
// efectivamente asignado a la materia antes de dejarlo reservar para ella
// (RF-04.1).
type ValidadorMateria interface {
	DocenteEstaAsignado(ctx context.Context, materiaID, usuarioID string) (bool, error)

	// MateriaAceptaReservas: la materia existe y ni ella, ni su curso, ni su
	// ciclo están archivados.
	MateriaAceptaReservas(ctx context.Context, materiaID string) (bool, error)
}

// ValidadorEquipo es el puerto hacia inventory — confirma que una PC existe y
// está en condiciones de reservarse (estado DISPONIBLE, no dada de baja)
// antes de dejarla incluir en una reserva.
type ValidadorEquipo interface {
	EquipoDisponibleParaReservar(ctx context.Context, equipoID string) (bool, error)

	// EquiposNoReservables responde lo mismo que EquipoDisponibleParaReservar
	// pero para una lista, en UNA consulta: devuelve cuáles de los pedidos no se
	// pueden reservar (no existen, no están disponibles, están dados de baja o
	// no son reservables).
	EquiposNoReservables(ctx context.Context, equipoIDs []string) ([]string, error)

	// EquipoEstaEnInventario es más laxo que EquipoDisponibleParaReservar: solo
	// exige que la PC exista y no esté dada de baja, sin mirar su estado.
	EquipoEstaEnInventario(ctx context.Context, equipoID string) (bool, error)

	// EtiquetasDeEquipos traduce los UUID al nombre con el que la gente reconoce
	// cada cosa ("PC 7", "Proyector Epson"), para poder decir en un aviso cuáles
	// se cancelaron.
	EtiquetasDeEquipos(ctx context.Context, equipoIDs []string) (map[string]string, error)
}

// ObtenedorNombreDocente es el puerto hacia auth — solo necesitamos el nombre
// completo para el snapshot (nombre_docente_snapshot), no el resto de la
// lógica de auth.
type ObtenedorNombreDocente interface {
	NombreCompletoDe(ctx context.Context, usuarioID string) (string, error)
}

type IDGenerator func() string

// PrestamoDetallado es un Prestamo con lo mínimo para saber de qué máquina
// habla y de dónde salió, resuelto por JOIN — mismo criterio que
// ReservaDetallada.
type PrestamoDetallado struct {
	Prestamo *domain.Prestamo
	// Identificador va en 0 para un equipo suelto.
	Identificador int
	Etiqueta      string
	// CarroNombre vacío en un equipo suelto.
	CarroNombre string
	// MateriaNombre solo en los préstamos que salieron contra una reserva.
	MateriaNombre *string
}

// ══════════════════════════════════════════════════════════════════ El
// barrido (RF-08.10 a RF-08.13)
// ══════════════════════════════════════════════════════════════════ Son DOS
// consultas y no cinco a propósito.

// ReservaParaVigilar es una reserva confirmada con todo lo que el barrido
// necesita: para decidir, y para poder avisar sin volver a la base.
type ReservaParaVigilar struct {
	ReservaID string
	GrupoID   *string
	EquipoID  string
	// Identificador es el número visible ("PC 7"), que es lo que el
	// docente reconoce.
	Identificador int
	// Etiqueta es cómo se nombra al equipo en un aviso: "PC 7" o "Proyector
	// Epson".
	Etiqueta   string
	Fecha      time.Time
	HoraInicio time.Duration
	HoraFin    time.Duration
	// Tipo distingue la clase de un docente de un bloqueo administrativo
	// estatal.
	Tipo          domain.TipoReserva
	MateriaNombre *string

	DocenteID     *string
	DocenteNombre string
	DocenteEmail  string

	RecordatorioEnviado            bool
	AvisoEquipoNoDisponibleEnviado bool
	// AvisoSinRetirarEnviado: ya salió el aviso de "todavía no las retiraste"
	// (RF-08.20).
	AvisoSinRetirarEnviado bool

	// EquipoAfuera: hay un préstamo sin devolver sobre esa máquina.
	EquipoAfuera bool
	// EquipoDebioVolverA es la hora en que esa máquina tenía que estar de vuelta.
	// nil si está adentro, o si salió sin hora pactada.
	EquipoDebioVolverA *time.Time

	// UltimaEntregaDelGrupo es cuándo se entregó por última vez alguna máquina
	// CONTRA ESTA RESERVA. Es el dato que distingue "el docente no vino" de
	// "vino y se llevó una parte", y de ahí sale el plazo corto de liberación
	// (RF-08.10) y el silencio del aviso (RF-08.20).
	UltimaEntregaDelGrupo *time.Time
}

// PrestamoParaVigilar es un préstamo abierto con la ubicación de la máquina
// y el contacto de quien la tiene, cuando esa persona tiene cuenta.
type PrestamoParaVigilar struct {
	Prestamo      *domain.Prestamo
	Identificador int
	// Etiqueta: "PC 7" o "Proyector Epson". Es lo que va en el reclamo.
	Etiqueta    string
	CarroNombre string
	// Email vacío si quien se la llevó no tiene cuenta en el sistema — el caso
	// normal de un préstamo para un trámite.
	Email string
}

// ProximaReserva es la siguiente reserva de una PC, con lo justo para poder
// avisarle a su docente.
type ProximaReserva struct {
	UsuarioID  string
	Email      string
	Nombre     string
	Fecha      time.Time
	HoraInicio time.Duration
}

// Solapamiento es una reserva confirmada que pisa lo que se está por crear.
type Solapamiento struct {
	EquipoID   string
	Etiqueta   string
	Fecha      time.Time
	HoraInicio time.Duration
	HoraFin    time.Duration
	// Docente es el nombre congelado de quien reservó. Vacío en un bloqueo
	// administrativo, que no es la clase de nadie.
	Docente string
	// MotivoBloqueo solo viene en los bloqueos. Es lo que ocupa el lugar que
	// en una clase tiene la materia, así que es lo que hay que mostrar.
	MotivoBloqueo string
}

// describir arma el fragmento que se lee dentro del mensaje de error.
func (s Solapamiento) describir() string {
	etiqueta := s.Etiqueta
	if etiqueta == "" {
		etiqueta = "un equipo"
	}
	quien := s.Docente
	if quien == "" {
		quien = s.MotivoBloqueo
	}
	texto := fmt.Sprintf("%s ya está reservado el %s de %s a %s",
		etiqueta, s.Fecha.Format("02/01"), formatearHora(s.HoraInicio), formatearHora(s.HoraFin))
	if quien != "" {
		texto += " (" + quien + ")"
	}
	return texto
}

func formatearHora(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// ValidadorJornada es el puerto hacia availability, que sabe qué días y en
// qué horas abre la institución (ver domain.PermiteReserva allá).
type ValidadorJornada interface {
	// PermiteReserva responde si ese día y ese rango horario caen dentro de la
	// jornada declarada.
	PermiteReserva(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) (bool, error)
}
