// Package http expone las rutas Fiber de notification — ver
// docs/08-api-spec.yaml para el contrato completo de cada endpoint.
package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

type notificacionResponse struct {
	ID        string  `json:"id"`
	ReservaID *string `json:"reservaId,omitempty"`
	Mensaje   string  `json:"mensaje"`
	// Tipo le permite a la interfaz ofrecer la acción que corresponde
	// (ej. "ir a aprobar") sin interpretar el texto del mensaje.
	Tipo     string     `json:"tipo"`
	Estado   string     `json:"estado"`
	CreadaEn time.Time  `json:"creadaEn"`
	LeidaEn  *time.Time `json:"leidaEn,omitempty"`
}

func toNotificacionResponse(n *domain.Notificacion) notificacionResponse {
	return notificacionResponse{
		ID: n.ID, ReservaID: n.ReservaID, Mensaje: n.Mensaje, Tipo: string(n.Tipo),
		Estado: string(n.Estado), CreadaEn: n.CreadaEn, LeidaEn: n.LeidaEn,
	}
}

// listarNotificacionesResponse reemplaza al fiber.Map suelto que devolvía
// este endpoint: con `meta` en juego conviene un tipo, que es además lo que
// los tests pueden deserializar sin adivinar la forma.
type listarNotificacionesResponse struct {
	Data []notificacionResponse `json:"data"`
	Meta paginacion.Meta        `json:"meta"`
}
