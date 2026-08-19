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
	// EmailsDeAdminsSuscriptos es lo mismo para la copia por correo, pero
	// filtrado por quiénes pidieron recibir esa categoría (RF-05.13). No hay
	// una versión sin filtrar a propósito: mientras esta sea la única forma de
	// escribirle por mail a todos los Admin, no se puede agregar un aviso que
	// se saltee la preferencia sin darse cuenta.
	EmailsDeAdminsSuscriptos(ctx context.Context, categoria domain.CategoriaEmail) ([]string, error)
}

// PreferenciasEmail guarda qué categorías eligió cada Admin.
type PreferenciasEmail interface {
	// ElegidasDe devuelve SOLO lo que la persona decidió explícitamente. Una
	// categoría que no está en el mapa no está apagada: está sin elegir, y
	// entonces vale su valor por defecto (domain.Efectivas).
	ElegidasDe(ctx context.Context, usuarioID string) (map[domain.CategoriaEmail]bool, error)
	// Reemplazar deja registradas exactamente estas decisiones y borra lo que
	// hubiera antes. Recibe el mapa entero y no un alta/baja suelta: es lo que
	// hace que guardar el panel dos veces dé el mismo resultado que guardarlo
	// una.
	Reemplazar(ctx context.Context, usuarioID string, decisiones map[domain.CategoriaEmail]bool) error
	// RecibePorEmail resuelve la pregunta del envío: ¿la persona de esta
	// dirección quiere este correo? Va por email y no por ID porque es el dato
	// que tienen todos los avisos personales —alguno ni siquiera trae usuario,
	// como el reclamo de una devolución que puede estar a nombre de alguien sin
	// cuenta—. Una dirección sin cuenta no eligió nada: vale el default.
	RecibePorEmail(ctx context.Context, email string, categoria domain.CategoriaEmail) (bool, error)
}

// EnviadorDeEmail es el puerto hacia el correo.
type EnviadorDeEmail interface {
	Enviar(ctx context.Context, para, asunto, cuerpo string) error
}

type IDGenerator func() string
