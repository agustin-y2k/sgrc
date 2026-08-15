package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// EstadoReserva es el estado de una PC puntual dentro de un ReservaGrupo
// (o de un bloqueo administrativo, que no pertenece a ningún grupo). Más simple que EstadoReservaGrupo — no hay "parcial" a este
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

// TipoReserva distingue la reserva de un docente para su clase de un
// bloqueo administrativo — este último no pertenece a ningún ReservaGrupo ni
// Materia (RF-04.7), y lleva su propio motivo.
//
// El tipo no nombra el motivo, y no es un descuido: un Admin se toma el
// laboratorio por una evaluación, por una jornada docente, por una
// capacitación o por una obra en el aula. Lo que esos casos tienen en común
// no es ninguna categoría, es que alguien con autoridad decidió que ese rato
// el equipo se usa para otra cosa. Por eso el motivo va en texto libre y
// obligatorio, en MotivoBloqueo.
type TipoReserva string

const (
	TipoNormal  TipoReserva = "NORMAL"
	TipoBloqueo TipoReserva = "BLOQUEO"
)

// MaxLargoMotivoBloqueo coincide con lo que se acepta en un motivo de
// cancelación: los dos son la misma clase de texto —una explicación corta
// que va a leer un docente— y tener dos topes distintos solo sorprende.
const MaxLargoMotivoBloqueo = 500

var (
	ErrTipoReservaInvalido = errors.New("tipo de reserva inválido")

	// ErrMotivoBloqueoVacio: un bloqueo cancela las clases de otros, así que
	// el porqué no es opcional. Quien tiene la autoridad para tomarse el
	// laboratorio puede escribir para qué.
	ErrMotivoBloqueoVacio = errors.New("hay que indicar por qué se bloquean los equipos")
	ErrMotivoBloqueoLargo = fmt.Errorf("el motivo no puede tener más de %d caracteres", MaxLargoMotivoBloqueo)
)

