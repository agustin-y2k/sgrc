package application

import "errors"

// Errores de negocio de auth. Todos exportados para que interfaces/http
// pueda mapearlos a códigos HTTP específicos (ver docs/08-api-spec.yaml)
// sin tener que parsear mensajes de texto.
var (
	ErrUsuarioNoEncontrado   = errors.New("usuario no encontrado")
	ErrCredencialesInvalidas = errors.New("credenciales inválidas")
	ErrCuentaNoHabilitada    = errors.New("cuenta no habilitada")
	ErrEmailYaRegistrado     = errors.New("email ya registrado")

	// ErrCuentaEnBaja es el mensaje específico de RF-01.3 — distinto del
	// genérico ErrEmailYaRegistrado, para que quien vuelve entienda que
	// es su propia cuenta vieja y no un conflicto con otra persona.
	ErrCuentaEnBaja = errors.New("este email pertenece a una cuenta dada de baja — pedile a un Admin que la elimine para poder registrarte de nuevo")

	ErrPasswordCorta = errors.New("la contraseña debe tener al menos 8 caracteres")

	// ErrDatosObligatorios: nombre/apellido/email vacíos. Antes esto era un
	// fmt.Errorf suelto, sin sentinel, así que mapearError lo mandaba al 500
	// genérico: registrarse con el nombre vacío devolvía "error interno" en
	// vez de decir qué faltaba.
	ErrDatosObligatorios = errors.New("nombre, apellido y email son obligatorios")

	// ErrUltimoAdmin: RF-01.8 — el sistema nunca puede quedar sin ningún Admin.
	ErrUltimoAdmin = errors.New("no se puede dejar al sistema sin ningún admin activo")

	// ErrSoloDesdeBaja: RF-01.9 — el hard delete solo aplica a cuentas en BAJA.
	ErrSoloDesdeBaja = errors.New("solo se puede eliminar definitivamente una cuenta en estado BAJA")

	// ErrIDInvalido: mismo criterio que en academic/inventory/reservation
	// — un ID sin formato UUID válido se mapea a 400, no a 500. auth se
	// armó antes de que este bug apareciera por primera vez (en
	// academic); este sentinel se agregó retroactivamente al revisar
	// auth/infrastructure para la cascada de DarDeBaja y notar que le
	// faltaba el mismo chequeo que ya tienen todos los demás paquetes.
	ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")

	// ErrReferenciaInexistente: SQLSTATE 23503 (foreign_key_violation) — el
	// request nombró un padre que no existe (un carro, un ciclo, una PC, un
	// usuario). Es un error del cliente, no una falla del servidor: sin este
	// sentinel caía al 500 genérico de mapearError, que era el modo de falla
	// más común de toda la API para cualquier ID válido-pero-inexistente.
	ErrReferenciaInexistente = errors.New("alguno de los datos referenciados no existe")
)

const minPasswordLen = 8
