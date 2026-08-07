// Package domain contiene la entidad Usuario y sus reglas de negocio puras
// — sin Postgres, sin Fiber, sin nada externo. Ver docs/03-diagrama-clases.md
// y docs/05-diagramas-estado.md para el modelo completo.
package domain

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

// Rol de un usuario. Solo dos valores posibles en todo el sistema
// (RF-01, docs/01-requisitos.md).
type Rol string

const (
	RolAdmin   Rol = "ADMIN"
	RolDocente Rol = "DOCENTE"
)

// ErrRolInvalido se devuelve cuando un string no mapea a ningún Rol conocido.
var ErrRolInvalido = errors.New("rol inválido")

// ParseRol valida un string contra los roles conocidos. Nunca construir un
// Rol directamente desde un string sin pasar por acá (evita que un typo o
// un dato corrupto de la base termine comparado silenciosamente contra
// nada).
func ParseRol(s string) (Rol, error) {
	switch Rol(s) {
	case RolAdmin, RolDocente:
		return Rol(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrRolInvalido, s)
	}
}

// Estado de la cuenta. BAJA y RECHAZADA son terminales — ver
// docs/05-diagramas-estado.md.
type Estado string

const (
	EstadoPendiente Estado = "PENDIENTE"
	EstadoAprobada  Estado = "APROBADA"
	EstadoRechazada Estado = "RECHAZADA"
	EstadoBaja      Estado = "BAJA"
)

// ErrEstadoInvalido se devuelve cuando un string no mapea a ningún Estado conocido.
var ErrEstadoInvalido = errors.New("estado inválido")

// ParseEstado valida un string contra los estados conocidos.
func ParseEstado(s string) (Estado, error) {
	switch Estado(s) {
	case EstadoPendiente, EstadoAprobada, EstadoRechazada, EstadoBaja:
		return Estado(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrEstadoInvalido, s)
	}
}

// PuedeTransicionarA implementa el diagrama de estados de Usuario
// (docs/05-diagramas-estado.md): PENDIENTE puede ir a APROBADA o
// RECHAZADA; APROBADA puede ir a BAJA; RECHAZADA y BAJA son terminales —
// ninguna transición sale de ahí, ni siquiera de vuelta a APROBADA
// (RF-02.9: la baja es permanente).
func (e Estado) PuedeTransicionarA(nuevo Estado) bool {
	switch e {
	case EstadoPendiente:
		return nuevo == EstadoAprobada || nuevo == EstadoRechazada
	case EstadoAprobada:
		return nuevo == EstadoBaja
	case EstadoRechazada, EstadoBaja:
		return false
	default:
		return false
	}
}

// ErrTransicionInvalida envuelve cualquier intento de transición no
// permitida por PuedeTransicionarA — incluye el estado actual y el
// destino pedido en el mensaje para que el error sea depurable sin tener
// que ir a buscar el diagrama de estados.
var ErrTransicionInvalida = errors.New("transición de estado inválida")

// ErrEmailInvalido se devuelve cuando un string no tiene forma de email.
var ErrEmailInvalido = errors.New("el email no tiene un formato válido")

// NormalizarEmail deja el email en la forma canónica con la que se guarda y
// se busca: sin espacios alrededor y en minúsculas.
//
// Es una regla de identidad, no cosmética. Sin esto, "Juan.Perez@escuela.ar"
// y "juan.perez@escuela.ar" eran dos cuentas distintas para el mismo buzón
// —el UNIQUE de Postgres compara byte a byte— y quien se registraba con una
// y después escribía la otra recibía "credenciales inválidas" sin ninguna
// pista de por qué. Se aplica en el único lugar donde un email entra al
// sistema (crearUsuario) y en el único donde se busca (Login), así que las
// dos puntas usan siempre la misma forma.
//
// Solo se toca el dominio conceptual, no la parte local: aunque el RFC 5321
// permite que "A@x" y "a@x" sean buzones distintos, ningún proveedor real lo
// hace y la institución no es la excepción.
func NormalizarEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidarEmail acepta lo que razonablemente es una dirección de correo.
//
// Con un chequeo de `email == ""` a secas, "no-es-un-email" entra a la
// tabla como una cuenta más y nada lo desmiente después: la fila queda ahí,
// sin forma de contactar a esa persona y ocupando un lugar en el listado de
// pendientes.
//
// Se exige un punto en el dominio además de lo que pide mail.ParseAddress:
// "docente@escuela" es válido para el RFC pero en la práctica siempre es un
// tipeo incompleto.
func ValidarEmail(email string) error {
	direccion, err := mail.ParseAddress(email)
	// ParseAddress también acepta la forma "Nombre <a@b.com>"; exigir que lo
	// parseado sea idéntico a lo recibido descarta esa variante, que no es lo
	// que se espera en un campo "email" de un formulario.
	if err != nil || direccion.Address != email {
		return fmt.Errorf("%w: %q", ErrEmailInvalido, email)
	}
	arroba := strings.LastIndex(email, "@")
	if dominio := email[arroba+1:]; !strings.Contains(dominio, ".") {
		return fmt.Errorf("%w: al dominio %q le falta la extensión", ErrEmailInvalido, dominio)
	}
	return nil
}

