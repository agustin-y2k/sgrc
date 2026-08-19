package application

import (
	"context"

	"github.com/ramiro/sgrc/internal/shared/paginacion"
	"github.com/ramiro/sgrc/internal/sugerencias/domain"
)

// Repo es todo lo que este paquete necesita de infrastructure/.
//
// BuscarPorID no está acotado por usuario, a diferencia de availability:
// acá quien resuelve es un Admin, que por definición responde mensajes de
// otros. La titularidad que sí importa —que un docente solo vea los suyos—
// la resuelve ListarDeUsuario, filtrando en la propia consulta.
type Repo interface {
	Crear(ctx context.Context, s *domain.Sugerencia) error
	BuscarPorID(ctx context.Context, id string) (*domain.Sugerencia, error)
	Guardar(ctx context.Context, s *domain.Sugerencia) error

	// ListarTodas es la pantalla del Admin. `soloAbiertas` existe porque lo
	// que casi siempre se quiere ver es lo que falta atender, no el archivo
	// completo.
	ListarTodas(ctx context.Context, soloAbiertas bool, p paginacion.Pagina) ([]*domain.Sugerencia, int, error)
	ListarDeUsuario(ctx context.Context, usuarioID string, p paginacion.Pagina) ([]*domain.Sugerencia, int, error)
	ContarAbiertas(ctx context.Context) (int, error)
}

// IDGenerator existe para no atar el dominio a una librería de UUID, igual
// que en el resto de los módulos.
type IDGenerator func() string
