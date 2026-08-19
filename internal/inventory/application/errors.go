package application

import "errors"

var (
	ErrCarroNoEncontrado      = errors.New("carro no encontrado")
	ErrEquipoNoEncontrado     = errors.New("equipo no encontrado")
	ErrIncidenciaNoEncontrada = errors.New("incidencia no encontrada")
	ErrLicenciaNoEncontrada   = errors.New("licencia no encontrada")

	// ErrEquipoPrestado: no se da de baja algo que está afuera del laboratorio.
	ErrEquipoPrestado = errors.New("ese equipo está prestado: marcá primero que volvió")

	// ErrLicenciaDuplicada: UNIQUE(equipo_id, lower(nombre)) — esa PC ya tiene
	// una licencia de ese software.
	ErrLicenciaDuplicada = errors.New("ese equipo ya tiene cargada una licencia de ese software")

	// ErrNombreCarroDuplicado: UNIQUE en carro.nombre — el nombre es lo único
	// que distingue a un carro en la pantalla de reservas, así que dos "Carro 1"
	// harían imposible saber cuál se está eligiendo.
	ErrNombreCarroDuplicado = errors.New("ya existe un carro con ese nombre")

	// ErrIdentificadorDuplicado: UNIQUE(carro_id, identificador) — "PC 27"
	// ya existe en ese carro puntual (puede repetirse en otro carro).
	ErrIdentificadorDuplicado = errors.New("ya existe un equipo con ese identificador en este carro")

	// ErrNombreDeEquipoDuplicado: entre los equipos que no están en ningún
	// carro, el nombre es lo único que los distingue.
	ErrNombreDeEquipoDuplicado = errors.New("ya existe un equipo con ese nombre")

	// ErrNumeroSerieDuplicado: UNIQUE global — el número de serie de
	// fábrica no puede repetirse en ninguna PC del sistema.
	ErrNumeroSerieDuplicado = errors.New("ya existe un equipo con ese número de serie")

	// ErrIDInvalido: mismo criterio que en academic — un ID sin formato
	// UUID válido se mapea a 400, no a 500.
	ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")

	// ErrReferenciaInexistente: SQLSTATE 23503 (foreign_key_violation) — el
	// request nombró un padre que no existe (un carro, un ciclo, una PC, un
	// usuario).
	ErrReferenciaInexistente = errors.New("alguno de los datos referenciados no existe")
)
