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
	// EliminarCiclo borra la fila. Sólo se llama con el ciclo vacío: la clave
	// foránea de `curso` es NO ACTION, así que con cursos adentro la base lo
	// rechaza con un error crudo.
	EliminarCiclo(ctx context.Context, id string) error
	ListarCiclos(ctx context.Context, filtroArchivado *bool) ([]*domain.CicloLectivo, error)

	// Curso
	CrearCurso(ctx context.Context, c *domain.Curso) error
	BuscarCursoPorID(ctx context.Context, id string) (*domain.Curso, error)
	// CiclosDeCursos devuelve, para cada id pedido, a qué ciclo pertenece —en
	// UNA consulta, no una por curso—. Un id que no existe simplemente no
	// aparece en el mapa, que es como quien llama se entera.
	//
	// Existe para validar los destinos de una copia de materias (RF-02.12): con
	// treinta destinos, preguntar de a uno son treinta viajes a la base para
	// contestar algo que un solo IN contesta entero.
	CiclosDeCursos(ctx context.Context, ids []string) (map[string]string, error)
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
	// ListarAsignaciones son TODAS las asignaciones docente-materia de un
	// ciclo, con los nombres ya resueltos. Es la misma información que
	// ListarDocentesDeMateria materia por materia, en una sola consulta:
	// las dos pantallas que la muestran —las materias de un curso y el
	// listado de usuarios— la necesitan entera, y pedirla de a una es un
	// N+1 sobre una tabla que se lee en cada carga.
	ListarAsignaciones(ctx context.Context, cicloID string) ([]AsignacionDocente, error)

	// Pedidos para dictar una materia
	CrearPedido(ctx context.Context, p *domain.PedidoDeMateria) error
	BuscarPedidoPorID(ctx context.Context, id string) (*domain.PedidoDeMateria, error)
	GuardarPedido(ctx context.Context, p *domain.PedidoDeMateria) error
	// ListarPedidos: `soloPendientes` es como se mira casi siempre — lo que
	// falta resolver, no el archivo.
	ListarPedidos(ctx context.Context, soloPendientes bool) ([]*PedidoDetallado, error)
	ListarPedidosDeUsuario(ctx context.Context, usuarioID string) ([]*PedidoDetallado, error)
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

	// Carga y descarga de la estructura entera (RF-02.12) — las otras tres
	// operaciones multi-tabla del paquete, por el mismo motivo que las dos de
	// arriba: se resuelven en una transacción, no fila por fila desde el
	// servicio.
	ListarEstructuraDeCiclo(ctx context.Context, cicloID string) ([]CursoConMaterias, error)
	ImportarEstructura(ctx context.Context, cicloID string, cursos []CursoConMaterias) (ResultadoImportacion, error)
	CopiarMateriasA(ctx context.Context, cursoOrigenID string, cursosDestinoIDs []string) (ResultadoCopia, error)
}

