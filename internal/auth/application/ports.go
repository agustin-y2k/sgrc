package application

import (
	"context"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// Repo es el único contrato que este paquete necesita de infrastructure/ —
// nunca importar pgx directamente desde acá (ver docs/06-arquitectura.md §3).
//
// BuscarPorEmail y BuscarPorID devuelven ErrUsuarioNoEncontrado (no nil,nil)
// cuando no existe, para no obligar al caller a chequear dos cosas (error Y
// puntero nil) en cada llamada.
type Repo interface {
	// EnTransaccion corre fn de forma atómica. Lo usa transicionar() para
	// que el guard del último Admin (RF-01.8) cuente y escriba sin que otro
	// pedido se cuele en el medio.
	EnTransaccion(ctx context.Context, fn func(Repo) error) error

	BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error)
	BuscarPorID(ctx context.Context, id string) (*domain.Usuario, error)
	// Listar devuelve una página de usuarios filtrados por estado/rol (nil =
	// sin ese filtro) y el total que matchean. Usado por GET
	// /api/auth/usuarios (solo Admin).
	Listar(ctx context.Context, filtroEstado *domain.Estado, filtroRol *domain.Rol, pagina paginacion.Pagina) ([]*domain.Usuario, int, error)
	Crear(ctx context.Context, u *domain.Usuario) error
	Guardar(ctx context.Context, u *domain.Usuario) error
	ContarAdminsAprobados(ctx context.Context) (int, error)
	Eliminar(ctx context.Context, id string) error
}

// Estas funciones se inyectan (no se llaman directamente a paquetes
// externos) para que Service sea testeable con fakes, sin argon2 ni JWT
// reales — mismo patrón que internal/shared/adminseed.
type (
	HashFunc            func(password string) (string, error)
	VerifyFunc          func(password, hash string) (bool, error)
	TokenSigner         func(u *domain.Usuario) (string, error)
	IDGenerator         func() string
	GenerarTemporalFunc func() (string, error)
)

// GestorMateriasDocente es el puerto hacia academic — necesario para la
// cascada de RF-02.8 (dar de baja al docente). Nunca se importa
// internal/academic directamente; la implementación real consulta
// docente_materia por SQL, igual que ValidadorUsuarioPostgres de academic
// consulta usuario.
type GestorMateriasDocente interface {
	// MateriasDeDocente devuelve los IDs de materia a los que el usuario
	// está asignado.
	MateriasDeDocente(ctx context.Context, usuarioID string) ([]string, error)
	// QuedaOtroDocenteActivo indica si, excluyendo a usuarioIDExcluido,
	// sigue habiendo al menos otro docente APROBADA asignado a esa
	// materia.
	QuedaOtroDocenteActivo(ctx context.Context, materiaID, usuarioIDExcluido string) (bool, error)
	// RemoverAsignacionesDeDocente elimina todas las filas docente_materia
	// de ese usuario (RF-02.8: se hace después de identificar las
	// materias huérfanas, nunca antes).
	RemoverAsignacionesDeDocente(ctx context.Context, usuarioID string) error
}

// CanceladorReservasDeMateria es el puerto hacia reservation — a
// diferencia de GestorMateriasDocente (una lectura/escritura simple sin
// máquina de estados, que sí va directo por SQL), esto es una ACCIÓN con
// reglas de negocio reales (cancelar una reserva, recalcular el estado de
// su ReservaGrupo padre), así que la implementación real vive en
// cmd/main.go envolviendo reservation.Service — nunca se reimplementa acá
// ni en infrastructure/ de este paquete. Ver el comentario en
// cmd/wiring_adapters.go.
type CanceladorReservasDeMateria interface {
	CancelarReservasFuturasDeMateria(ctx context.Context, materiaID, motivo string) (canceladas int, err error)
}
