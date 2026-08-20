// Package domain contiene la entidad Usuario y sus reglas de negocio puras —
// sin Postgres, sin Fiber, sin nada externo.
package domain

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
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

// ParseRol valida un string contra los roles conocidos.
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

// Los dos valores que admite RolSolicitado.
const (
	RolSolicitadoTitular  = "TITULAR"
	RolSolicitadoSuplente = "SUPLENTE"
)

// ErrRolSolicitadoInvalido se devuelve cuando el registro declara un rol que
// no es titular ni suplente.
var ErrRolSolicitadoInvalido = errors.New("rol solicitado inválido")

// NormalizarRolSolicitado acepta el vacío —declararlo es opcional, igual que
// el curso y la materia— y rechaza cualquier otra cosa.
func NormalizarRolSolicitado(s string) (string, error) {
	switch s {
	case "", RolSolicitadoTitular, RolSolicitadoSuplente:
		return s, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrRolSolicitadoInvalido, s)
	}
}

// Los dos valores que admite CargoSolicitado.
const (
	// CargoSolicitadoDocente es quien da clase frente a alumnos.
	CargoSolicitadoDocente = "DOCENTE"

	// CargoSolicitadoAdminSistema cubre al auxiliar informático, al
	// administrador de red y a los demás cargos docentes que administran el
	// laboratorio sin estar frente a alumnos.
	CargoSolicitadoAdminSistema = "ADMIN_SISTEMA"
)

// ErrCargoSolicitadoInvalido se devuelve cuando el registro declara un cargo
// que no es ninguno de los dos.
var ErrCargoSolicitadoInvalido = errors.New("cargo solicitado inválido")

// NormalizarCargoSolicitado acepta el vacío —las cuentas anteriores a esta
// columna no declararon ninguno, y un ADMIN creado por otro ADMIN tampoco— y
// rechaza cualquier otra cosa. Que el registro EXIJA declararlo es una regla
// del caso de uso, no del dato: vive en Registrar (ver application/service.go).
//
// Ojo con el nombre: el cargo declarado NO es domain.Rol y no otorga ningún
// permiso. Es una declaración, igual que el curso y la materia.
func NormalizarCargoSolicitado(s string) (string, error) {
	switch s {
	case "", CargoSolicitadoDocente, CargoSolicitadoAdminSistema:
		return s, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrCargoSolicitadoInvalido, s)
	}
}

// PuedeTransicionarA implementa el diagrama de estados de Usuario
// (docs/05-diagramas-estado.md): PENDIENTE puede ir a APROBADA o RECHAZADA;
// APROBADA puede ir a BAJA; RECHAZADA y BAJA son terminales — ninguna
// transición sale de ahí, ni siquiera de vuelta a APROBADA (RF-02.9: la baja
// es permanente).
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

// ErrTransicionInvalida envuelve cualquier intento de transición no permitida
// por PuedeTransicionarA — incluye el estado actual y el destino pedido en el
// mensaje para que el error sea depurable sin tener que ir a buscar el
// diagrama de estados.
var ErrTransicionInvalida = errors.New("transición de estado inválida")

// LargoMaxNombre es lo que entra en las columnas nombre y apellido
// (VARCHAR(100) en migrations/001_esquema_inicial.sql).
const LargoMaxNombre = 100

// ErrNombreVacio y ErrNombreDemasiadoLargo son las dos formas en que un
// nombre no sirve.
var (
	ErrNombreVacio = errors.New("el nombre y el apellido son obligatorios")

	ErrNombreDemasiadoLargo = fmt.Errorf(
		"el nombre y el apellido no pueden tener más de %d caracteres", LargoMaxNombre)
)

// NormalizarNombreYApellido recorta los espacios y aplica la única regla que
// tiene un nombre propio en este sistema: que exista y que entre en la
// columna. Vive en el dominio porque son dos las puertas por las que se
// escribe —el registro y la edición del propio perfil— y hasta ahora el largo
// lo cortaba solamente el formulario del navegador.
func NormalizarNombreYApellido(nombre, apellido string) (string, string, error) {
	nombre, apellido = strings.TrimSpace(nombre), strings.TrimSpace(apellido)
	if nombre == "" || apellido == "" {
		return "", "", ErrNombreVacio
	}
	// Se cuentan runas y no bytes: VARCHAR(100) en Postgres son 100
	// caracteres, y un apellido con eñes o acentos ocupa más bytes que letras.
	if utf8.RuneCountInString(nombre) > LargoMaxNombre || utf8.RuneCountInString(apellido) > LargoMaxNombre {
		return "", "", ErrNombreDemasiadoLargo
	}
	return nombre, apellido, nil
}

// ErrEmailInvalido se devuelve cuando un string no tiene forma de email.
var ErrEmailInvalido = errors.New("el email no tiene un formato válido")

// NormalizarEmail deja el email en la forma canónica con la que se guarda y
// se busca: sin espacios alrededor y en minúsculas.
func NormalizarEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidarEmail acepta lo que razonablemente es una dirección de correo.
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

	// Lo que la persona declaró al registrarse: qué curso y qué materia va a
	// dictar.
	CursoSolicitado   string
	MateriaSolicitada string

	// RolSolicitado es si se ofrece como titular o como suplente.
	RolSolicitado string

	// CargoSolicitado es qué dijo ser al registrarse: DOCENTE o
	// ADMIN_SISTEMA. Vacío en las cuentas anteriores a la columna y en los
	// Admin creados por otro Admin. No otorga permisos — ver
	// NormalizarCargoSolicitado.
	CargoSolicitado string

	// GoogleSub es el claim `sub` del ID token de Google: el identificador
	// estable de esa cuenta.
	GoogleSub string

	// VersionSesion es el contador que permite echar a las sesiones abiertas
	// cuando la contraseña cambia (ver InvalidarSesiones y
	// migrations/001_esquema_inicial.sql).
	VersionSesion int
}

// InvalidarSesiones hace que todos los tokens ya emitidos para esta cuenta
// dejen de valer a partir del request siguiente (RF-01.11).
func (u *Usuario) InvalidarSesiones() { u.VersionSesion++ }

// PuedeIngresarConPassword indica si la cuenta tiene contraseña propia.
func (u *Usuario) PuedeIngresarConPassword() bool { return u.PasswordHash != "" }

// PuedeIngresarConGoogle indica si la cuenta está vinculada a una cuenta de
// Google.
func (u *Usuario) PuedeIngresarConGoogle() bool { return u.GoogleSub != "" }

// CambiarEstado aplica una transición si es válida, o devuelve
// ErrTransicionInvalida si no.
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

// ErrPromocionInvalida envuelve cualquier intento de promover una cuenta que
// no está en condiciones.
var ErrPromocionInvalida = errors.New("no se puede promover esta cuenta a ADMIN")

// PromoverAAdmin convierte a un docente en Admin.
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
// Admin a alguien y lo deja como docente.
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
