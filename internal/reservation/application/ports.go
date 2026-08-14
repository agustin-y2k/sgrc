package application

import (
	"context"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// Repo es el único contrato que este paquete necesita de infrastructure/.
// El anti-solapamiento en sí NO se valida acá (ver domain.Reserva.SolapaCon
// para una validación anticipada de mejor mensaje) — la garantía real vive
// en la constraint EXCLUDE de la base (docs/07-modelo-datos.md), que
// CrearReserva debe traducir a un error de negocio claro si se dispara.
type Repo interface {
	// EnTransaccion corre fn de forma atómica: o se aplican todas las
	// escrituras que haga sobre el Repo que recibe, o no queda ninguna.
	// Lo necesitan todas las operaciones que tocan más de una fila —
	// crear un grupo con N reservas, cancelar una serie recurrente
	// completa, bloquear varios equipos por un rato — porque a mitad
	// de camino puede saltar la constraint EXCLUDE y RF-04.5 exige que en
	// ese caso no quede nada creado.
	EnTransaccion(ctx context.Context, fn func(Repo) error) error

	CrearReservaGrupo(ctx context.Context, g *domain.ReservaGrupo) error
	BuscarReservaGrupoPorID(ctx context.Context, id string) (*domain.ReservaGrupo, error)
	GuardarReservaGrupo(ctx context.Context, g *domain.ReservaGrupo) error

	CrearReserva(ctx context.Context, r *domain.Reserva) error
	BuscarReservaPorID(ctx context.Context, id string) (*domain.Reserva, error)
	GuardarReserva(ctx context.Context, r *domain.Reserva) error
	ListarReservasPorGrupo(ctx context.Context, reservaGrupoID string) ([]*domain.Reserva, error)

	// ListarReservasFuturasDeEquipo: usado tanto por la cascada de inventory
	// (cambio de estado / baja de un equipo) como por el bloqueo
	// administrativo (RF-04.7) — todas las Reserva CONFIRMADA de un equipo a
	// partir de cierta fecha/hora.
	ListarReservasFuturasDeEquipo(ctx context.Context, equipoID string, desde time.Time) ([]*domain.Reserva, error)

	// BuscarSolapamientos resuelve el pre-chequeo de TODO el lote —todos los
	// equipos contra todas las fechas— en una sola consulta, y devuelve qué
	// chocó contra qué.
	//
	// Es una sola porque el horario es el mismo para toda la serie: una
	// recurrencia semanal de un cuatrimestre sobre ocho equipos son ocho
	// listas de fechas y un único rango horario, no cuarenta consultas por
	// equipo. Preguntar de a uno era el grueso del costo de crear una
	// recurrencia, y todo antes de escribir la primera fila.
	//
	// No reemplaza a la constraint EXCLUDE, que sigue siendo la garantía ante
	// dos pedidos simultáneos: esto existe para poder decir cuál chocó.
	BuscarSolapamientos(ctx context.Context, equipoIDs []string, fechas []time.Time, horaInicio, horaFin time.Duration) ([]Solapamiento, error)

	// ListarReservasFuturasDeMateria: usado por la cascada de auth
	// (dar de baja al único docente de una materia, RF-02.8) — todas las
	// Reserva CONFIRMADA vinculadas a esa materia a partir de cierta
	// fecha/hora.
	ListarReservasFuturasDeMateria(ctx context.Context, materiaID string, desde time.Time) ([]*domain.Reserva, error)

	// EliminarReservasYGruposDeCiclo: usado por la cascada de archivado de
	// academic (RF-02.4) — borra FÍSICAMENTE (no cancela) toda
	// Reserva/ReservaGrupo vinculada a materias de ese ciclo lectivo, sin
	// importar su estado. Se llama DESPUÉS de que reporting ya calculó y
	// guardó el snapshot histórico — invertir el orden dejaría el
	// snapshot vacío. Devuelve cuántos ReservaGrupo y cuántas Reserva se
	// borraron.
	EliminarReservasYGruposDeCiclo(ctx context.Context, cicloID string) (gruposEliminados int, reservasEliminadas int, err error)

	// ListarReservasConfirmadasVencidas: para el job RF-04.9 — las Reserva
	// CONFIRMADA cuya fecha+horaFin ya pasó respecto a "ahora", de la más
	// vieja a la más nueva y como mucho `limite` filas.
	//
	// El límite no es una optimización: sin él, el job cargaba en memoria y
	// metía en UNA transacción todo lo vencido, y lo vencido crece con cada
	// hora que el proceso esté caído. Ver FinalizarVencidas.
	ListarReservasConfirmadasVencidas(ctx context.Context, ahora time.Time, limite int) ([]*domain.Reserva, error)

	// ── Préstamos (custodia física de una PC) ───────────────────────
	//
	// Viven en este paquete y no en inventory porque casi todas sus reglas
	// son sobre reservas: contra qué reserva se entregó, si volvió antes de
	// que empiece la siguiente, quién es el próximo que la tiene reservada.
	CrearPrestamo(ctx context.Context, p *domain.Prestamo) error
	BuscarPrestamoPorID(ctx context.Context, id string) (*domain.Prestamo, error)
	GuardarPrestamo(ctx context.Context, p *domain.Prestamo) error
	// BuscarPrestamoAbiertoDeEquipo devuelve el préstamo sin devolver de esa PC,
	// o ErrPrestamoNoEncontrado si la máquina está en el laboratorio. Es la
	// forma de responder "¿dónde está la PC 3?" — no hay ninguna columna en
	// `pc` que lo diga, justamente para que no pueda desincronizarse.
	BuscarPrestamoAbiertoDeEquipo(ctx context.Context, equipoID string) (*domain.Prestamo, error)
	// ListarPrestamosAbiertos es "qué hay afuera ahora mismo": la pantalla
	// que reemplaza al papel. Vienen ordenados por hora de devolución, así
	// que lo más atrasado queda arriba.
	ListarPrestamosAbiertos(ctx context.Context) ([]*PrestamoDetallado, error)
	// ListarPrestamosDeEquipo es el historial de una máquina, de lo más reciente
	// a lo más viejo.
	ListarPrestamosDeEquipo(ctx context.Context, equipoID string, limite int) ([]*PrestamoDetallado, error)

	// ── El barrido (RF-08.10 a RF-08.13) ────────────────────────────
	//
	// ReservasAVigilar trae las reservas CONFIRMADA de hoy y mañana, con el
	// contacto del docente y el estado de custodia de cada PC ya resueltos.
	// El filtro es grueso: qué corresponde hacer con cada una lo decide el
	// dominio (CorrespondeRecordar, CorrespondeLiberar…), para que la regla
	// no exista en dos lugares.
	//
	// Mañana también, y no solo hoy, porque el recordatorio sale una hora
	// antes: una clase a las 8 de mañana no necesita nada hoy, pero la
	// consulta no tiene por qué saberlo — lo decide el dominio, y el costo
	// de traer un día más es despreciable.
	ReservasAVigilar(ctx context.Context, hoy time.Time) ([]ReservaParaVigilar, error)
	// PrestamosAVigilar son todos los abiertos, con ubicación y contacto.
	PrestamosAVigilar(ctx context.Context) ([]PrestamoParaVigilar, error)

	// ProximaReservaDeEquipo es a quién le va a faltar esa máquina, con el
	// contacto resuelto. Existe aparte de ListarReservasFuturasDeEquipo porque
	// el corte de fin de jornada necesita MANDARLE UN CORREO al docente, y
	// esa consulta devuelve reservas peladas: sin dirección, el aviso no
	// puede salir. Devuelve nil si no hay ninguna.
	ProximaReservaDeEquipo(ctx context.Context, equipoID string, desde time.Time) (*ProximaReserva, error)

	// Las cinco marcas de idempotencia. Cada una toca UNA columna: el
	// barrido no puede pisar nada que un Admin haya cambiado desde la
	// pantalla mientras el correo salía.
	MarcarRecordatorioEnviado(ctx context.Context, grupoID string, ahora time.Time) error
	MarcarAvisoSinRetirarEnviado(ctx context.Context, grupoID string, ahora time.Time) error
	MarcarAvisoEquipoNoDisponible(ctx context.Context, reservaID string, ahora time.Time) error
	MarcarDemoraAvisada(ctx context.Context, prestamoID string, ahora time.Time) error
	MarcarCierreAvisado(ctx context.Context, prestamoID string, jornada time.Time) error

	// ListarReservas devuelve las reservas que matcheen el filtro, con los
	// nombres de PC, carro, materia y curso ya resueltos. Es lo que
	// necesita un docente para ver sus propias reservas: sin esto la única
	// forma de recuperar una era acordarse del ID del grupo.
	//
	// Devuelve además el total de filas que matchean, no las de la página:
	// es lo que el cliente necesita para saber si hay una página siguiente.
	ListarReservas(ctx context.Context, f FiltroReservas) ([]ReservaDetallada, int, error)

	// CalendarioDeEquipo implementa RF-04.4 — los bloques ocupados de una PC en
	// un rango de fechas, con el nombre del docente y de la materia para
	// poder mostrarlos. Devuelve también los bloqueos administrativos
	// estatal (que no tienen materia).
	CalendarioDeEquipo(ctx context.Context, equipoID string, desde, hasta time.Time) ([]BloqueCalendario, error)

	// ListarEquiposDisponiblesEn implementa el "tildar casillas" de RF-04.2:
	// qué equipos están libres para un día y franja horaria concretos. Sin
	// esto el frontend tendría que pedir el calendario de cada PC por
	// separado y cruzarlo a mano.
	//
	// materiaID entra en la firma por RF-03.21: la lista no tiene un orden
	// único, se ordena PARA una materia (los equipos que la prefieren
	// primero, los que prefieren a otra al final). Vacío es un caso normal
	// —un Admin que reserva sin materia— y devuelve el orden de siempre.
	ListarEquiposDisponiblesEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration, materiaID string) ([]EquipoDisponible, error)

	// ListarEquiposOcupadosEn es la otra mitad de la misma pregunta
	// (RF-04.11): qué equipos de ese universo ya tiene alguien en esa
	// franja, y quién. Sin esto, "no hay nada libre" y "los tiene alguien
	// con quien puedo hablar" se ven igual en pantalla, y solo la segunda
	// tiene salida.
	//
	// Es una consulta aparte y no un OUTER JOIN de la anterior porque las
	// dos listas se muestran en lugares distintos y con columnas distintas:
	// mezclarlas obligaría a que cada fila cargue los campos de la otra en
	// nulo.
	ListarEquiposOcupadosEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) ([]EquipoOcupado, error)

	// ListarEquiposLibresEnLaSerie: los equipos libres en TODAS las fechas
	// que le quedan a la serie de ese grupo, de esa fecha en adelante
	// (RF-08.14). Ofrecer los libres de hoy y rechazar el cambio cuando
	// choca en la tercera fecha es hacerle adivinar al docente.
	ListarEquiposLibresEnLaSerie(ctx context.Context, grupoID string) ([]EquipoDisponible, error)

	// ReservasDeLaSerieDesde: la misma máquina, en todas las ocurrencias que
	// le quedan a la serie a partir de esta (RF-08.14). Devuelve vacío si la
	// reserva no pertenece a ninguna serie — ahí "esta y las siguientes" es
	// lo mismo que "solo esta".
	ReservasDeLaSerieDesde(ctx context.Context, reservaID string) ([]*domain.Reserva, error)

	// DatosParaPedirLiberacion trae, en una sola consulta, todo lo que el
	// pedido de RF-04.12 necesita decidir y decir: en qué estado está la
	// reserva, de quién es, con qué contacto, qué máquina y de qué franja.
	DatosParaPedirLiberacion(ctx context.Context, reservaID string) (*ReservaParaPedido, error)

	// YaPidioLiberacionHoy sostiene la regla de un pedido por reserva, por
	// solicitante y por día (RF-04.12).
	//
	// Se responde mirando las notificaciones ya emitidas y no una tabla
	// propia: un pedido es un mensaje, no una entidad, y la fila que ya se
	// escribe alcanza. La consulta cruza el límite de módulos por la base
	// —lee `notificacion` desde reservation— con el mismo criterio que el
	// JOIN contra `usuario` del barrido: es una lectura de tres columnas,
	// no una regla de negocio de notification.
	YaPidioLiberacionHoy(ctx context.Context, reservaID, solicitanteID string, dia time.Time) (bool, error)

	CrearReglaRecurrencia(ctx context.Context, regla *domain.ReglaRecurrencia) error
	// ListarGruposFuturosDeRegla: los ReservaGrupo de una regla recurrente
	// con fecha posterior a la indicada — usado por "cancelar esta y las
	// siguientes" (RF-04.6).
	ListarGruposFuturosDeRegla(ctx context.Context, reglaID string, desde time.Time) ([]*domain.ReservaGrupo, error)
}

