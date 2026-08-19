package application

import "errors"

// Errores de negocio de academic. Todos exportados para que
// interfaces/http los mapee a códigos HTTP específicos sin parsear texto.
var (
	ErrCicloNoEncontrado = errors.New("ciclo lectivo no encontrado")
	ErrYaHayCicloActivo  = errors.New("ya existe un ciclo lectivo activo — archivalo antes de crear uno nuevo")
	ErrCicloYaTieneAnio  = errors.New("ya existe un ciclo lectivo para ese año")

	ErrCursoNoEncontrado    = errors.New("curso no encontrado")
	ErrCursoNombreDuplicado = errors.New("ya existe otro curso con ese nombre en el mismo ciclo lectivo")
	ErrCursoConReservas     = errors.New("el curso tiene materias con reservas asociadas — no se puede eliminar")

	ErrMateriaNoEncontrada    = errors.New("materia no encontrada")
	ErrMateriaNombreDuplicado = errors.New("ya existe otra materia con ese nombre en el mismo curso")
	ErrMateriaConReservas     = errors.New("la materia tiene reservas asociadas — no se puede eliminar")

	ErrDocenteMateriaNoEncontrado = errors.New("asignación docente-materia no encontrada")
	ErrUsuarioNoValidoParaAsignar = errors.New("el usuario no existe o no está en estado APROBADA")

	// ErrIDInvalido: el ID recibido no tiene formato UUID válido.
	ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")

	// ErrReferenciaInexistente: SQLSTATE 23503 (foreign_key_violation) — el
	// request nombró un padre que no existe (un carro, un ciclo, una PC, un
	// usuario).
	ErrReferenciaInexistente = errors.New("alguno de los datos referenciados no existe")
)
