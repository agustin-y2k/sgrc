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

	// ── Ingreso con Google ───────────────────────────────────────────
	//
	// ErrLoginGoogleNoDisponible: no hay GOOGLE_CLIENT_ID configurado, así
	// que el sistema no tiene contra qué validar un token. No es un error
	// del cliente (el pedido está bien formado), es una capacidad que este
	// despliegue no tiene: se mapea a 503, no a 400.
	ErrLoginGoogleNoDisponible = errors.New("el ingreso con Google no está configurado en este sistema")

	// ErrTokenGoogleInvalido cubre todo lo que hace que un ID token no sea
	// creíble: firma que no valida, expirado, emitido para otra aplicación,
	// o directamente basura. Los casos NO se distinguen en el mensaje a
	// propósito — quien manda un token que no le pertenece no gana nada
	// sabiendo cuál de los chequeos lo frenó.
	ErrTokenGoogleInvalido = errors.New("el token de Google no es válido")

	// ErrEmailNoVerificadoPorGoogle: el token trae email_verified=false.
	// Sin esa garantía, cualquiera que pueda escribir una dirección ajena
	// en su perfil entraría a la cuenta de otra persona.
	ErrEmailNoVerificadoPorGoogle = errors.New("Google no confirmó que ese email sea tuyo; verificalo en tu cuenta de Google e intentá de nuevo")

	// ErrDominioNoPermitido: hay una lista de dominios habilitados
	// (GOOGLE_DOMINIOS_PERMITIDOS) y el email del token no pertenece a
	// ninguno. La devuelve el verificador de infrastructure/, que es quien
	// tiene esa configuración.
	ErrDominioNoPermitido = errors.New("esa cuenta de Google no pertenece a un dominio habilitado para esta institución")

	// ErrCuentaGoogleNoRegistrada: el token es válido pero no hay ninguna
	// cuenta para ese email. No es un fallo: es el camino normal la
	// primera vez, y lo que le dice al frontend que tiene que pedir curso
	// y materia antes de crear nada (POST /api/auth/google/registro).
	ErrCuentaGoogleNoRegistrada = errors.New("todavía no hay ninguna cuenta con ese email")

	// ErrCuentaSinPassword: se intentó cambiar la contraseña de una cuenta
	// que entra solo con Google. No tiene contraseña actual que verificar
	// ni nada que cambiar.
	ErrCuentaSinPassword = errors.New("esta cuenta ingresa con Google y no tiene contraseña; pedile a un Admin que te genere una si querés entrar también con email y contraseña")

	// ErrReferenciaInexistente: SQLSTATE 23503 (foreign_key_violation) — el
	// request nombró un padre que no existe (un carro, un ciclo, una PC, un
	// usuario). Es un error del cliente, no una falla del servidor: sin este
	// sentinel caía al 500 genérico de mapearError, que era el modo de falla
	// más común de toda la API para cualquier ID válido-pero-inexistente.
	ErrReferenciaInexistente = errors.New("alguno de los datos referenciados no existe")
)

const minPasswordLen = 8
