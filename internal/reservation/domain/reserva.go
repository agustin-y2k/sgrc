package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// EstadoReserva es el estado de una PC puntual dentro de un ReservaGrupo (o
// de un bloqueo administrativo, que no pertenece a ningún grupo).
type EstadoReserva string

const (
	ReservaConfirmada EstadoReserva = "CONFIRMADA"
	ReservaCancelada  EstadoReserva = "CANCELADA"
	ReservaFinalizada EstadoReserva = "FINALIZADA"
	// ReservaNoRetirada: nadie vino a buscar la máquina dentro del plazo de
	// gracia, así que dejó de bloquear el horario (RF-08.10).
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
func (e EstadoReserva) PuedeTransicionarA(nuevo EstadoReserva) bool {
	return e == ReservaConfirmada &&
		(nuevo == ReservaCancelada || nuevo == ReservaFinalizada || nuevo == ReservaNoRetirada)
}

var ErrTransicionReservaInvalida = errors.New("transición de estado de reserva inválida")

// TipoReserva distingue la reserva de un docente para su clase de un bloqueo
// administrativo — este último no pertenece a ningún ReservaGrupo ni Materia
// (RF-04.7), y lleva su propio motivo.
type TipoReserva string

const (
	TipoNormal  TipoReserva = "NORMAL"
	TipoBloqueo TipoReserva = "BLOQUEO"
)

// MaxLargoMotivoBloqueo coincide con lo que se acepta en un motivo de
// cancelación: los dos son la misma clase de texto —una explicación corta que
// va a leer un docente— y tener dos topes distintos solo sorprende.
const MaxLargoMotivoBloqueo = 500

var (
	ErrTipoReservaInvalido = errors.New("tipo de reserva inválido")

	// ErrMotivoBloqueoVacio: un bloqueo cancela las clases de otros, así que el
	// porqué no es opcional.
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

// Reserva es la ocupación puntual de una PC concreta en un rango horario.
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

	// MotivoBloqueo: por qué se tomó el equipo.
	MotivoBloqueo string
}

// NuevaReservaNormal crea una Reserva perteneciente a un ReservaGrupo
// (RF-04.1) — reservaGrupoID y materiaID son obligatorios acá, a diferencia
// de un bloqueo administrativo.
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

// Cancelar aplica la transición y deja registro de quién canceló, cuándo, y
// por qué (RF-04.4/04.5/04.6 — motivo obligatorio en todos los casos, aunque
// algunos lo generen el propio sistema, ej.
func (r *Reserva) Cancelar(canceladoPor *string, motivo string, ahora time.Time) error {
	if err := r.cambiarEstado(ReservaCancelada); err != nil {
		return err
	}
	r.CanceladoPor = canceladoPor
	r.MotivoCancelacion = &motivo
	r.CanceladaEn = &ahora
	return nil
}

// Finalizar marca la reserva como concluida (el job de vencimiento la llama
// una vez que pasó su hora de fin, RF-04.9) — no lleva motivo ni "cancelado
// por", es una transición natural del paso del tiempo.
func (r *Reserva) Finalizar() error {
	return r.cambiarEstado(ReservaFinalizada)
}

// Liberar marca que nadie vino a buscar la máquina dentro del plazo de gracia
// (RF-08.10).
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

// SolapaCon indica si el rango horario de esta reserva se superpone con otro
// rango dado — útil para validaciones en application/ antes de llegar a la
// constraint de la base (da un error de negocio más claro que esperar el
// 500/409 crudo de Postgres).
func (r *Reserva) SolapaCon(horaInicio, horaFin time.Duration) bool {
	finPropio := r.HoraInicio + DuracionDe(r.HoraInicio, r.HoraFin)
	finOtro := horaInicio + DuracionDe(horaInicio, horaFin)
	return r.HoraInicio < finOtro && horaInicio < finPropio
}

// MaxDuracionReserva acota cuánto puede durar un solo bloque.
const MaxDuracionReserva = 8 * time.Hour

var (
	// ErrReservaEnElPasado: el bloque pedido ya terminó al momento de crearlo.
	ErrReservaEnElPasado = errors.New("no se puede reservar un horario que ya terminó")

	// ErrDuracionExcesiva acompaña a MaxDuracionReserva.
	ErrDuracionExcesiva = fmt.Errorf("la reserva no puede durar más de %d horas", int(MaxDuracionReserva.Hours()))
)

// YaTermino dice si el bloque (fecha, horaFin) quedó atrás respecto de ahora.
func YaTermino(fecha time.Time, horaInicio, horaFin time.Duration, ahora time.Time) bool {
	fin := horaDePared(fecha, horaFin)
	if CruzaMedianoche(horaInicio, horaFin) {
		fin = fin.AddDate(0, 0, 1)
	}
	return !fin.After(horaDePared(ahora, horaDelDia(ahora)))
}

// YaEmpezo es la mitad de arriba de lo mismo: la franja ya arrancó.
func YaEmpezo(fecha time.Time, horaInicio time.Duration, ahora time.Time) bool {
	return !horaDePared(fecha, horaInicio).After(horaDePared(ahora, horaDelDia(ahora)))
}

func horaDePared(fecha time.Time, hora time.Duration) time.Time {
	y, m, d := fecha.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Add(hora)
}

// InstanteDePared convierte una fecha de calendario más una hora de pared en
// el instante real, en la zona indicada.
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
// ═══════════════════════════════════════════════════════════════════════ Una
// escuela nocturna dicta de 22:00 a 01:00. Hasta acá eso era inexpresable: el
// sistema exigía hora_fin > hora_inicio, así que la única salida era partir
// la clase en dos reservas —una hasta las 23:59 y otra desde las 00:00 del
// día siguiente— que el sistema trataba como dos cosas sin relación.

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
// cumplir, sea una reserva normal o un bloqueo administrativo: rango horario
// coherente, duración acotada y que no esté en el pasado.
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
