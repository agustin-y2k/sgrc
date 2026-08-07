package application

import (
	"context"
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
	// completa, bloquear varias PCs para una evaluación — porque a mitad
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

	// ListarReservasFuturasDePC: usado tanto por la cascada de inventory
	// (cambio de estado / baja de una PC) como por el bloqueo de
	// evaluación — todas las Reserva CONFIRMADA de una PC a partir de
	// cierta fecha/hora.
	ListarReservasFuturasDePC(ctx context.Context, pcID string, desde time.Time) ([]*domain.Reserva, error)

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
	// BuscarPrestamoAbiertoDePC devuelve el préstamo sin devolver de esa PC,
	// o ErrPrestamoNoEncontrado si la máquina está en el laboratorio. Es la
	// forma de responder "¿dónde está la PC 3?" — no hay ninguna columna en
	// `pc` que lo diga, justamente para que no pueda desincronizarse.
	BuscarPrestamoAbiertoDePC(ctx context.Context, pcID string) (*domain.Prestamo, error)
	// ListarPrestamosAbiertos es "qué hay afuera ahora mismo": la pantalla
	// que reemplaza al papel. Vienen ordenados por hora de devolución, así
	// que lo más atrasado queda arriba.
	ListarPrestamosAbiertos(ctx context.Context) ([]*PrestamoDetallado, error)
	// ListarPrestamosDePC es el historial de una máquina, de lo más reciente
	// a lo más viejo.
	ListarPrestamosDePC(ctx context.Context, pcID string, limite int) ([]*PrestamoDetallado, error)

	// ListarReservas devuelve las reservas que matcheen el filtro, con los
	// nombres de PC, carro, materia y curso ya resueltos. Es lo que
	// necesita un docente para ver sus propias reservas: sin esto la única
	// forma de recuperar una era acordarse del ID del grupo.
	//
	// Devuelve además el total de filas que matchean, no las de la página:
	// es lo que el cliente necesita para saber si hay una página siguiente.
	ListarReservas(ctx context.Context, f FiltroReservas) ([]ReservaDetallada, int, error)

	// CalendarioDePC implementa RF-04.4 — los bloques ocupados de una PC en
	// un rango de fechas, con el nombre del docente y de la materia para
	// poder mostrarlos. Devuelve también los bloqueos por evaluación
	// estatal (que no tienen materia).
	CalendarioDePC(ctx context.Context, pcID string, desde, hasta time.Time) ([]BloqueCalendario, error)

	// ListarPCsDisponiblesEn implementa el "tildar casillas" de RF-04.2:
	// qué PCs están libres para un día y franja horaria concretos. Sin
	// esto el frontend tendría que pedir el calendario de cada PC por
	// separado y cruzarlo a mano.
	ListarPCsDisponiblesEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) ([]PCDisponible, error)

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
	PCID      *string
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
// devolvía pc_id y materia_id como UUIDs, así que una reserva de ocho PCs
// se veía como ocho tarjetas idénticas e indistinguibles. Mismo criterio
// que BloqueCalendario: un JOIN de solo lectura, sin importar el domain/
// de inventory ni el de academic.
type ReservaDetallada struct {
	Reserva         *domain.Reserva
	PCIdentificador int
	CarroNombre     string
	MateriaNombre   string
	CursoNombre     string
	// ReglaRecurrenciaID viene del ReservaGrupo, no de la Reserva. Es lo
	// único que distingue una reserva recurrente de una puntual, y de eso
	// depende si tiene sentido ofrecer "cancelar esta y las siguientes"
	// (RF-04.6).
	ReglaRecurrenciaID *string
}

// PCDisponible es una PC libre en la franja consultada, con los datos que
// RF-03.7 dice que el docente necesita para elegir (software instalado,
// freezado) sin tener que pedirlos a inventory por separado.
type PCDisponible struct {
	PCID              string
	Identificador     int
	CarroID           string
	CarroNombre       string
	Freezado          bool
	SoftwareInstalado string
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

// ValidadorPC es el puerto hacia inventory — confirma que una PC existe y
// está en condiciones de reservarse (estado DISPONIBLE, no dada de baja)
// antes de dejarla incluir en una reserva. Nunca se importa
// internal/inventory directamente.
type ValidadorPC interface {
	PCDisponibleParaReservar(ctx context.Context, pcID string) (bool, error)

	// PCEstaEnInventario es más laxo que PCDisponibleParaReservar: solo
	// exige que la PC exista y no esté dada de baja, sin mirar su estado.
	//
	// Es lo que corresponde para ENTREGAR una máquina, que no es lo mismo
	// que reservarla: llevarle al técnico una PC en mantenimiento es
	// justamente un préstamo, y prohibirlo obligaría a sacarla del sistema
	// para poder anotarlo. Lo que sí se rechaza es entregar una que ya no
	// está en el inventario.
	PCEstaEnInventario(ctx context.Context, pcID string) (bool, error)

	// IdentificadoresDePCs traduce los UUID de PC al número visible que la
	// gente reconoce ("PC 7"), para poder decir en un aviso cuáles se
	// cancelaron. Los que no existan simplemente no aparecen en el mapa.
	//
	// Es una lectura de una columna, así que la resuelve el propio
	// infrastructure/ de este paquete con SQL directo sobre `pc`, igual que
	// PCDisponibleParaReservar — no hace falta pasar por inventory, que es
	// lo que sí corresponde cuando hay reglas de negocio en juego (ver el
	// comentario de las cascadas al final de service.go).
	IdentificadoresDePCs(ctx context.Context, pcIDs []string) (map[string]int, error)
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
	Prestamo        *domain.Prestamo
	PCIdentificador int
	CarroNombre     string
	// MateriaNombre solo en los préstamos que salieron contra una reserva.
	// Nil en los espontáneos —no hay materia— y también en los que la
	// tenían pero cuya reserva ya se borró al archivar el ciclo lectivo
	// (RF-02.4): el préstamo sobrevive, la reserva no.
	MateriaNombre *string
}
