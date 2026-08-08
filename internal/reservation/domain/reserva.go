package domain

import (
	"errors"
	"fmt"
	"time"
)

// EstadoReserva es el estado de una PC puntual dentro de un ReservaGrupo
// (o de un bloqueo de evaluación estatal, que no pertenece a ningún
// grupo). Más simple que EstadoReservaGrupo — no hay "parcial" a este
// nivel, una Reserva individual está confirmada, cancelada, o finalizada.
type EstadoReserva string

const (
	ReservaConfirmada EstadoReserva = "CONFIRMADA"
	ReservaCancelada  EstadoReserva = "CANCELADA"
	ReservaFinalizada EstadoReserva = "FINALIZADA"
	// ReservaNoRetirada: nadie vino a buscar la máquina dentro del plazo de
	// gracia, así que dejó de bloquear el horario (RF-08.10).
	//
	// Es un estado propio y no CANCELADA porque son dos noticias distintas
	// para quien las lee: "te la cancelaron" pide saber quién y por qué,
	// "no la retiraste" se explica sola. Y porque el reporte de uso
	// (RF-06.1) puede dejar de contarla como una clase dada, que es lo que
	// hoy hace.
	ReservaNoRetirada EstadoReserva = "NO_RETIRADA"
)

var ErrEstadoReservaInvalido = errors.New("estado de reserva inválido")

func ParseEstadoReserva(s string) (EstadoReserva, error) {
	switch EstadoReserva(s) {
	case ReservaConfirmada, ReservaCancelada, ReservaFinalizada, ReservaNoRetirada:
		return EstadoReserva(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrEstadoReservaInvalido, s)
	}
}

// PuedeTransicionarA: CONFIRMADA puede cancelarse, finalizar o liberarse por
// no retiro; los otros tres son terminales.
//
// NO_RETIRADA no vuelve a CONFIRMADA aunque el docente aparezca a los
// cincuenta minutos: liberar no es prohibir. Si las máquinas siguen ahí se
// las entregás igual, y eso queda registrado como un préstamo (RF-08) —
// que es otra cosa que la reserva.
func (e EstadoReserva) PuedeTransicionarA(nuevo EstadoReserva) bool {
	return e == ReservaConfirmada &&
		(nuevo == ReservaCancelada || nuevo == ReservaFinalizada || nuevo == ReservaNoRetirada)
}

var ErrTransicionReservaInvalida = errors.New("transición de estado de reserva inválida")

// TipoReserva distingue una reserva normal (docente/materia) de un
// bloqueo administrativo por evaluación estatal — este último no
// pertenece a ningún ReservaGrupo ni Materia (RF-04.7).
type TipoReserva string

const (
	TipoNormal            TipoReserva = "NORMAL"
	TipoEvaluacionEstatal TipoReserva = "EVALUACION_ESTATAL"
)

var ErrTipoReservaInvalido = errors.New("tipo de reserva inválido")

func ParseTipoReserva(s string) (TipoReserva, error) {
	switch TipoReserva(s) {
	case TipoNormal, TipoEvaluacionEstatal:
		return TipoReserva(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrTipoReservaInvalido, s)
	}
}

// Reserva es la ocupación puntual de una PC concreta en un rango
// horario. Es la unidad real que protege la constraint EXCLUDE de
// anti-solapamiento en la base (docs/07-modelo-datos.md) — el chequeo de
// solapamiento en sí NO se hace acá en dominio, porque requiere consultar
// todas las demás reservas de esa PC (responsabilidad de application/ +
// infrastructure/, donde vive la constraint real).
type Reserva struct {
	ID                    string
	ReservaGrupoID        *string
	PCID                  string
	MateriaID             *string
	NombreDocenteSnapshot *string
	Fecha                 time.Time
	HoraInicio            time.Duration
	HoraFin               time.Duration
	Estado                EstadoReserva
	Tipo                  TipoReserva
	CreadoPor             *string
	CreadaEn              time.Time
	CanceladoPor          *string
	MotivoCancelacion     *string
	CanceladaEn           *time.Time
}

