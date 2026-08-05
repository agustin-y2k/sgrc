package application

import "errors"

var (
	ErrCarroNoEncontrado      = errors.New("carro no encontrado")
	ErrPCNoEncontrada         = errors.New("PC no encontrada")
	ErrIncidenciaNoEncontrada = errors.New("incidencia no encontrada")

	// ErrIdentificadorDuplicado: UNIQUE(carro_id, identificador) — "PC 27"
	// ya existe en ese carro puntual (puede repetirse en otro carro).
	ErrIdentificadorDuplicado = errors.New("ya existe una PC con ese identificador en este carro")

	// ErrNumeroSerieDuplicado: UNIQUE global — el número de serie de
	// fábrica no puede repetirse en ninguna PC del sistema.
	ErrNumeroSerieDuplicado = errors.New("ya existe una PC con ese número de serie")

	// ErrIDInvalido: mismo criterio que en academic — un ID sin formato
	// UUID válido se mapea a 400, no a 500.
	ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")

	// ErrReferenciaInexistente: SQLSTATE 23503 (foreign_key_violation) — el
	// request nombró un padre que no existe (un carro, un ciclo, una PC, un
	// usuario). Es un error del cliente, no una falla del servidor: sin este
	// sentinel caía al 500 genérico de mapearError, que era el modo de falla
	// más común de toda la API para cualquier ID válido-pero-inexistente.
	ErrReferenciaInexistente = errors.New("alguno de los datos referenciados no existe")
)
