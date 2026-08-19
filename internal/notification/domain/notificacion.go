// Package domain contiene la entidad Notificacion — puramente interna al
// sistema (RF-05), sin dependencias de infraestructura.
package domain

import (
	"errors"
	"fmt"
	"time"
)

type Estado string

const (
	NoLeida Estado = "NO_LEIDA"
	Leida   Estado = "LEIDA"
)

var ErrEstadoInvalido = errors.New("estado de notificación inválido")

func ParseEstado(s string) (Estado, error) {
	switch Estado(s) {
	case NoLeida, Leida:
		return Estado(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrEstadoInvalido, s)
	}
}

var ErrMensajeVacio = errors.New("el mensaje de la notificación no puede estar vacío")

// Tipo dice de qué se trata el aviso.
type Tipo string

const (
	// TipoGeneral: un aviso que se lee y nada más.
	TipoGeneral Tipo = "GENERAL"
	// TipoDocentePendiente: alguien se registró y espera aprobación
	// (RF-05.6). La pantalla del Admin lo enlaza con la de aprobación.
	TipoDocentePendiente Tipo = "DOCENTE_PENDIENTE"
	// TipoReservaCancelada: RF-05.1/05.2/05.3.
	TipoReservaCancelada Tipo = "RESERVA_CANCELADA"
	// TipoLicenciaPorVencer: hay licencias de software que hay que renovar
	// (RF-05.9).
	TipoLicenciaPorVencer Tipo = "LICENCIA_POR_VENCER"
	// TipoReservaPorComenzar: el recordatorio de que en un rato hay clase, y
	// también el aviso de que una de esas máquinas no volvió.
	TipoReservaPorComenzar Tipo = "RESERVA_POR_COMENZAR"
	// TipoReservaNoRetirada: pasaron los minutos de gracia y esas máquinas
	// dejaron de estar reservadas (RF-08.10).
	TipoReservaNoRetirada Tipo = "RESERVA_NO_RETIRADA"
	// TipoPedidoDeLiberacion: otro docente necesita un equipo que esta persona
	// tiene reservado (RF-04.12).
	TipoPedidoDeLiberacion Tipo = "PEDIDO_DE_LIBERACION"
	// TipoEquipoSinDevolver: para los Admin — una máquina no volvió a horario.
	// Lleva a la pantalla de entregas, no a la de reservas.
	TipoEquipoSinDevolver Tipo = "EQUIPO_SIN_DEVOLVER"

	// TipoPedidoDeMateria: alguien pidió dictar una materia.
	TipoPedidoDeMateria Tipo = "PEDIDO_DE_MATERIA"
	// TipoPedidoDeMateriaResuelto: el Admin contestó ese pedido. Le llega a
	// quien lo hizo, aprobado o no.
	TipoPedidoDeMateriaResuelto Tipo = "PEDIDO_DE_MATERIA_RESUELTO"
	// TipoSugerencia: alguien escribió en el buzón. Para los Admin.
	TipoSugerencia Tipo = "SUGERENCIA"
	// TipoSugerenciaRespondida: le contestaron. Para quien escribió.
	TipoSugerenciaRespondida Tipo = "SUGERENCIA_RESPONDIDA"
)

var ErrTipoInvalido = errors.New("tipo de notificación inválido")

func ParseTipo(s string) (Tipo, error) {
	switch Tipo(s) {
	case TipoGeneral, TipoDocentePendiente, TipoReservaCancelada, TipoLicenciaPorVencer,
		TipoReservaPorComenzar, TipoReservaNoRetirada, TipoEquipoSinDevolver,
		TipoPedidoDeLiberacion, TipoPedidoDeMateria, TipoPedidoDeMateriaResuelto,
		TipoSugerencia, TipoSugerenciaRespondida:
		return Tipo(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrTipoInvalido, s)
	}
}

// Notificacion es un aviso interno para un usuario puntual — puede estar
// vinculada a una Reserva concreta (ReservaID) o ser un aviso genérico sin
// esa referencia (ej.
type Notificacion struct {
	ID        string
	UsuarioID string
	ReservaID *string
	// SobreUsuarioID: de quién habla el aviso, cuando habla de alguien.
	SobreUsuarioID *string
	Mensaje        string
	Tipo           Tipo
	Estado         Estado
	CreadaEn       time.Time
	LeidaEn        *time.Time
}

// Referencias es a qué apunta un aviso. Los dos campos son opcionales y se
// excluyen en la práctica: un aviso habla de una reserva o de una cuenta.
type Referencias struct {
	ReservaID      *string
	SobreUsuarioID *string
}

func NuevaNotificacion(id, usuarioID, mensaje string, tipo Tipo, ref Referencias, ahora time.Time) (*Notificacion, error) {
	if mensaje == "" {
		return nil, ErrMensajeVacio
	}
	if tipo == "" {
		tipo = TipoGeneral
	}
	return &Notificacion{
		ID:             id,
		UsuarioID:      usuarioID,
		ReservaID:      ref.ReservaID,
		SobreUsuarioID: ref.SobreUsuarioID,
		Mensaje:        mensaje,
		Tipo:           tipo,
		Estado:         NoLeida,
		CreadaEn:       ahora,
	}, nil
}

var ErrYaLeida = errors.New("la notificación ya está marcada como leída")

// MarcarLeida es idempotente a propósito de NO serlo silenciosamente:
// devuelve ErrYaLeida si se llama dos veces, para que la capa de arriba
// decida si eso es un error real o algo que puede ignorar (ej.
func (n *Notificacion) MarcarLeida(ahora time.Time) error {
	if n.Estado == Leida {
		return ErrYaLeida
	}
	n.Estado = Leida
	n.LeidaEn = &ahora
	return nil
}
