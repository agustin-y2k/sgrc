// Package application orquesta la lectura del registro de auditoría.
package application

import (
	"context"
	"errors"

	"github.com/ramiro/sgrc/internal/auditoria/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// ErrIDInvalido: el ID recibido no tiene formato UUID válido.
var ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")

// Repo es el único contrato que este paquete necesita de infrastructure/.
//
// No tiene ni un método de escritura, y eso es deliberado: el registro de
// auditoría se escribe desde internal/shared/audit y desde ningún otro lado.
// Un puerto de lectura que no sepa escribir es la forma más barata de garantizar
// que esta parte del sistema no pueda reescribir el registro ni por accidente.
type Repo interface {
	Listar(ctx context.Context, f domain.Filtro, p paginacion.Pagina) ([]domain.Entrada, int, error)
	// AccionesEnUso son las acciones que de verdad aparecen en el registro, para
	// que el filtro de la pantalla ofrezca esas y no las treinta del catálogo —
	// la mitad de las cuales nunca ocurrió en esta instalación.
	AccionesEnUso(ctx context.Context) ([]string, error)
	// EntidadesEnUso, lo mismo para el otro eje.
	EntidadesEnUso(ctx context.Context) ([]string, error)
}
