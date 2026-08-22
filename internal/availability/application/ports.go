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

	// ReemplazarJornada deja la jornada exactamente igual a `bloques`, en una
	// transacción.
	//
	// Se reemplaza entera y no tramo por tramo porque la jornada es una sola
	// decisión de siete días: mientras se aplicaba de a partes, quedaba a la
	// vista una jornada a medias que PermiteReserva ya estaba usando para
	// aceptar o rechazar reservas.
	//
	// Una lista vacía es válida: deja a la escuela sin restricción de horario.
	ReemplazarJornada(ctx context.Context, bloques []*domain.BloqueJornada) error
}

// ── Lo que un cambio de jornada deja afuera ──────────────────────────────

// ReservaFutura es una reserva ya cargada, con lo justo para decidir si entra
// en una jornada y para poder nombrarla en la pantalla de confirmación.
type ReservaFutura struct {
	ID string
	// GrupoID es la clase a la que pertenece. Una clase con cinco máquinas son
	// CINCO ReservaFutura con el mismo GrupoID, porque el sistema guarda una
	// fila por equipo: sin esto, cancelar una clase de cinco equipos durante
	// quince semanas se le muestra al Admin como "75 reservas" cuando el
	// docente que las pierde cuenta quince clases.
	GrupoID    string
	Fecha      time.Time
	HoraInicio time.Duration
	HoraFin    time.Duration
	Equipo     string
	Materia    string
	Docente    string
}

// PrestamoAbierto es una máquina que está fuera del laboratorio ahora mismo.
//
// La jornada NO restringe los préstamos: una máquina se entrega cualquier día
// y a cualquier hora mientras esté en el laboratorio. Lo que importa acá es
// otra cosa —contra qué reserva salió— porque si esa reserva se cancela, hay
// alguien con equipos en la mano para una clase que dejó de existir.
type PrestamoAbierto struct {
	ID     string
	Equipo string
	Quien  string
	// ReservaID nil = préstamo espontáneo, sin reserva detrás. Un cambio de
	// jornada no lo toca ni de lejos.
	ReservaID *string
}

// ReservasDeLaInstitucion es el puerto hacia reservation.
//
// La regla de qué entra en la jornada NO viaja por acá: este puerto solo trae
// las reservas y los préstamos, y cancela los que se le indiquen. Quién queda
// afuera lo decide domain.PermiteReserva, que vive en este paquete y es la
// misma función que usa el alta de una reserva. Si la regla se duplicara del
// otro lado, el sistema podría cancelar algo que después aceptaría volver a
// cargar.
type ReservasDeLaInstitucion interface {
	// ReservasFuturas son las CONFIRMADA de esa fecha en adelante.
	ReservasFuturas(ctx context.Context, desde time.Time) ([]ReservaFutura, error)
	// PrestamosAbiertos son las máquinas que hoy están afuera.
	PrestamosAbiertos(ctx context.Context) ([]PrestamoAbierto, error)
	// CancelarReservas cancela esas reservas puntuales con ese motivo, que
	// llega tal cual al correo del docente.
	CancelarReservas(ctx context.Context, reservaIDs []string, motivo string) (int, error)
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
