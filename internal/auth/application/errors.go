package application

import "errors"

// Errores de negocio de auth.
var (
	ErrUsuarioNoEncontrado   = errors.New("usuario no encontrado")
	ErrCredencialesInvalidas = errors.New("credenciales inválidas")
	ErrEmailYaRegistrado     = errors.New("email ya registrado")

	// ErrCuentaNoHabilitada es el paraguas de "la contraseña estaba bien pero
	// esta cuenta no puede entrar".
	ErrCuentaNoHabilitada = errors.New("cuenta no habilitada")

	// Los tres motivos concretos por los que una cuenta existente no entra.
	ErrIngresoCuentaPendiente = &errorDeCuenta{
		"tu cuenta todavía está esperando la aprobación de un Admin — vas a poder entrar apenas la aprueben",
	}
	ErrIngresoCuentaRechazada = &errorDeCuenta{
		"tu solicitud de cuenta fue rechazada; si creés que es un error, hablá con el equipo de administración de la escuela",
	}
	// Distinto de ErrCuentaEnBaja: aquel es el del registro ("no podés crear una
	// cuenta con este email"), este es el del ingreso ("no podés entrar").
	ErrIngresoCuentaEnBaja = &errorDeCuenta{
		"esta cuenta fue dada de baja y no se puede volver a habilitar",
	}

	// ErrCuentaEnBaja es el mensaje específico de RF-01.3 — distinto del
	// genérico ErrEmailYaRegistrado, para que quien vuelve entienda que es su
	// propia cuenta vieja y no un conflicto con otra persona.
	ErrCuentaEnBaja = errors.New("este email pertenece a una cuenta dada de baja — pedile a un Admin que la elimine para poder registrarte de nuevo")

	ErrPasswordCorta = errors.New("la contraseña debe tener al menos 8 caracteres")

	// ErrPasswordActualIncorrecta: la contraseña que se escribió en "actual" al
	// cambiarla no es la de la cuenta.
	ErrPasswordActualIncorrecta = errors.New("la contraseña actual no es correcta")

	// ErrDatosObligatorios: nombre/apellido/email vacíos.
	ErrDatosObligatorios = errors.New("nombre, apellido y email son obligatorios")

	// ErrCargoObligatorio y ErrRolSolicitadoObligatorio: los dos únicos campos
	// que el registro exige además del nombre, el email y la contraseña. El
	// curso y la materia siguen siendo opcionales — quien todavía no sabe qué
	// va a dictar se registra igual y lo arregla con el Admin (RF-01.3).
	ErrCargoObligatorio = errors.New("hay que elegir con qué cargo te registrás: docente o administrador de sistema")

	ErrRolSolicitadoObligatorio = errors.New("hay que elegir si sos titular o suplente")

	// ErrUltimoAdmin: RF-01.8 — el sistema nunca puede quedar sin ningún Admin.
	ErrUltimoAdmin = errors.New("no se puede dejar al sistema sin ningún admin activo")

	// ErrAutoDegradacion: quitarse los permisos de Admin a uno mismo.
	ErrAutoDegradacion = errors.New("no podés quitarte a vos mismo los permisos de Admin — pedíselo a otro Admin")

	// ErrSoloDesdeBajaORechazada: RF-01.9 — el hard delete solo aplica a cuentas
	// ya cerradas, que son las dos terminales de la máquina de estados: BAJA
	// (llegó a estar aprobada y se cerró) y RECHAZADA (nunca se aprobó).
	ErrSoloDesdeBajaORechazada = errors.New("solo se puede eliminar definitivamente una cuenta en estado BAJA o RECHAZADA")

	// ErrIDInvalido: mismo criterio que en academic/inventory/reservation — un
	// ID sin formato UUID válido se mapea a 400 y no a 500, porque es un error
	// del cliente y no una falla del servidor.
	ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")

	// ── Ingreso con Google ───────────────────────────────────────────
	// ErrLoginGoogleNoDisponible: no hay GOOGLE_CLIENT_ID configurado, así que
	// el sistema no tiene contra qué validar un token.
	ErrLoginGoogleNoDisponible = errors.New("el ingreso con Google no está configurado en este sistema")

	// ErrTokenGoogleInvalido cubre todo lo que hace que un ID token no sea
	// creíble: firma que no valida, expirado, emitido para otra aplicación, o
	// directamente basura.
	ErrTokenGoogleInvalido = errors.New("el token de Google no es válido")

	// ErrEmailNoVerificadoPorGoogle: el token trae email_verified=false.
	ErrEmailNoVerificadoPorGoogle = errors.New("Google no confirmó que ese email sea tuyo; verificalo en tu cuenta de Google e intentá de nuevo")

	// ErrDominioNoPermitido: hay una lista de dominios habilitados
	// (GOOGLE_DOMINIOS_PERMITIDOS) y el email del token no pertenece a ninguno.
	ErrDominioNoPermitido = errors.New("esa cuenta de Google no pertenece a un dominio habilitado para esta institución")

	// ErrCuentaGoogleNoRegistrada: el token es válido pero no hay ninguna cuenta
	// para ese email.
	ErrCuentaGoogleNoRegistrada = errors.New("todavía no hay ninguna cuenta con ese email")

	// ErrCuentaSinPassword: se intentó cambiar la contraseña de una cuenta que
	// entra solo con Google.
	ErrCuentaSinPassword = errors.New("esta cuenta ingresa con Google y no tiene contraseña; pedile a un Admin que te genere una si querés entrar también con email y contraseña")

	// ── Recuperación de contraseña por autoservicio ──────────────────
	// ErrCodigoNoEncontrado lo devuelve el repositorio cuando la persona no
	// tiene ningún código sin usar.
	ErrCodigoNoEncontrado = errors.New("no hay ningún código de recuperación pendiente")

	// ErrCodigoRecuperacionInvalido es lo ÚNICO que ve quien manda un código que
	// no sirve, sin importar si el email no existe, si la persona nunca pidió un
	// código o si el código es de otra cuenta.
	ErrCodigoRecuperacionInvalido = errors.New("el código no es válido o ya venció; pedí uno nuevo")

	// ErrCodigoRecuperacionVencido y ErrCodigoRecuperacionSinIntentos sí se
	// distinguen: le pasan a la persona legítima, que ya demostró tener acceso a
	// la casilla, y necesita saber que tiene que pedir otro código en vez de
	// seguir tipeando el mismo.
	ErrCodigoRecuperacionVencido = errors.New("el código venció; pedí uno nuevo")

	ErrCodigoRecuperacionSinIntentos = errors.New("se agotaron los intentos para ese código; pedí uno nuevo")

	// ErrRecuperacionNoDisponible: no hay SMTP configurado, así que el sistema
	// no puede mandar el código a ningún lado.
	ErrRecuperacionNoDisponible = errors.New("la recuperación de contraseña por email no está configurada en este sistema; pedile a un Admin que te resetee la contraseña")

	// ErrReferenciaInexistente: SQLSTATE 23503 (foreign_key_violation) — el
	// request nombró un padre que no existe (un carro, un ciclo, una PC, un
	// usuario).
	ErrReferenciaInexistente = errors.New("alguno de los datos referenciados no existe")
)

// errorDeCuenta es un sentinel que además matchea contra
// ErrCuentaNoHabilitada.
type errorDeCuenta struct{ mensaje string }

func (e *errorDeCuenta) Error() string { return e.mensaje }

// Is hace que errors.Is(err, ErrCuentaNoHabilitada) siga siendo verdadero
// para los tres motivos.
func (e *errorDeCuenta) Is(objetivo error) bool { return objetivo == ErrCuentaNoHabilitada }

const minPasswordLen = 8
