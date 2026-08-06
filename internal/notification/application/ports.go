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
	// ListarPorUsuario devuelve las notificaciones de un usuario,
	// filtradas por estado si filtroEstado no es nil.
	// Devuelve además el total de filas que matchean, no las de la página.
	ListarPorUsuario(ctx context.Context, usuarioID string, filtroEstado *domain.Estado, pagina paginacion.Pagina) ([]*domain.Notificacion, int, error)

	// ListarNoLeidasSobreUsuario: los avisos sin leer de un tipo que hablan
	// de una persona puntual, sin importar a quién le llegaron. Es lo que
	// permite cerrar el aviso de "cuenta pendiente" para todos los Admin
	// cuando uno cualquiera la resuelve.
	ListarNoLeidasSobreUsuario(ctx context.Context, sobreUsuarioID string, tipo domain.Tipo) ([]*domain.Notificacion, error)
	// MarcarTodasLeidasDe marca de una todas las NO_LEIDA de un usuario y
	// devuelve cuántas cambió. Es un UPDATE masivo a propósito: hacerlo
	// fila por fila desde application sería N queries para algo que la
	// base resuelve en una.
	MarcarTodasLeidasDe(ctx context.Context, usuarioID string, ahora time.Time) (int, error)
}

// ListadorAdmins es el puerto hacia auth — SQL directo contra usuario,
// sin importar internal/auth (mismo criterio que todos los demás
// validadores de solo lectura del proyecto).
type ListadorAdmins interface {
	IDsDeAdminsAprobados(ctx context.Context) ([]string, error)
	// EmailsDeAdminsAprobados es lo mismo pero para la copia por correo.
	// Son dos consultas y no una que devuelva las dos columnas porque los
	// dos caminos son independientes: el aviso interno tiene que llegar
	// aunque no haya correo configurado, y el correo se manda desde otra
	// goroutine, más tarde, sin que el aviso lo espere.
	EmailsDeAdminsAprobados(ctx context.Context) ([]string, error)
}

// EnviadorDeEmail es el puerto hacia el correo. La implementación real es
// internal/shared/email; este paquete no la importa, solo declara la forma
// que necesita (mismo criterio que HashFunc en auth).
//
// Nunca devuelve error hacia arriba en los suscriptores: el correo es una
// copia de cortesía del aviso que ya existe dentro del sistema. Si falla,
// se loguea y listo — la notificación interna sigue estando, así que no se
// pierde nada que la persona no pueda ver entrando.
type EnviadorDeEmail interface {
	Enviar(ctx context.Context, para, asunto, cuerpo string) error
}

type IDGenerator func() string
