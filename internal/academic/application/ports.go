package application

import (
	"context"

	"github.com/ramiro/sgrc/internal/academic/domain"
)

// Repo es el único contrato que este paquete necesita de infrastructure/
// (ver docs/06-arquitectura.md §3). Todo método "BuscarX" devuelve el
// Err_X_NoEncontrado correspondiente cuando no existe, nunca nil,nil.
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
	// TienePedidoAbierto evita que apretar dos veces el botón mande dos
	// avisos a todos los Admin por lo mismo. La base también lo impide (hay
	// un índice único parcial), pero fallar acá permite explicarlo en
	// castellano en vez de devolver un error de restricción.
	TienePedidoAbierto(ctx context.Context, usuarioID, materiaID string) (bool, error)

	// ListarMateriasReservables devuelve las materias en las que el
	// usuario puede reservar (RF-04.1): las de un ciclo sin archivar a las
	// que está asignado. Si soloDelDocente es nil devuelve todas las
	// reservables, que es lo que corresponde a un Admin.
	//
	// Existe como una sola consulta a propósito: sin esto, para saber "en
	// qué materias puedo reservar" habría que recorrer ciclos → cursos →
	// materias → docentes desde el cliente, una cascada de pedidos que
	// crece con el tamaño de la institución.
	ListarMateriasReservables(ctx context.Context, soloDelDocente *string) ([]MateriaReservable, error)

	// Archivar y clonar (RF-02.4/02.5) — operaciones multi-tabla, se
	// implementan como una sola transacción en infrastructure/.
	ArchivarCiclo(ctx context.Context, cicloID string) error
	ClonarCicloA(ctx context.Context, cicloOrigenID string, nuevoCiclo *domain.CicloLectivo) (cursosClonados int, materiasClonadas int, err error)
}

// ValidadorUsuario es el puerto hacia auth — una interfaz chica, nunca un
// import directo de internal/auth (ver docs/06-arquitectura.md §3). Solo
// necesitamos saber si un usuario existe y está en condiciones de ser
// asignado a una materia (RF-02.6: únicamente usuarios APROBADA).
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
//
// Hace falta para los avisos de un pedido de materia: quién lo pide (va en
// el aviso a los Admin) y quiénes ya dictan esa materia (les llega el aviso
// de que alguien más la pidió). Es un puerto y no un JOIN para que academic
// siga sin saber cómo está guardada una cuenta.
type DatosDeUsuario interface {
	Contacto(ctx context.Context, usuarioID string) (ContactoDeDocente, error)
	Contactos(ctx context.Context, usuarioIDs []string) ([]ContactoDeDocente, error)
}

// ValidadorReservas es el puerto hacia reservation — todavía no existe
// ese paquete, así que hasta que exista se usa una implementación stub
// que siempre devuelve false (ver infrastructure/stub_reservas.go). Esa
// respuesta es CORRECTA hoy — no hay ninguna reserva en el sistema
// todavía porque reservation no está implementado — no es un atajo falso,
// es la realidad actual. El día que reservation exista, se reemplaza la
// implementación sin tocar application/.
type ValidadorReservas interface {
	TieneReservasCurso(ctx context.Context, cursoID string) (bool, error)
	TieneReservasMateria(ctx context.Context, materiaID string) (bool, error)

	// TieneReservasDeCiclo existe solo para distinguir dos situaciones que
	// desde afuera se ven igual —un ciclo con archivado=true— pero que
	// merecen respuestas opuestas al pedir archivarlo de nuevo:
	//
	//   - archivado y sin reservas: la operación ya terminó. Pedirlo otra
	//     vez es un error del Admin y devuelve 409 (RF-02.4: no se archiva
	//     dos veces).
	//   - archivado y CON reservas: un intento anterior murió después de
	//     marcar el ciclo y antes de borrarlas. Acá el 409 era el problema,
	//     no la protección: dejaba la limpieza a medio hacer sin ninguna
	//     forma de completarla desde la API.
	TieneReservasDeCiclo(ctx context.Context, cicloID string) (bool, error)
}

// ArchivadorHistorico es el puerto hacia reporting+reservation para la
// cascada de archivado (RF-02.4/06.3) — a diferencia de ValidadorReservas
// (una lectura simple, SQL directo), esto es una ACCIÓN de dos pasos que
// deben ejecutarse en orden (primero calcular y guardar el snapshot
// histórico, después borrar físicamente las reservas — invertido, el
// snapshot quedaría vacío), y cruza DOS paquetes (reporting Y
// reservation). La implementación real vive en cmd/wiring_adapters.go,
// envolviendo reporting.Service y reservation.Service — nunca se
// reimplementa acá ni en infrastructure/ de este paquete. Ver el mismo
// criterio ya usado en auth/inventory hacia reservation.
type ArchivadorHistorico interface {
	// GuardarSnapshotDeCiclo calcula y persiste las estadísticas agregadas
	// del año (reporting). Es idempotente-por-reintento en el sentido que
	// importa: si falla, no se borró nada todavía.
	GuardarSnapshotDeCiclo(ctx context.Context, cicloID string, anio int) error

	// EliminarReservasDeCiclo borra FÍSICAMENTE las reservas del ciclo
	// (reservation). Es el único paso irreversible de toda la cascada, y
	// por eso ArchivarYClonar lo deja para el final — ver el comentario
	// del método sobre el orden.
	EliminarReservasDeCiclo(ctx context.Context, cicloID string) error
}

// CanceladorReservasDeMateria es el puerto hacia reservation para la
// cascada de RF-02.8 al quitar al último docente de una materia.
//
// Es el MISMO contrato que auth.CanceladorReservasDeMateria, y a propósito:
// quitar la asignación y dar de baja al docente son dos caminos al mismo
// estado —una materia sin nadie a cargo y con reservas futuras vivas— y solo
// uno de los dos lo manejaba. Se declara acá de nuevo en vez de importar el
// de auth porque cada paquete declara los puertos que necesita
// (docs/06-arquitectura.md §3); la implementación real es la misma y vive en
// cmd/wiring_adapters.go, envolviendo reservation.Service.
type CanceladorReservasDeMateria interface {
	CancelarReservasFuturasDeMateria(ctx context.Context, materiaID, motivo string) (canceladas int, err error)
}

// MateriaReservable es una materia lista para mostrar en un selector: trae
// el curso y el año del ciclo ya resueltos, porque "Matemáticas" a secas no
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