// FiltroReservas acota qué reservas devuelve ListarReservas. Todos los
// campos son opcionales y se combinan con AND; nil significa "sin ese
// filtro".
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
	// Pagina acota cuántas filas vuelven. Es el único listado del sistema
	// que crece con el uso —una reserva por PC, por clase, por semana— y
	// era el que devolvía 2,1 MB en una sola respuesta.
	Pagina paginacion.Pagina
}

// BloqueCalendario es una reserva vista desde el calendario de una PC
// (RF-04.4): además del horario, lleva el nombre del docente y de la
// materia ya resueltos, que es lo que se muestra en pantalla. Los nombres
// se leen con un JOIN de solo lectura, mismo criterio que los validadores
// de este paquete hacia academic/auth.
type BloqueCalendario struct {
	Reserva       *domain.Reserva
	MateriaNombre string
	CursoNombre   string
}

// ReservaDetallada es una Reserva con los nombres ya resueltos para
// mostrarla en pantalla: de qué PC y carro se trata, y de qué materia y
// curso.
//
// Sin esto, "Mis reservas" solo podía mostrar fecha y horario — el DTO
// devolvía equipo_id y materia_id como UUIDs, así que una reserva de ocho equipos
// se veía como ocho tarjetas idénticas e indistinguibles. Mismo criterio
// que BloqueCalendario: un JOIN de solo lectura, sin importar el domain/
// de inventory ni el de academic.
type ReservaDetallada struct {
	Reserva *domain.Reserva
	// Identificador va en 0 y CarroNombre vacío en un equipo suelto: un
	// proyector no está en ningún carro. Lo que se muestra es Etiqueta.
	Identificador int
	CarroNombre   string
	// Etiqueta es cómo se nombra al equipo en pantalla: "PC 3" o "Proyector
	// Epson". La resuelve el repositorio para que la misma cosa no se vea
	// distinta según la pantalla.
	Etiqueta      string
	MateriaNombre string
	CursoNombre   string
	// ReglaRecurrenciaID viene del ReservaGrupo, no de la Reserva. Es lo
	// único que distingue una reserva recurrente de una puntual, y de eso
	// depende si tiene sentido ofrecer "cancelar esta y las siguientes"
	// (RF-04.6).
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
	// Etiqueta es cómo se lo nombra: "PC 3" o "Proyector Epson". Se resuelve
	// del lado del servidor para que la misma máquina no se vea distinta
	// según la pantalla, y para que un proyector no salga rotulado "PC 0".
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
	// preferente, la ajena más fuerte si no. Vacíos y 0 en un equipo neutral.
	PreferenciaMateria  string
	PreferenciaAnio     int
	PreferenciaDivision string
}

