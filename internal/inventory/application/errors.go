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

	// ── Cuentas de equipo (RF-03.22) ──────────────────────────────────
	ErrCuentaDeEquipoNoEncontrada = errors.New("esa cuenta no existe")

	// ErrCuentaDeEquipoDuplicada: UNIQUE(equipo_id, usuario_normalizado) — dos
	// cuentas con el mismo nombre en la misma máquina no existen.
	ErrCuentaDeEquipoDuplicada = errors.New("ese equipo ya tiene una cuenta con ese nombre")

	// ErrSinClaveDeCifrado: el despliegue no configuró CUENTAS_SECRET, así que
	// puede registrar cuentas pero no guardar sus contraseñas. No es una falla
	// del sistema: es una función que ese despliegue no habilitó.
	ErrSinClaveDeCifrado = errors.New("este despliegue no puede guardar contraseñas de equipos: falta configurar CUENTAS_SECRET en el .env")

	// ErrNoAutorizado: se pidió revelar una contraseña marcada SOLO_ADMIN sin
	// ser ADMIN. Vive acá y no en el handler porque es una regla de negocio: si
	// estuviera en la capa HTTP, una ruta nueva podría saltearla sin notarlo.
	ErrNoAutorizado = errors.New("esa contraseña solo la puede ver un administrador")

	// ErrPasswordNoGuardada: se pidió ver una contraseña que no tenemos
	// anotada. Es el tercer estado —la cuenta pide contraseña y no la sabemos—
	// y decirlo es más útil que devolver un vacío que parece un error.
	ErrPasswordNoGuardada = errors.New("esa cuenta no tiene una contraseña anotada en el sistema")

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