// Usuario es la entidad — Admins y Docentes viven en la misma tabla,
// distinguidos por Rol (ver docs/07-modelo-datos.md).
type Usuario struct {
	ID                  string
	Nombre              string
	Apellido            string
	Email               string
	PasswordHash        string
	DebeCambiarPassword bool
	Rol                 Rol
	Estado              Estado
	FechaRegistro       time.Time
	FechaAprobacion     *time.Time
	AprobadoPor         *string

	// Lo que la persona declaró al registrarse: qué curso y qué materia va
	// a dictar. Es texto libre y puede venir vacío.
	//
	// No son referencias a Curso ni a Materia a propósito: al registrarse
	// todavía no está autenticada, así que no puede elegir de una lista, y
	// lo que dice puede no existir todavía en el sistema —de hecho el Admin
	// quizás lo tenga que crear al aprobarla (RF-02.6)—. Es una declaración
	// de intención para que el Admin sepa a qué asignarla, no un vínculo.
	CursoSolicitado   string
	MateriaSolicitada string

	// GoogleSub es el claim `sub` del ID token de Google: el identificador
	// estable de esa cuenta. Vacío en las cuentas que solo entran con
	// contraseña.
	//
	// Se guarda el sub y no el email porque el email de una cuenta de
	// Google puede cambiar y el sub no. El email sigue siendo la identidad
	// dentro del sistema; el vínculo con Google cuelga del sub.
	GoogleSub string

	// VersionSesion es el contador que permite echar a las sesiones
	// abiertas cuando la contraseña cambia (ver InvalidarSesiones y
	// migrations/010). Viaja dentro del JWT y el middleware lo compara
	// contra el de la fila en cada request.
	VersionSesion int
}

// InvalidarSesiones hace que todos los tokens ya emitidos para esta cuenta
// dejen de valer a partir del request siguiente (RF-01.11).
//
// Es la única forma permitida de tocar VersionSesion desde fuera de este
// paquete, igual que CambiarEstado con el estado: siendo un método, un grep
// encuentra todos los lugares donde una sesión se corta.
//
// La llaman los tres caminos por los que cambia una contraseña (RF-01.6,
// RF-01.7 y RF-01.10), por el caso que le da sentido: quien sospecha que
// entraron a su cuenta la cambia para cortar el acceso del otro, no para
// que siga adentro hasta que se le venza el token.
//
// Quien además emite un token nuevo tiene que hacerlo DESPUÉS de esto, o el
// token que entrega nace inválido.
func (u *Usuario) InvalidarSesiones() { u.VersionSesion++ }

// PuedeIngresarConPassword indica si la cuenta tiene contraseña propia.
//
// Es falso en las cuentas creadas con Google, que no tienen ninguna: a
// quien las verifica es Google. Sin este chequeo, el login local llamaría
// a verify() contra un hash vacío —error de formato, o sea 500— y
// CambiarPassword pediría "la contraseña actual" de algo que no existe.
func (u *Usuario) PuedeIngresarConPassword() bool { return u.PasswordHash != "" }

// PuedeIngresarConGoogle indica si la cuenta está vinculada a una cuenta
// de Google. Una misma cuenta puede tener las dos formas de ingreso: un
// docente que se registró con contraseña y después entra con Google
// conserva las dos (ver migrations/008_login_con_google.sql).
func (u *Usuario) PuedeIngresarConGoogle() bool { return u.GoogleSub != "" }