func ParseTipoReserva(s string) (TipoReserva, error) {
	switch TipoReserva(s) {
	case TipoNormal, TipoBloqueo:
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
	EquipoID              string
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

	// MotivoBloqueo: por qué se tomó el equipo. Solo en los TipoBloqueo, y
	// ahí es obligatorio; vacío en las normales, que ya dicen para qué son
	// por su materia.
	MotivoBloqueo string
}

// NuevaReservaNormal crea una Reserva perteneciente a un ReservaGrupo
// (RF-04.1) — reservaGrupoID y materiaID son obligatorios acá, a
// diferencia de un bloqueo administrativo.
func NuevaReservaNormal(id, reservaGrupoID, equipoID, materiaID string, nombreDocenteSnapshot string, creadoPor *string, fecha time.Time, horaInicio, horaFin time.Duration, ahora time.Time) (*Reserva, error) {
	if horaFin == horaInicio {
		return nil, ErrRangoHorarioInvalido
	}
	return &Reserva{
		ID:                    id,
		ReservaGrupoID:        &reservaGrupoID,
		EquipoID:              equipoID,
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

// NuevaReservaDeBloqueo crea un bloqueo administrativo sobre una PC
// puntual, sin pertenecer a ningún ReservaGrupo ni Materia (RF-04.7).
func NuevaReservaBloqueo(id, equipoID string, creadoPor *string, fecha time.Time, horaInicio, horaFin time.Duration, motivo string, ahora time.Time) (*Reserva, error) {
	if horaFin == horaInicio {
		return nil, ErrRangoHorarioInvalido
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return nil, ErrMotivoBloqueoVacio
	}
	if len([]rune(motivo)) > MaxLargoMotivoBloqueo {
		return nil, ErrMotivoBloqueoLargo
	}
	return &Reserva{
		ID:            id,
		EquipoID:      equipoID,
		Fecha:         fecha,
		HoraInicio:    horaInicio,
		HoraFin:       horaFin,
		Estado:        ReservaConfirmada,
		Tipo:          TipoBloqueo,
		MotivoBloqueo: motivo,
		CreadoPor:     creadoPor,
		CreadaEn:      ahora,
	}, nil
}

// Cancelar aplica la transición y deja registro de quién canceló, cuándo,
// y por qué (RF-04.4/04.5/04.6 — motivo obligatorio en todos los casos,
// aunque algunos lo generen el propio sistema, ej. cascada de un bloqueo).
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
// Los dos bloques se miden como offsets desde la MISMA medianoche, y por eso
// el fin puede pasar de las 24 horas: 22:00–01:00 es [22h, 25h). Comparar
// hora_fin cruda daría 01:00 < 22:00 y concluiría que no se pisan con nada,
// que es justamente el bug que habilitaría reservar encima de una clase
// nocturna.
//
// Solo compara bloques de la MISMA fecha, que es para lo que se usa (el
// pre-chequeo al crear un grupo). Un bloque de la noche anterior que se
// mete en esta madrugada lo detectan la constraint EXCLUDE de la base y la
// consulta de solapamiento, que trabajan con instantes absolutos.
func (r *Reserva) SolapaCon(horaInicio, horaFin time.Duration) bool {
	finPropio := r.HoraInicio + DuracionDe(r.HoraInicio, r.HoraFin)
	finOtro := horaInicio + DuracionDe(horaInicio, horaFin)
	return r.HoraInicio < finOtro && horaInicio < finPropio
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
// Recibe también horaInicio porque sin ella no se sabe de qué día es el fin:
// un bloque de 22:00 a 01:00 termina la madrugada SIGUIENTE, y compararlo
// contra "fecha + 01:00" lo daría por terminado veintiuna horas antes de que
// empiece. Ese error no se ve como un error: la clase nocturna aparece
// finalizada apenas se crea.
func YaTermino(fecha time.Time, horaInicio, horaFin time.Duration, ahora time.Time) bool {
	fin := horaDePared(fecha, horaFin)
	if CruzaMedianoche(horaInicio, horaFin) {
		fin = fin.AddDate(0, 0, 1)
	}
	return !fin.After(horaDePared(ahora, horaDelDia(ahora)))
}

// YaEmpezo es la mitad de arriba de lo mismo: la franja ya arrancó. Lo usa el
// pedido de liberación (RF-04.12), que no tiene sentido sobre una clase en
// curso — el docente ya está usando esas máquinas.
//
// Misma comparación de hora de pared que YaTermino, y por la misma razón.
func YaEmpezo(fecha time.Time, horaInicio time.Duration, ahora time.Time) bool {
	return !horaDePared(fecha, horaInicio).After(horaDePared(ahora, horaDelDia(ahora)))
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

// ═══════════════════════════════════════════════════════════════════════
// Bloques que cruzan la medianoche
// ═══════════════════════════════════════════════════════════════════════
//
// Una escuela nocturna dicta de 22:00 a 01:00. Hasta acá eso era
// inexpresable: el sistema exigía hora_fin > hora_inicio, así que la única
// salida era partir la clase en dos reservas —una hasta las 23:59 y otra
// desde las 00:00 del día siguiente— que el sistema trataba como dos cosas
// sin relación. Cancelar una dejaba la otra viva, los reportes contaban dos
// clases donde hubo una, y el equipo aparecía devuelto a medianoche.
//
// La regla es la que cualquiera lee sin que se la expliquen:
//
//	hora_fin > hora_inicio  → termina el mismo día (22:00–23:00)
//	hora_fin < hora_inicio  → termina al día siguiente (22:00–01:00)
//	hora_fin = hora_inicio  → inválido
//
// El caso de la igualdad podría significar "veinticuatro horas", y se
// rechaza igual: nadie escribe 08:00–08:00 queriendo un día entero, y el
// tope de duración lo rechazaría después con un mensaje sobre las horas que
// no explicaría el verdadero problema, que es un tipeo.
//
// La misma regla vive en el esquema, en la función fin_de_pared() que usan
// la constraint de anti-solapamiento y todas las consultas. Las dos tienen
// que decir lo mismo: si divergen, la aplicación acepta reservas que la base
// rechaza, o peor, deja pasar solapamientos que creía haber chequeado.

// CruzaMedianoche dice si el bloque termina al día siguiente del que lo
// nombra.
func CruzaMedianoche(horaInicio, horaFin time.Duration) bool {
	return horaFin < horaInicio
}

// DuracionDe es cuánto dura el bloque, cruce o no la medianoche. Para
// 22:00–01:00 son tres horas, no menos veintiuna.
func DuracionDe(horaInicio, horaFin time.Duration) time.Duration {
	if CruzaMedianoche(horaInicio, horaFin) {
		return 24*time.Hour - horaInicio + horaFin
	}
	return horaFin - horaInicio
}

// FinDePared es el instante en que el bloque termina de verdad: la fecha que
// lo nombra más la hora de fin, más un día si cruzó la medianoche.
//
// Es la contracara de InicioDePared y el reemplazo de "fecha + hora_fin",
// que era correcto mientras ningún bloque pudiera pasar de las 00:00 y ahora
// devolvería un fin ANTERIOR al inicio.
func FinDePared(fecha time.Time, horaInicio, horaFin time.Duration, loc *time.Location) time.Time {
	fin := InstanteDePared(fecha, horaFin, loc)
	if CruzaMedianoche(horaInicio, horaFin) {
		return fin.AddDate(0, 0, 1)
	}
	return fin
}

// InicioDePared existe por simetría con FinDePared: en los lugares donde se
// usan los dos juntos, ver "InstanteDePared" de un lado y "FinDePared" del
// otro invita a leerlos como si hicieran cosas distintas.
func InicioDePared(fecha time.Time, horaInicio time.Duration, loc *time.Location) time.Time {
	return InstanteDePared(fecha, horaInicio, loc)
}

// ValidarVentanaTemporal reúne las tres reglas que todo bloque tiene que
// cumplir, sea una reserva normal o un bloqueo administrativo: rango
// horario coherente, duración acotada y que no esté en el pasado.
func ValidarVentanaTemporal(fecha time.Time, horaInicio, horaFin time.Duration, ahora time.Time) error {
	if horaFin == horaInicio {
		return ErrRangoHorarioInvalido
	}
	if DuracionDe(horaInicio, horaFin) > MaxDuracionReserva {
		return ErrDuracionExcesiva
	}
	if YaTermino(fecha, horaInicio, horaFin, ahora) {
		return ErrReservaEnElPasado
	}
	return nil
}