// TramoPreferencia agrupa los equipos libres según qué materia los prefiere
// (RF-03.21). No es un permiso: los tres tramos se pueden reservar igual, lo
// único que cambia es el orden y el cartel.
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
//
// Se resuelve del lado del servidor y no en cada cliente por el mismo
// criterio que Etiqueta: el alcance de una marca se arma con tres campos, y
// reconstruir esa frase en cada pantalla es tres oportunidades de que digan
// cosas distintas sobre el mismo dato.
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
// (RF-04.11). Se muestra para poder ir a hablarle o mandarle un pedido, no
// para tildarlo.
//
// De la otra persona viaja el nombre y nunca el email: es el mismo dato que
// ya publica el calendario de cualquier equipo (RF-04.4), y para el pedido no
// hace falta más — el correo lo manda el servidor.
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
	// coincidir con la franja consultada: alguien que necesita el equipo de
	// 10 a 12 tiene que poder ver que quien lo tiene lo usa de 8 a 11.
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
//
// Va en una sola consulta porque son cuatro tablas —reserva, grupo, materia,
// usuario— y pedirlas por separado sería resolver a mano un JOIN para una
// operación que no escribe nada.
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

// ValidadorMateria es el puerto hacia academic — confirma que un docente
// está efectivamente asignado a la materia antes de dejarlo reservar para
// ella (RF-04.1). Nunca se importa internal/academic directamente.
type ValidadorMateria interface {
	DocenteEstaAsignado(ctx context.Context, materiaID, usuarioID string) (bool, error)

	// MateriaAceptaReservas: la materia existe y ni ella, ni su curso, ni
	// su ciclo están archivados. RF-04.1 lo exige aparte de la asignación:
	// "una materia de un ciclo ya cerrado no admite reservas nuevas aunque
	// el registro se conserve".
	MateriaAceptaReservas(ctx context.Context, materiaID string) (bool, error)
}

