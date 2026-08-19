package application

import (
	"context"
	"time"

	"github.com/ramiro/sgrc/internal/availability/domain"
)

// Repo es el único contrato que este paquete necesita de infrastructure/.
type Repo interface {
	ListarBloquesDeUsuario(ctx context.Context, usuarioID string) ([]*domain.BloqueHorario, error)

	// ListarBloquesDeUsuarios es la versión en lote para RF-07.2, indexada por
	// usuarioID. El listado de disponibilidad es la pantalla que ve cualquier
	// docente que necesita ubicar a un Admin, y resolverlo con
	// ListarBloquesDeUsuario en un for era una consulta por Admin (más otra por
	// su excepción).
	ListarBloquesDeUsuarios(ctx context.Context, usuarioIDs []string) (map[string][]*domain.BloqueHorario, error)
	CrearBloque(ctx context.Context, b *domain.BloqueHorario) error
	// BuscarBloqueDeUsuario y EliminarBloqueDeUsuario están acotados por (id,
	// usuarioID) — devuelven ErrBloqueNoEncontrado tanto si el ID no existe como
	// si existe pero es de otro usuario.
	BuscarBloqueDeUsuario(ctx context.Context, id, usuarioID string) (*domain.BloqueHorario, error)
	// GuardarBloque actualiza acotando por (b.ID, b.UsuarioID) — el
	// UsuarioID nunca cambia entre BuscarBloqueDeUsuario y este llamado.
	GuardarBloque(ctx context.Context, b *domain.BloqueHorario) error
	EliminarBloqueDeUsuario(ctx context.Context, id, usuarioID string) error

	// BuscarExcepcionDeFecha devuelve (nil, nil) si el usuario no cargó ninguna
	// excepción para esa fecha — no tenerla no es un error, es el caso normal
	// (RF-07.4 es opcional).
	BuscarExcepcionDeFecha(ctx context.Context, usuarioID string, fecha time.Time) (*domain.Excepcion, error)

	// BuscarExcepcionesDeFecha es la versión en lote de la anterior, misma
	// razón.
	BuscarExcepcionesDeFecha(ctx context.Context, usuarioIDs []string, fecha time.Time) (map[string]*domain.Excepcion, error)
	// GuardarExcepcion hace upsert por (usuario_id, fecha) — volver a postear
	// para la misma fecha reemplaza la excepción anterior (UNIQUE(usuario_id,
	// fecha), ver docs/08-api-spec.yaml).
	GuardarExcepcion(ctx context.Context, e *domain.Excepcion) error

	// ── Jornada de la institución ────────────────────────────────── Sin
	// usuarioID: la jornada no tiene dueño, describe a la escuela entera.

	// ListarJornada devuelve TODOS los bloques, de todos los días.
	ListarJornada(ctx context.Context) ([]*domain.BloqueJornada, error)
	CrearBloqueJornada(ctx context.Context, b *domain.BloqueJornada) error
	BuscarBloqueJornada(ctx context.Context, id string) (*domain.BloqueJornada, error)
	GuardarBloqueJornada(ctx context.Context, b *domain.BloqueJornada) error
	EliminarBloqueJornada(ctx context.Context, id string) error
}

// AdminInfo es lo mínimo que se necesita de cada Admin para RF-07.2 —
// nombre y apellido para el DTO, no el resto de la lógica de auth.
type AdminInfo struct {
	ID       string
	Nombre   string
	Apellido string
}

// ListadorAdmins es el puerto hacia auth — a diferencia del ListadorAdmins de
// notification (que solo necesita IDs para notificar), acá hace falta
// nombre/apellido de cada Admin aprobado para el listado de disponibilidad.
type ListadorAdmins interface {
	AdminsAprobados(ctx context.Context) ([]AdminInfo, error)
}

type IDGenerator func() string
