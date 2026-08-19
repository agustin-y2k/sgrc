package application

import (
	"context"

	"github.com/ramiro/sgrc/internal/academic/domain"
)

// Repo es el único contrato que este paquete necesita de infrastructure/ (ver
// docs/06-arquitectura.md §3).
type Repo interface {
	// Ciclo lectivo
	CrearCiclo(ctx context.Context, c *domain.CicloLectivo) error
	BuscarCicloActivo(ctx context.Context) (*domain.CicloLectivo, error)
	BuscarCicloPorID(ctx context.Context, id string) (*domain.CicloLectivo, error)
	GuardarCiclo(ctx context.Context, c *domain.CicloLectivo) error
	ListarCiclos(ctx context.Context, filtroArchivado *bool) ([]*domain.CicloLectivo, error)

	// Curso
	CrearCurso(ctx context.Context, c *domain.Curso) error
	BuscarCursoPorID(ctx context.Context, id string) (*domain.Curso, error)
	GuardarCurso(ctx context.Context, c *domain.Curso) error
	EliminarCurso(ctx context.Context, id string) error
	ListarCursosPorCiclo(ctx context.Context, cicloID string) ([]*domain.Curso, error)

	// Materia
	CrearMateria(ctx context.Context, m *domain.Materia) error
	BuscarMateriaPorID(ctx context.Context, id string) (*domain.Materia, error)
	GuardarMateria(ctx context.Context, m *domain.Materia) error
	EliminarMateria(ctx context.Context, id string) error
	ListarMateriasPorCurso(ctx context.Context, cursoID string) ([]*domain.Materia, error)

	// DocenteMateria
	AsignarDocente(ctx context.Context, dm *domain.DocenteMateria) error
	BuscarDocenteMateria(ctx context.Context, id string) (*domain.DocenteMateria, error)
	GuardarDocenteMateria(ctx context.Context, dm *domain.DocenteMateria) error
	RemoverDocenteMateria(ctx context.Context, id string) error
	ListarDocentesDeMateria(ctx context.Context, materiaID string) ([]*domain.DocenteMateria, error)

	// Pedidos para dictar una materia
	CrearPedido(ctx context.Context, p *domain.PedidoDeMateria) error
	BuscarPedidoPorID(ctx context.Context, id string) (*domain.PedidoDeMateria, error)
	GuardarPedido(ctx context.Context, p *domain.PedidoDeMateria) error
	// ListarPedidos: `soloPendientes` es como se mira casi siempre — lo que
	// falta resolver, no el archivo.
	ListarPedidos(ctx context.Context, soloPendientes bool) ([]*domain.PedidoDeMateria, error)
	ListarPedidosDeUsuario(ctx context.Context, usuarioID string) ([]*domain.PedidoDeMateria, error)
	ContarPedidosPendientes(ctx context.Context) (int, error)
	// TienePedidoAbierto evita que apretar dos veces el botón mande dos avisos a
	// todos los Admin por lo mismo.
	TienePedidoAbierto(ctx context.Context, usuarioID, materiaID string) (bool, error)

	// ListarMateriasReservables devuelve las materias en las que el usuario
	// puede reservar (RF-04.1): las de un ciclo sin archivar a las que está
	// asignado.
	ListarMateriasReservables(ctx context.Context, soloDelDocente *string) ([]MateriaReservable, error)

	// Archivar y clonar (RF-02.4/02.5) — operaciones multi-tabla, se
	// implementan como una sola transacción en infrastructure/.
	ArchivarCiclo(ctx context.Context, cicloID string) error
	ClonarCicloA(ctx context.Context, cicloOrigenID string, nuevoCiclo *domain.CicloLectivo) (cursosClonados int, materiasClonadas int, err error)
}

// ValidadorUsuario es el puerto hacia auth — una interfaz chica, nunca un
// import directo de internal/auth (ver docs/06-arquitectura.md §3).
type ValidadorUsuario interface {
	ExisteYAprobado(ctx context.Context, usuarioID string) (bool, error)
}

// ContactoDeDocente es lo mínimo para avisarle a alguien: quién es y a dónde
// escribirle.
type ContactoDeDocente struct {
	UsuarioID string
	Nombre    string
	Email     string
}

// DatosDeUsuario resuelve nombres y correos, que viven en auth.
type DatosDeUsuario interface {
	Contacto(ctx context.Context, usuarioID string) (ContactoDeDocente, error)
	Contactos(ctx context.Context, usuarioIDs []string) ([]ContactoDeDocente, error)
}

// ValidadorReservas es el puerto hacia reservation — todavía no existe ese
// paquete, así que hasta que exista se usa una implementación stub que
// siempre devuelve false (ver infrastructure/stub_reservas.go).
type ValidadorReservas interface {
	TieneReservasCurso(ctx context.Context, cursoID string) (bool, error)
	TieneReservasMateria(ctx context.Context, materiaID string) (bool, error)

	// TieneReservasDeCiclo existe solo para distinguir dos situaciones que desde
	// afuera se ven igual —un ciclo con archivado=true— pero que merecen
	// respuestas opuestas al pedir archivarlo de nuevo: - archivado y sin
	// reservas: la operación ya terminó.
	TieneReservasDeCiclo(ctx context.Context, cicloID string) (bool, error)
}

// ArchivadorHistorico es el puerto hacia reporting+reservation para la
// cascada de archivado (RF-02.4/06.3) — a diferencia de ValidadorReservas
// (una lectura simple, SQL directo), esto es una ACCIÓN de dos pasos que
// deben ejecutarse en orden (primero calcular y guardar el snapshot
// histórico, después borrar físicamente las reservas — invertido, el snapshot
// quedaría vacío), y cruza DOS paquetes (reporting Y reservation).
type ArchivadorHistorico interface {
	// GuardarSnapshotDeCiclo calcula y persiste las estadísticas agregadas del
	// año (reporting).
	GuardarSnapshotDeCiclo(ctx context.Context, cicloID string, anio int) error

	// EliminarReservasDeCiclo borra FÍSICAMENTE las reservas del ciclo
	// (reservation).
	EliminarReservasDeCiclo(ctx context.Context, cicloID string) error
}

// CanceladorReservasDeMateria es el puerto hacia reservation para la cascada
// de RF-02.8 al quitar al último docente de una materia.
type CanceladorReservasDeMateria interface {
	CancelarReservasFuturasDeMateria(ctx context.Context, materiaID, motivo string) (canceladas int, err error)
}

// MateriaReservable es una materia lista para mostrar en un selector: trae el
// curso y el año del ciclo ya resueltos, porque "Matemáticas" a secas no
// alcanza para distinguir la de 1°A de la de 3°B.
type MateriaReservable struct {
	MateriaID     string
	MateriaNombre string
	CursoID       string
	CursoNombre   string
	CicloID       string
	CicloAnio     int
}

// IDGenerator genera un ID nuevo — inyectado, mismo patrón que auth.
type IDGenerator func() string