// ValidadorEquipo es el puerto hacia inventory — confirma que una PC existe y
// está en condiciones de reservarse (estado DISPONIBLE, no dada de baja)
// antes de dejarla incluir en una reserva. Nunca se importa
// internal/inventory directamente.
type ValidadorEquipo interface {
	EquipoDisponibleParaReservar(ctx context.Context, equipoID string) (bool, error)

	// EquiposNoReservables responde lo mismo que EquipoDisponibleParaReservar
	// pero para una lista, en UNA consulta: devuelve cuáles de los pedidos no
	// se pueden reservar (no existen, no están disponibles, están dados de
	// baja o no son reservables). Lista vacía = están todos bien.
	//
	// Existe porque reservar es una operación de LOTE: un docente tilda varias
	// máquinas y un bloqueo administrativo puede tomar un carro entero.
	// Preguntando de a una, bloquear un carro son tantas consultas como
	// equipos tenga, antes de escribir la primera fila — y eso lo dispara el
	// uso normal, no un abuso.
	//
	// Devuelve los que fallan y no un bool para poder decir CUÁLES: con un
	// "alguno no se puede" el docente tiene que adivinar a cuál destildar.
	EquiposNoReservables(ctx context.Context, equipoIDs []string) ([]string, error)

	// EquipoEstaEnInventario es más laxo que EquipoDisponibleParaReservar: solo
	// exige que la PC exista y no esté dada de baja, sin mirar su estado.
	//
	// Es lo que corresponde para ENTREGAR una máquina, que no es lo mismo
	// que reservarla: llevarle al técnico una PC en mantenimiento es
	// justamente un préstamo, y prohibirlo obligaría a sacarla del sistema
	// para poder anotarlo. Lo que sí se rechaza es entregar una que ya no
	// está en el inventario.
	EquipoEstaEnInventario(ctx context.Context, equipoID string) (bool, error)

	// EtiquetasDeEquipos traduce los UUID al nombre con el que la gente
	// reconoce cada cosa ("PC 7", "Proyector Epson"), para poder decir en un
	// aviso cuáles se cancelaron. Los que no existan simplemente no
	// aparecen en el mapa.
	//
	// Devuelve texto y no un número: lo prestable puede no
	// tener identificador, y un proyector rotulado "PC 0" es lo que sale de
	// formatear uno que no existe.
	//
	// Es una lectura de una columna, así que la resuelve el propio
	// infrastructure/ de este paquete con SQL directo sobre `pc`, igual que
	// EquipoDisponibleParaReservar — no hace falta pasar por inventory, que es
	// lo que sí corresponde cuando hay reglas de negocio en juego (ver el
	// comentario de las cascadas al final de service.go).
	EtiquetasDeEquipos(ctx context.Context, equipoIDs []string) (map[string]string, error)
}

