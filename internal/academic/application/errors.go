package application

import "errors"

// Errores de negocio de academic. Todos exportados para que
// interfaces/http los mapee a códigos HTTP específicos sin parsear texto.
var (
	ErrCicloNoEncontrado = errors.New("ciclo lectivo no encontrado")
	ErrYaHayCicloActivo  = errors.New("ya existe un ciclo lectivo activo — archivalo antes de crear uno nuevo")
	ErrCicloYaTieneAnio  = errors.New("ya existe un ciclo lectivo para ese año")
	// ErrCicloConReservas: corregir el año de un ciclo que ya tiene reservas lo
	// dejaría diciendo que las clases de un año pasaron en otro — las reservas
	// llevan su propia fecha y no se mueven con él.
	ErrCicloConReservas = errors.New("el ciclo lectivo ya tiene reservas: corregirle el año dejaría esas clases en el año anterior")
	// ErrAnioConBloqueos: el año de destino ya tiene bloqueos administrativos, y
	// un bloqueo pertenece a un ciclo por el año de su fecha. Mudarse ahí sería
	// adoptarlos sin que nadie lo haya pedido.
	ErrAnioConBloqueos = errors.New("ese año ya tiene bloqueos administrativos cargados, que pasarían a pertenecer a este ciclo")
	// ErrCicloConCursos: eliminar existe para deshacer un ciclo recién creado
	// con el año equivocado. Uno que ya tiene cursos cargados se archiva, no se
	// borra: sus cursos y materias son el registro de cómo se organizó ese año.
	ErrCicloConCursos = errors.New("el ciclo lectivo tiene cursos cargados: archivalo en vez de eliminarlo")

	ErrCursoNoEncontrado    = errors.New("curso no encontrado")
	ErrCursoNombreDuplicado = errors.New("ya existe otro curso con ese nombre en el mismo ciclo lectivo")
	ErrCursoConReservas     = errors.New("el curso tiene materias con reservas asociadas — no se puede eliminar")

	ErrMateriaNoEncontrada = errors.New("materia no encontrada")
	// El "sin distinguir" no es un detalle: sin decirlo, quien acaba de
	// escribir «Matematica» en un curso que ya tiene «Matemática» lee que ya
	// existe "ese nombre" mirando dos textos que no son iguales, y el mensaje
	// parece un error del sistema.
	ErrMateriaNombreDuplicado = errors.New("ya existe otra materia con ese nombre en el mismo curso, sin distinguir tildes, mayúsculas ni espacios de más")
	ErrMateriaConReservas     = errors.New("la materia tiene reservas asociadas — no se puede eliminar")

	ErrDocenteMateriaNoEncontrado = errors.New("asignación docente-materia no encontrada")
	ErrUsuarioNoValidoParaAsignar = errors.New("el usuario no existe o no está en estado APROBADA")

	// ErrCicloArchivado cubre la carga masiva y la copia entre cursos con la
	// misma regla que ya tenía la carga de a uno (RF-02.11): un ciclo cerrado
	// se conserva como referencia y no se edita.
	ErrCicloArchivado = errors.New("el ciclo lectivo está archivado — sus cursos y materias ya no se pueden modificar")

	// ErrSinCursosParaImportar: el archivo se leyó y no trajo ninguna fila
	// aprovechable. Es distinto de un archivo ilegible, y se dice distinto.
	ErrSinCursosParaImportar = errors.New("el archivo no trae ningún curso para cargar")
	// ErrImportacionDemasiadoGrande: tope de sanidad. Una planilla de cursos de
	// una institución entra holgada; lo que no entra es un archivo equivocado.
	ErrImportacionDemasiadoGrande = errors.New("el archivo trae demasiados cursos para una sola carga")

	ErrSinCursosDestino   = errors.New("hay que elegir al menos un curso al que copiar las materias")
	ErrCopiaAlMismoCurso  = errors.New("no se puede copiar las materias de un curso a sí mismo")
	ErrCopiaEntreCiclos   = errors.New("los cursos de destino tienen que ser del mismo ciclo lectivo que el de origen")
	ErrCursoOrigenSinMat  = errors.New("el curso de origen no tiene ninguna materia para copiar")
	ErrDemasiadosDestinos = errors.New("demasiados cursos de destino para una sola copia")

	// ErrIDInvalido: el ID recibido no tiene formato UUID válido.
	ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")

	// ErrReferenciaInexistente: SQLSTATE 23503 (foreign_key_violation) — el
	// request nombró un padre que no existe (un carro, un ciclo, una PC, un
	// usuario).
	ErrReferenciaInexistente = errors.New("alguno de los datos referenciados no existe")
)
