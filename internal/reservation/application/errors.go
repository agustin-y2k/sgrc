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
	// confirmada que se superpone con el horario pedido. Se intenta
	// detectar esto ANTES de golpear la constraint de la base
	// (mejor mensaje), pero la constraint EXCLUDE sigue siendo la
	// garantía real ante condiciones de carrera.
	//
	// El pre-chequeo devuelve un *ErrorDeSolapamiento, que envuelve a este y
	// agrega qué equipo chocó contra qué. La constraint devuelve este pelado:
	// ahí el conflicto lo detectó la base y no tenemos el detalle a mano.
	ErrSolapamiento = errors.New("uno o más equipos ya tienen una reserva en ese horario")

	// ErrSinEquipos: una reserva necesita al menos una PC.
	ErrSinEquipos = errors.New("hay que seleccionar al menos un equipo")

	// ErrDemasiadosEquipos acota el tamaño del lote. No lo pide ninguna regla
	// de la escuela: lo pide que la operación es de lote y el pedido lo arma
	// el cliente. Sin tope, mandar diez mil identificadores hace que el
	// servidor arme diez mil filas en una transacción antes de que la base
	// pueda quejarse de nada.
	//
	// El límite es holgado a propósito: MaxEquiposPorOperacion es varias
	// veces el inventario de una escuela como esta, así que frena el pedido
	// absurdo sin poder molestar a ninguno legítimo.
	ErrDemasiadosEquipos = fmt.Errorf("no se pueden pedir más de %d equipos en una sola operación", MaxEquiposPorOperacion)

	// ErrDemasiadasOcurrencias: RF-04.2 materializa un ReservaGrupo por
	// cada fecha de la serie, todo en una transacción. Sin un tope, un
	// rango de fechas largo (fechaFin en 2099) genera miles de grupos y
	// reservas en un solo request: además de bloquear el proceso, deja al
	// docente con una serie que no puede administrar y una respuesta JSON
	// de megabytes.
	ErrDemasiadasOcurrencias = errors.New("el período pedido genera demasiadas clases; acotá la fecha de fin")

	// ErrSinOcurrencias: el rango es válido pero no contiene ni un solo
	// día de la semana pedido (ej. "los martes, del 4 al 6 de marzo").
	// Sin esto se creaba una regla de recurrencia que no materializaba
	// nada y quedaba huérfana en la base.
	ErrSinOcurrencias = errors.New("el período pedido no incluye ningún día de la semana elegido")

	// ErrMotivoObligatorio: RF-04.8 — cuando un Admin cancela la reserva
	// de otra persona tiene que decir por qué. El docente recibe ese texto
	// en la notificación (RF-05.1), así que un motivo vacío la dejaría sin
	// explicación. Cancelar una reserva propia no lo exige.
	ErrMotivoObligatorio = errors.New("el motivo es obligatorio para cancelar una reserva ajena")

	// ── Préstamos ───────────────────────────────────────────────────

	ErrPrestamoNoEncontrado = errors.New("no se encontró ese registro de entrega")

	// ErrEquipoYaPrestado: un índice único parcial impide dos préstamos
	// abiertos sobre el mismo equipo. Es la garantía que el papel no puede
	// dar — entregar dos veces la misma máquina porque dos Admin la anotaron
	// a la vez no lo detecta nadie hasta que aparece un docente sin
	// computadora.
	ErrEquipoYaPrestado = errors.New("ese equipo ya figura entregado y todavía no volvió")

	// ErrEquipoDadoDeBaja: no se entrega una máquina que salió del inventario.
	// Las EN_MANTENIMIENTO y FUERA_DE_SERVICIO sí se pueden entregar: llevar
	// una PC rota al técnico es justamente un préstamo.
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

	// ErrReservaSinDueño: un bloqueo administrativo no tiene docente detrás,
	// así que no hay a quién hacerle el pedido. Lo que corresponde ahí es
	// hablar con un Admin, que es quien lo puso.
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
	// usuario). Es un error del cliente, no una falla del servidor: sin este
	// sentinel caía al 500 genérico de mapearError, que era el modo de falla
	// más común de toda la API para cualquier ID válido-pero-inexistente.
	ErrReferenciaInexistente = errors.New("alguno de los datos referenciados no existe")
)

// maxConflictosEnElMensaje acota cuántos se nombran. Bloquear un carro
// entero contra una semana de clases puede dar decenas de choques, y un
// error de veinte renglones no lo lee nadie: los primeros alcanzan para
// entender qué pasó, y el resto se resume en un "y N más".
const maxConflictosEnElMensaje = 5

// ErrorDeSolapamiento dice QUÉ chocó, no solo que algo chocó.
//
// El pre-chequeo ya tiene esa información en la mano cuando decide rechazar
// el pedido —la trajo de la misma consulta con la que la detectó— así que
// tirarla obligaba al docente a adivinar cuál de los ocho equipos que tildó
// estaba ocupado.
//
// Envuelve a ErrSolapamiento para que el mapeo a HTTP siga funcionando con
// errors.Is: el código de estado no cambia, solo el texto.
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

// ErrFueraDeJornada: el día o el horario pedidos caen fuera de la jornada
// que declaró la institución.
//
// Reemplaza a ErrDiaNoLectivo, que decía "no se puede reservar un sábado o
// un domingo" para todo el mundo. La diferencia no es solo qué días abarca:
// aquel era una regla del código y este es la consecuencia de un dato que
// alguien cargó y puede cambiar.
var ErrFueraDeJornada = errors.New("ese día y horario están fuera de la jornada de la institución")
