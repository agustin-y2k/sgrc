// Package http expone la consulta del registro de auditoría bajo /api/auditoria
// — ver docs/08-api-spec.yaml para el contrato completo.
package http

import (
	"encoding/json"
	"time"

	"github.com/ramiro/sgrc/internal/auditoria/domain"
)

// entradaResponse es una fila del registro tal como se muestra.
type entradaResponse struct {
	ID string `json:"id"`
	// ActorID viaja SIEMPRE, también cuando hay nombre: es lo único que sigue
	// identificando a quien hizo la acción si esa cuenta después se elimina, y
	// es con lo que se filtra «todo lo que hizo esta persona».
	ActorID string `json:"actorId"`
	// ActorNombre falta cuando la cuenta ya no existe (RF-01.9). No se rellena
	// con un texto de relleno: la pantalla decide cómo decir «una cuenta
	// eliminada», y un "Desconocido" escrito acá se confundiría con el nombre
	// de alguien.
	ActorNombre string          `json:"actorNombre,omitempty"`
	Accion      string          `json:"accion"`
	Entidad     string          `json:"entidad"`
	EntidadID   string          `json:"entidadId,omitempty"`
	Detalle     json.RawMessage `json:"detalle,omitempty"`
	// IPOrigen sólo lo ve un Admin, que es el único que llega a este endpoint.
	IPOrigen string    `json:"ipOrigen,omitempty"`
	CreadoEn time.Time `json:"creadoEn"`
}

func toEntradaResponse(e domain.Entrada) entradaResponse {
	return entradaResponse{
		ID: e.ID, ActorID: e.UsuarioID, ActorNombre: e.ActorNombre,
		Accion: e.Accion, Entidad: e.Entidad, EntidadID: e.EntidadID,
		// El detalle pasa como JSON crudo, sin volver a serializarlo: es lo que
		// guardó cada acción y este paquete no lo interpreta.
		Detalle:  json.RawMessage(e.Detalle),
		IPOrigen: e.IPOrigen,
		CreadoEn: e.CreadoEn,
	}
}

// opcionesResponse son los valores que HOY existen en el registro, para armar
// los selectores de la pantalla.
type opcionesResponse struct {
	Acciones  []string `json:"acciones"`
	Entidades []string `json:"entidades"`
}