// ObtenedorNombreDocente es el puerto hacia auth — solo necesitamos el
// nombre completo para el snapshot (nombre_docente_snapshot), no el resto
// de la lógica de auth. Mismo criterio que ValidadorUsuarioPostgres de
// academic: una consulta de una sola fila a la tabla usuario, sin
// importar internal/auth.
type ObtenedorNombreDocente interface {
	NombreCompletoDe(ctx context.Context, usuarioID string) (string, error)
}

type IDGenerator func() string

// PrestamoDetallado es un Prestamo con lo mínimo para saber de qué máquina
// habla y de dónde salió, resuelto por JOIN — mismo criterio que
// ReservaDetallada.
//
// La ubicación va siempre: un renglón que dice "entregada a Ana Pérez" sin
// decir qué computadora no sirve para nada, y es exactamente lo que el papel
// sí anota.
type PrestamoDetallado struct {
	Prestamo *domain.Prestamo
	// Identificador va en 0 para un equipo suelto. Lo que se muestra es
	// Etiqueta: un proyector rotulado "PC 0" es lo que sale de formatear un
	// identificador que no existe.
	Identificador int
	Etiqueta      string
	// CarroNombre vacío en un equipo suelto.
	CarroNombre string
	// MateriaNombre solo en los préstamos que salieron contra una reserva.
	// Nil en los espontáneos —no hay materia— y también en los que la
	// tenían pero cuya reserva ya se borró al archivar el ciclo lectivo
	// (RF-02.4): el préstamo sobrevive, la reserva no.
	MateriaNombre *string
}