// NuevaReservaNormal crea una Reserva perteneciente a un ReservaGrupo
// (RF-04.1) — reservaGrupoID y materiaID son obligatorios acá, a
// diferencia de un bloqueo de evaluación.
func NuevaReservaNormal(id, reservaGrupoID, pcID, materiaID string, nombreDocenteSnapshot string, creadoPor *string, fecha time.Time, horaInicio, horaFin time.Duration, ahora time.Time) (*Reserva, error) {
	if horaFin <= horaInicio {
		return nil, ErrRangoHorarioInvalido
	}
	return &Reserva{
		ID:                    id,
		ReservaGrupoID:        &reservaGrupoID,
		PCID:                  pcID,
		MateriaID:             &materiaID,
		NombreDocenteSnapshot: &nombreDocenteSnapshot,
		Fecha:                 fecha,
		HoraInicio:            horaInicio,
		HoraFin:               horaFin,
		Estado:                ReservaConfirmada,
		Tipo:                  TipoNormal,
		CreadoPor:             creadoPor,
		CreadaEn:              ahora,
	}, nil
}

// NuevaReservaEvaluacion crea un bloqueo administrativo sobre una PC
// puntual, sin pertenecer a ningún ReservaGrupo ni Materia (RF-04.7).
func NuevaReservaEvaluacion(id, pcID string, creadoPor *string, fecha time.Time, horaInicio, horaFin time.Duration, ahora time.Time) (*Reserva, error) {
	if horaFin <= horaInicio {
		return nil, ErrRangoHorarioInvalido
	}
	return &Reserva{
		ID:         id,
		PCID:       pcID,
		Fecha:      fecha,
		HoraInicio: horaInicio,
		HoraFin:    horaFin,
		Estado:     ReservaConfirmada,
		Tipo:       TipoEvaluacionEstatal,
		CreadoPor:  creadoPor,
		CreadaEn:   ahora,
	}, nil
}

// Cancelar aplica la transición y deja registro de quién canceló, cuándo,
// y por qué (RF-04.4/04.5/04.6 — motivo obligatorio en todos los casos,
// aunque algunos lo generen el propio sistema, ej. cascada de evaluación).
func (r *Reserva) Cancelar(canceladoPor *string, motivo string, ahora time.Time) error {
	if err := r.cambiarEstado(ReservaCancelada); err != nil {
		return err
	}
	r.CanceladoPor = canceladoPor
	r.MotivoCancelacion = &motivo
	r.CanceladaEn = &ahora
	return nil
}

// Finalizar marca la reserva como concluida (el job de vencimiento la
// llama una vez que pasó su hora de fin, RF-04.9) — no lleva motivo ni
// "cancelado por", es una transición natural del paso del tiempo.
func (r *Reserva) Finalizar() error {
	return r.cambiarEstado(ReservaFinalizada)
}

// Liberar marca que nadie vino a buscar la máquina dentro del plazo de
// gracia (RF-08.10). Con eso deja de bloquear el horario, porque la
// constraint de anti-solapamiento solo mira las CONFIRMADA.
//
// No lleva quién ni por qué, a diferencia de Cancelar: no lo decidió nadie.
func (r *Reserva) Liberar() error {
	return r.cambiarEstado(ReservaNoRetirada)
}

func (r *Reserva) cambiarEstado(nuevo EstadoReserva) error {
	if !r.Estado.PuedeTransicionarA(nuevo) {
		return fmt.Errorf("%w: de %s a %s", ErrTransicionReservaInvalida, r.Estado, nuevo)
	}
	r.Estado = nuevo
	return nil
}

