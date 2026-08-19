package application

import (
	"context"
	"time"

	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// Repo es el único contrato que este paquete necesita de infrastructure/.
type Repo interface {
	Crear(ctx context.Context, n *domain.Notificacion) error
	BuscarPorID(ctx context.Context, id string) (*domain.Notificacion, error)
	Guardar(ctx context.Context, n *domain.Notificacion) error
	// ListarPorUsuario devuelve las notificaciones de un usuario, filtradas por
	// estado si filtroEstado no es nil.
	ListarPorUsuario(ctx context.Context, usuarioID string, filtroEstado *domain.Estado, pagina paginacion.Pagina) ([]*domain.Notificacion, int, error)

	// ListarNoLeidasSobreUsuario: los avisos sin leer de un tipo que hablan de
	// una persona puntual, sin importar a quién le llegaron.
	ListarNoLeidasSobreUsuario(ctx context.Context, sobreUsuarioID string, tipo domain.Tipo) ([]*domain.Notificacion, error)
	// MarcarTodasLeidasDe marca de una todas las NO_LEIDA de un usuario y
	// devuelve cuántas cambió.
	MarcarTodasLeidasDe(ctx context.Context, usuarioID string, ahora time.Time) (int, error)
}

// ListadorAdmins es el puerto hacia auth — SQL directo contra usuario, sin
// importar internal/auth (mismo criterio que todos los demás validadores de
// solo lectura del proyecto).
type ListadorAdmins interface {
	IDsDeAdminsAprobados(ctx context.Context) ([]string, error)
	// EmailsDeAdminsAprobados es lo mismo pero para la copia por correo.
	EmailsDeAdminsAprobados(ctx context.Context) ([]string, error)
}

// EnviadorDeEmail es el puerto hacia el correo.
type EnviadorDeEmail interface {
	Enviar(ctx context.Context, para, asunto, cuerpo string) error
}

type IDGenerator func() string
