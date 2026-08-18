package application

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrReservaGrupoNoEncontrado = errors.New("reserva no encontrada")
	ErrReservaNoEncontrada      = errors.New("reserva de equipo no encontrado")

	// ErrDocenteNoAsignado: RF-04.1 — solo un docente asignado a la
	// materia puede reservar para ella.
	ErrDocenteNoAsignado = errors.New("el docente no está asignado a esta materia")

	// ErrMateriaArchivada: RF-04.1 — una materia de un ciclo cerrado
	// conserva su registro pero no admite reservas nuevas.
	ErrMateriaArchivada = errors.New("la materia está archivada y no admite reservas nuevas")

	// ErrEquipoNoDisponible: la PC no existe, está dada de baja, o no está en
	// estado DISPONIBLE.
	ErrEquipoNoDisponible = errors.New("el equipo no está disponible para reservar")

	// ErrSolapamiento: alguno de los equipos pedidos ya tiene una reserva
	// confirmada que se superpone con el horario pedido.
	ErrSolapamiento = errors.New("uno o más equipos ya tienen una reserva en ese horario")

	// ErrSinEquipos: una reserva necesita al menos una PC.
	ErrSinEquipos = errors.New("hay que seleccionar al menos un equipo")

	// ErrDemasiadosEquipos acota el tamaño del lote.
	ErrDemasiadosEquipos = fmt.Errorf("no se pueden pedir más de %d equipos en una sola operación", MaxEquiposPorOperacion)

	// ErrDemasiadasOcurrencias: RF-04.2 materializa un ReservaGrupo por cada
	// fecha de la serie, todo en una transacción.
	ErrDemasiadasOcurrencias = errors.New("el período pedido genera demasiadas clases; acotá la fecha de fin")

	// ErrSinOcurrencias: el rango es válido pero no contiene ni un solo día de
	// la semana pedido (ej.
	ErrSinOcurrencias = errors.New("el período pedido no incluye ningún día de la semana elegido")

	// ErrMotivoObligatorio: RF-04.8 — cuando un Admin cancela la reserva de otra
	// persona tiene que decir por qué.
	ErrMotivoObligatorio = errors.New("el motivo es obligatorio para cancelar una reserva ajena")

	// ── Préstamos ───────────────────────────────────────────────────

	ErrPrestamoNoEncontrado = errors.New("no se encontró ese registro de entrega")

	// ErrEquipoYaPrestado: un índice único parcial impide dos préstamos abiertos
	// sobre el mismo equipo.
	ErrEquipoYaPrestado = errors.New("ese equipo ya figura entregado y todavía no volvió")

	// ErrEquipoDadoDeBaja: no se entrega una máquina que salió del inventario.
	ErrEquipoDadoDeBaja = errors.New("ese equipo está dado de baja del inventario")

	// ErrEquipoNoEncontrado: la dirección nombra un equipo que no está en el
	// inventario.
	//
	// Hace falta para el calendario, que hasta ahora no preguntaba: devolvía
	// 200 con la lista de bloques vacía, así que la pantalla dibujaba la
	// grilla completa y no había forma de distinguir "este equipo no existe"
	// de "este equipo está libre toda la semana". Es un 404 y no un 400: la
	// dirección está bien formada, lo que no hay es el recurso.
	ErrEquipoNoEncontrado = errors.New("ese equipo no está en el inventario")

	// ErrReservaAjena: RF-04.4 — un docente solo toca sus propias reservas.
	// El Admin puede tocar cualquiera.
	ErrReservaAjena = errors.New("esa reserva es de otra persona")

	// Los tres del pedido de liberación (RF-04.12).

	// ErrReservaPropia: no tiene sentido pedirse a uno mismo que libere.
	ErrReservaPropia = errors.New("esa reserva es tuya: no hay a quién pedirle")

	// ErrReservaSinDueño: un bloqueo administrativo no tiene docente detrás, así
	// que no hay a quién hacerle el pedido.
	ErrReservaSinDueño = errors.New("esa franja la tomó un administrador: no hay docente a quien pedirle")

	// ErrPedidoRepetido: ya se pidió por esa reserva hoy. El segundo correo
	// idéntico no agrega información, es presión.
	ErrPedidoRepetido = errors.New("ya le pediste esos equipos hoy")

	// ErrReservaYaEmpezada: pedirle a alguien que libere una franja que ya
	// está usando llega tarde — está dando la clase con esas máquinas.
	ErrReservaYaEmpezada = errors.New("esa reserva ya empezó")

	ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")

	// ErrReferenciaInexistente: SQLSTATE 23503 (foreign_key_violation) — el
	// request nombró un padre que no existe (un carro, un ciclo, una PC, un
	// usuario).
	ErrReferenciaInexistente = errors.New("alguno de los datos referenciados no existe")
)

// maxConflictosEnElMensaje acota cuántos se nombran.
const maxConflictosEnElMensaje = 5

// ErrorDeSolapamiento dice QUÉ chocó, no solo que algo chocó.
type ErrorDeSolapamiento struct {
	Conflictos []Solapamiento
}

func (e *ErrorDeSolapamiento) Unwrap() error { return ErrSolapamiento }

func (e *ErrorDeSolapamiento) Error() string {
	if len(e.Conflictos) == 0 {
		return ErrSolapamiento.Error()
	}

	partes := make([]string, 0, maxConflictosEnElMensaje)
	for _, c := range e.Conflictos {
		if len(partes) == maxConflictosEnElMensaje {
			break
		}
		partes = append(partes, c.describir())
	}
	if sobran := len(e.Conflictos) - len(partes); sobran > 0 {
		partes = append(partes, fmt.Sprintf("y %d más", sobran))
	}

	return "no se pudo reservar: " + strings.Join(partes, "; ")
}

// ErrFueraDeJornada: el día o el horario pedidos caen fuera de la jornada que
// declaró la institución.
var ErrFueraDeJornada = errors.New("ese día y horario están fuera de la jornada de la institución")