// ══════════════════════════════════════════════════════════════════
// El barrido (RF-08.10 a RF-08.13)
// ══════════════════════════════════════════════════════════════════
//
// Son DOS consultas y no cinco a propósito. Los cinco avisos del barrido
// —recordar, liberar, reclamar al que no devolvió, avisarle al docente
// siguiente y el corte de fin de jornada— miran las mismas dos cosas: las
// reservas confirmadas de hoy y las máquinas que están afuera. Partirlo en
// una consulta por aviso significaría releer lo mismo cinco veces por
// barrido y, peor, dejar que cada una filtrara con un criterio apenas
// distinto.

// ReservaParaVigilar es una reserva confirmada con todo lo que el barrido
// necesita: para decidir, y para poder avisar sin volver a la base.
//
// El contacto del docente viaja resuelto porque el correo lo manda
// notification, que no puede importar el domain de auth (docs/06 §3): o
// viaja en el evento, o habría que agregar otro puerto de lectura para algo
// que esta consulta ya tiene a mano.
type ReservaParaVigilar struct {
	ReservaID string
	GrupoID   *string
	EquipoID  string
	// Identificador es el número visible ("PC 7"), que es lo que el
	// docente reconoce.
	Identificador int
	// Etiqueta es cómo se nombra al equipo en un aviso: "PC 7" o "Proyector
	// Epson". Un proyector rotulado "PC 0" es lo que sale de formatear un
	// identificador que no existe.
	Etiqueta   string
	Fecha      time.Time
	HoraInicio time.Duration
	HoraFin    time.Duration
	// Tipo distingue la clase de un docente de un bloqueo administrativo
	// estatal. Importa: un bloqueo no lo retira nadie, así que ni se
	// recuerda ni se libera.
	Tipo          domain.TipoReserva
	MateriaNombre *string

	DocenteID     *string
	DocenteNombre string
	DocenteEmail  string

	RecordatorioEnviado            bool
	AvisoEquipoNoDisponibleEnviado bool
	// AvisoSinRetirarEnviado: ya salió el aviso de "todavía no las
	// retiraste" (RF-08.20). Vive en el grupo, como el recordatorio: es uno
	// por clase, no uno por equipo.
	AvisoSinRetirarEnviado bool

	// EquipoAfuera: hay un préstamo sin devolver sobre esa máquina. Es lo que
	// distingue "el docente no vino" de "el docente vino y se la llevó", y
	// también lo que impide liberar una reserva cuya PC está en manos de
	// alguien.
	EquipoAfuera bool
	// EquipoDebioVolverA es la hora en que esa máquina tenía que estar de vuelta.
	// nil si está adentro, o si salió sin hora pactada.
	EquipoDebioVolverA *time.Time

	// UltimaEntregaDelGrupo es cuándo se entregó por última vez alguna
	// máquina CONTRA ESTA RESERVA. Es el dato que distingue "el docente no
	// vino" de "vino y se llevó una parte", y de ahí sale el plazo corto de
	// liberación (RF-08.10) y el silencio del aviso (RF-08.20).
	//
	// Se mira por reserva y no por equipo, al revés que EquipoAfuera: una
	// máquina prestada a otra persona por un trámite no dice nada sobre si
	// este docente vino a dar su clase. Y no filtra por devuelto_en, porque
	// lo que importa es que la entrega ocurrió: que ya la haya devuelto no
	// lo vuelve un docente que no vino.
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
	// Email vacío si quien se la llevó no tiene cuenta en el sistema — el
	// caso normal de un préstamo para un trámite. Sin correo no se le puede
	// reclamar a esa persona, pero el reclamo a los Admin sale igual.
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
//
// Existe aparte de domain.Reserva porque lo que hace falta acá es explicarle
// el choque a quien reserva, no operar sobre la fila: la etiqueta del equipo
// ya resuelta y el nombre de quien la tiene, en vez de dos UUID.
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