// ValidadorUsuario es el puerto hacia auth — una interfaz chica, nunca un
// import directo de internal/auth (ver docs/06-arquitectura.md §3).
type ValidadorUsuario interface {
	ExisteYAprobado(ctx context.Context, usuarioID string) (bool, error)
	// AlgunoAprobado contesta la misma pregunta para un conjunto, en UNA
	// consulta: «¿queda alguno de éstos con la cuenta viva?».
	//
	// La usa la cascada de RF-02.8, que corre al quitarle una materia a un
	// docente y decide si hay que cancelar las reservas futuras de esa materia.
	// Preguntar de a uno era un viaje por docente para una decisión que es un
	// EXISTS — y esa cascada corre en el camino de una baja, con alguien
	// esperando la respuesta.
	AlgunoAprobado(ctx context.Context, usuarioIDs []string) (bool, error)
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

// MarcasDeInventario es el puerto hacia inventory — una interfaz chica, nunca
// un import directo (ver docs/06-arquitectura.md §3).
//
// Existe por una consecuencia de RF-03.21: las marcas de preferencia de equipo
// se vinculan a la materia POR NOMBRE y no por referencia, deliberadamente,
// para que sobrevivan al clonado anual del ciclo. El precio es que renombrar o
// borrar una materia las deja apuntando a un nombre que ya no está, y en
// silencio.
//
// Este puerto sirve para AVISAR, no para arreglar. Arrastrar la marca al nombre
// nuevo sería peor: una marca sin alcance aplica a TODAS las materias que se
// llamen igual, así que renombrar una de veintiséis «Matemática» y llevarse la
// marca dejaría a las otras veinticinco sin ella.
type MarcasDeInventario interface {
	// CuantasDejarianDeAplicar: cuántas marcas apuntan a ese nombre de materia.
	CuantasDejarianDeAplicar(ctx context.Context, materiaNombre string) (int, error)
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

	// HayBloqueosEnElAnio mira un AÑO, no un ciclo, y ésa es toda la razón por
	// la que existe aparte del método de arriba.
	//
	// Un bloqueo administrativo no tiene materia ni clave foránea al ciclo: se
	// le atribuye a uno por el año de su fecha (ver EliminarReservasDeCiclo en
	// reservation). Eso significa que corregir el año de un ciclo puede hacerle
	// adoptar bloqueos que nadie le asignó, si el año de destino ya tenía
	// alguno — y crear un bloqueo en un año sin ciclo está permitido, así que el
	// caso no es hipotético.
	HayBloqueosEnElAnio(ctx context.Context, anio int) (bool, error)
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
	// CursoModalidad acompaña al nombre porque el nombre solo dejó de ser
	// único: dos carreras pueden tener cada una su "1°A" (RF-02.2). Vacía
	// cuando el curso no pertenece a ninguna agrupación.
	CursoModalidad string
	CicloID        string
	CicloAnio      int
}

// AsignacionDocente es una fila de docente_materia con los nombres de las dos
// puntas ya resueltos: quién dicta y qué materia de qué curso. Mismo criterio
// que MateriaReservable.
//
// Existe porque la relación se mira desde los dos lados y en las dos pantallas
// se leía a medias: las materias de un curso mostraban sus docentes sólo al
// desplegar una por una, y el listado de usuarios no decía qué dictaba cada
// quien.
type AsignacionDocente struct {
	ID        string
	UsuarioID string
	// DocenteNombre es "Nombre Apellido", tal como se muestra.
	DocenteNombre  string
	Rol            string
	MateriaID      string
	MateriaNombre  string
	CursoID        string
	CursoNombre    string
	CursoModalidad string
}

// PedidoDetallado es un PedidoDeMateria con el nombre de lo que se pidió ya
// resuelto por JOIN — mismo criterio que MateriaReservable acá arriba y que
// PrestamoDetallado en reservation.
//
// Existe porque sin esto no había forma de decir QUÉ materia se pedía cuando
// la materia ya existe: el pedido guarda `materia_id` y nada más, así que las
// dos pantallas que lo muestran —la bandeja del Admin y el perfil de quien
// pidió— caían a un texto de relleno ("Una materia existente"), y el Admin
// terminaba aprobando sin saber qué. Con una sola materia cargada no se nota;
// con varias, dos pedidos distintos se ven idénticos.
type PedidoDetallado struct {
	Pedido *domain.PedidoDeMateria
	// Los dos vacíos cuando la materia todavía no existe: ahí lo que se pidió
	// está en CursoSolicitado y MateriaSolicitada, escrito a mano.
	MateriaNombre string
	CursoNombre   string
	// DocenteNombre es quién lo pidió, "Nombre Apellido". El pedido guarda el
	// UUID, y aprobar es asignar a ESA persona a la materia: sin el nombre, la
	// bandeja del Admin muestra un pedido que no se sabe de quién es.
	DocenteNombre string
}

// CursoConMaterias es un curso con los nombres de sus materias: la estructura
// de un ciclo entero leída o escrita de una sola vez.
//
// NO lleva identificadores, y eso es lo que la hace útil. Lo que nombra a un
// curso acá es la terna año + división + modalidad —la misma del índice único
// de la base— y a una materia su nombre dentro del curso. Con IDs adentro, el
// archivo que se descarga de un ciclo no se podría volver a cargar sobre otro,
// que es justamente para lo que se descarga.
type CursoConMaterias struct {
	Anio      int
	Division  string
	Modalidad string
	Materias  []string
}

// ResultadoImportacion separa lo creado de lo que ya estaba. Son dos noticias
// distintas para quien acaba de subir un archivo: una importación que no creó
// nada no falló —el ciclo ya tenía todo— y decirlo con todas las letras evita
// el segundo intento.
type ResultadoImportacion struct {
	CursosCreados      int
	CursosExistentes   int
	MateriasCreadas    int
	MateriasExistentes int
}

// ResultadoCopia es lo mismo para la copia de materias entre cursos: cuántas
// se crearon y cuántas el destino ya tenía.
type ResultadoCopia struct {
	MateriasCreadas    int
	MateriasExistentes int
	CursosDestino      int
}

// IDGenerator genera un ID nuevo — inyectado, mismo patrón que auth.
type IDGenerator func() string
