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
// Antes el único chequeo era `email == ""`, así que "no-es-un-email" entraba
// a la tabla como una cuenta más — y como el sistema no manda mails, nada
// lo desmentía después: la fila quedaba ahí, sin forma de contactar a esa
// persona y ocupando un lugar en el listado de pendientes.
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
}

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

// EsAdmin / EsDocente / EstaAprobado son helpers de lectura — evitan
// comparar u.Rol == domain.RolAdmin desperdigado por toda la aplicación.
func (u *Usuario) EsAdmin() bool      { return u.Rol == RolAdmin }
func (u *Usuario) EsDocente() bool    { return u.Rol == RolDocente }
func (u *Usuario) EstaAprobado() bool { return u.Estado == EstadoAprobada }