// CambiarEstado aplica una transición si es válida, o devuelve
// ErrTransicionInvalida si no. Es la única forma permitida de cambiar
// u.Estado — nunca asignarlo directamente desde fuera de este paquete.
func (u *Usuario) CambiarEstado(nuevo Estado, ahora time.Time) error {
	if !u.Estado.PuedeTransicionarA(nuevo) {
		return fmt.Errorf("%w: de %s a %s", ErrTransicionInvalida, u.Estado, nuevo)
	}
	u.Estado = nuevo
	if nuevo == EstadoAprobada {
		u.FechaAprobacion = &ahora
	}
	return nil
}

// ErrPromocionInvalida envuelve cualquier intento de promover una cuenta
// que no está en condiciones. Incluye el motivo en el mensaje para que el
// Admin sepa qué le falta a esa cuenta sin tener que adivinar.
var ErrPromocionInvalida = errors.New("no se puede promover esta cuenta a ADMIN")

// PromoverAAdmin convierte a un docente en Admin. Es la única forma
// permitida de cambiar u.Rol — igual que con CambiarEstado, nunca asignarlo
// directamente desde fuera de este paquete.
//
// Las dos condiciones no son burocracia:
//
//   - Tiene que estar APROBADA. Promover una cuenta PENDIENTE sería
//     aprobarla por la puerta de atrás, salteándose el paso donde alguien
//     mira quién es esa persona (RF-01.3). Y sobre una RECHAZADA o en BAJA
//     no tiene ningún sentido: son estados terminales.
//   - No puede ser Admin ya. No es un no-op silencioso a propósito: si el
//     Admin apretó "promover" sobre alguien que ya lo era, se equivocó de
//     fila, y decírselo es mejor que dejarlo creyendo que hizo algo.
//
// La operación inversa es DegradarADocente, más abajo.
func (u *Usuario) PromoverAAdmin() error {
	if u.EsAdmin() {
		return fmt.Errorf("%w: ya tiene rol ADMIN", ErrPromocionInvalida)
	}
	if !u.EstaAprobado() {
		return fmt.Errorf("%w: primero hay que aprobar la cuenta (está en %s)", ErrPromocionInvalida, u.Estado)
	}
	u.Rol = RolAdmin
	return nil
}

// ErrDegradacionInvalida envuelve cualquier intento de quitarle el rol
// ADMIN a una cuenta que no está en condiciones, con el motivo adentro.
var ErrDegradacionInvalida = errors.New("no se puede quitar el rol ADMIN de esta cuenta")

// DegradarADocente es la inversa de PromoverAAdmin: le saca los permisos de
// Admin a alguien y lo deja como docente. Junto con PromoverAAdmin son las
// dos únicas formas permitidas de cambiar u.Rol.
//
// Las dos condiciones son el espejo de las de promover:
//
//   - Tiene que ser Admin. Sobre un docente no hay nada que quitar, y
//     decirlo es mejor que un no-op silencioso: quien apretó el botón se
//     equivocó de fila.
//   - Tiene que estar APROBADA. En una cuenta RECHAZADA o en BAJA el rol ya
//     no habilita nada —esos estados no entran al sistema—, así que
//     cambiarlo sería tocar el historial de una cuenta cerrada sin efecto
//     ninguno.
//
// Lo que este método NO puede ver es si es el último Admin del sistema
// (RF-01.8): eso se cuenta contra la base y vive en application, igual que
// para la baja. Degradar al único Admin dejaría al sistema sin nadie que
// pueda aprobar cuentas ni volver a promover a nadie — sin salida.
//
// No toca nada más de la cuenta, por la misma razón que promover: conserva
// sus materias y sus reservas. Quien deja de coordinar sigue dando clase.
func (u *Usuario) DegradarADocente() error {
	if u.EsDocente() {
		return fmt.Errorf("%w: ya tiene rol DOCENTE", ErrDegradacionInvalida)
	}
	if !u.EstaAprobado() {
		return fmt.Errorf("%w: la cuenta está en %s y el rol ya no le habilita nada", ErrDegradacionInvalida, u.Estado)
	}
	u.Rol = RolDocente
	return nil
}

// EsAdmin / EsDocente / EstaAprobado son helpers de lectura — evitan
// comparar u.Rol == domain.RolAdmin desperdigado por toda la aplicación.
func (u *Usuario) EsAdmin() bool      { return u.Rol == RolAdmin }
func (u *Usuario) EsDocente() bool    { return u.Rol == RolDocente }
func (u *Usuario) EstaAprobado() bool { return u.Estado == EstadoAprobada }