// SolapaCon indica si el rango horario de esta reserva se superpone con
// otro rango dado — útil para validaciones en application/ antes de
// llegar a la constraint de la base (da un error de negocio más claro que
// esperar el 500/409 crudo de Postgres).
func (r *Reserva) SolapaCon(horaInicio, horaFin time.Duration) bool {
	return r.HoraInicio < horaFin && horaInicio < r.HoraFin
}

// MaxDuracionReserva acota cuánto puede durar un solo bloque. No hay
// módulos horarios fijos en la escuela —hora_inicio/hora_fin son libres a
// propósito— así que el tope no puede ser "un módulo": es el largo de un
// turno completo, que es lo máximo que una clase real puede ocupar. Sin
// tope, un 00:00–23:59 se aceptaba sin chistar y dejaba la PC bloqueada
// todo el día contra la constraint EXCLUDE.
const MaxDuracionReserva = 8 * time.Hour

var (
	// ErrReservaEnElPasado: el bloque pedido ya terminó al momento de
	// crearlo. El job de RF-04.9 lo finalizaba en el siguiente ciclo, pero
	// mientras tanto ocupaba el slot de la constraint EXCLUDE y ensuciaba
	// los reportes con clases que nunca se dieron.
	ErrReservaEnElPasado = errors.New("no se puede reservar un horario que ya terminó")

	// ErrDuracionExcesiva acompaña a MaxDuracionReserva.
	ErrDuracionExcesiva = fmt.Errorf("la reserva no puede durar más de %d horas", int(MaxDuracionReserva.Hours()))
)

// YaTermino dice si el bloque (fecha, horaFin) quedó atrás respecto de
// ahora. Es el mismo criterio que condicionNoTerminada en
// infrastructure/ —"fecha + hora_fin contra el instante actual", no la
// fecha pelada— para que crear y cancelar en cascada no usen dos
// definiciones distintas de "ya pasó".
//
// La comparación es de hora de pared, no de instantes: Fecha llega como
// medianoche UTC (solo importan año/mes/día, ver interfaces/http.parseFecha)
// mientras que ahora viene en APP_TIMEZONE. Comparar los time.Time crudos
// mezclaría las dos zonas y correría el límite del día tantas horas como el
// offset de la escuela.
func YaTermino(fecha time.Time, horaFin time.Duration, ahora time.Time) bool {
	return !horaDePared(fecha, horaFin).After(horaDePared(ahora, horaDelDia(ahora)))
}

func horaDePared(fecha time.Time, hora time.Duration) time.Time {
	y, m, d := fecha.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Add(hora)
}

// InstanteDePared convierte una fecha de calendario más una hora de pared
// en el instante real, en la zona indicada.
//
// Hace falta porque `fecha` es un DATE y `hora_fin` un TIME: juntos
// significan "las 9 de la mañana de la escuela", no un momento absoluto.
// Para guardar ese momento en una columna TIMESTAMPTZ —la hora en que una
// máquina debería volver— hay que resolverlo contra una zona, y la que
// corresponde es la de la institución (APP_TIMEZONE), no la del servidor.
func InstanteDePared(fecha time.Time, hora time.Duration, loc *time.Location) time.Time {
	y, m, d := fecha.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc).Add(hora)
}

func horaDelDia(t time.Time) time.Duration {
	return time.Duration(t.Hour())*time.Hour +
		time.Duration(t.Minute())*time.Minute +
		time.Duration(t.Second())*time.Second
}

// ValidarVentanaTemporal reúne las tres reglas que todo bloque tiene que
// cumplir, sea una reserva normal o un bloqueo por evaluación: rango
// horario coherente, duración acotada y que no esté en el pasado.
func ValidarVentanaTemporal(fecha time.Time, horaInicio, horaFin time.Duration, ahora time.Time) error {
	if horaFin <= horaInicio {
		return ErrRangoHorarioInvalido
	}
	if horaFin-horaInicio > MaxDuracionReserva {
		return ErrDuracionExcesiva
	}
	if YaTermino(fecha, horaFin, ahora) {
		return ErrReservaEnElPasado
	}
	return nil
}
