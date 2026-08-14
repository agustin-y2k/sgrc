// Package http expone las rutas Fiber de auth — ver docs/08-api-spec.yaml
// para el contrato completo de cada endpoint.
package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// ── Requests ────────────────────────────────────────────────────────────

type registroRequest struct {
	Nombre   string `json:"nombre"`
	Apellido string `json:"apellido"`
	Email    string `json:"email"`
	Password string `json:"password"`
	// Qué va a dictar y con qué rol. Opcionales: quien todavía no lo sepa se
	// registra igual y lo arregla con el Admin. Ver RF-01.3 / RF-02.6.
	CursoSolicitado   string `json:"cursoSolicitado,omitempty"`
	MateriaSolicitada string `json:"materiaSolicitada,omitempty"`
	RolSolicitado     string `json:"rolSolicitado,omitempty"` // TITULAR | SUPLENTE
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// googleLoginRequest lleva el ID token que el navegador recibió de Google.
// Se llama "credential" porque es el nombre del campo con el que Google
// Identity Services se lo entrega al frontend — mantener el mismo nombre
// evita una traducción a mitad de camino que no aporta nada.
type googleLoginRequest struct {
	Credential string `json:"credential"`
}

// googleRegistroRequest es el login con Google más lo único que el token
// no puede traer: qué va a dictar la persona (RF-01.3) y, si quiere
// corregirlo, su nombre tal como figura en la escuela.
type googleRegistroRequest struct {
	Credential string `json:"credential"`
	// Opcionales: vacíos, se usan los del token (given_name/family_name).
	Nombre   string `json:"nombre,omitempty"`
	Apellido string `json:"apellido,omitempty"`

	CursoSolicitado   string `json:"cursoSolicitado,omitempty"`
	MateriaSolicitada string `json:"materiaSolicitada,omitempty"`
	RolSolicitado     string `json:"rolSolicitado,omitempty"` // TITULAR | SUPLENTE
}

type cambiarPasswordRequest struct {
	PasswordActual string `json:"passwordActual"`
	PasswordNueva  string `json:"passwordNueva"`
}

// cambiarPasswordResponse trae un token nuevo: el anterior lleva
// debeCambiarPassword=true congelado en los claims y el cliente tiene que
// reemplazarlo (RF-01.6).
type cambiarPasswordResponse struct {
	Token string `json:"token"`
}

// ── Recuperación de contraseña por autoservicio ─────────────────────────

// olvidePasswordRequest es el primer paso: solo la dirección a la que
// mandar el código.
type olvidePasswordRequest struct {
	Email string `json:"email"`
}

// restablecerPasswordRequest es el segundo: el código que llegó al mail
// más la contraseña elegida.
//
// El email viaja de nuevo y no en una sesión intermedia a propósito: sin
// estado del lado del servidor, el paso 2 funciona aunque la persona haya
// cerrado la pestaña, cambiado de dispositivo o pedido el código desde la
// computadora de la escuela y lo lea en el celular.
type restablecerPasswordRequest struct {
	Email         string `json:"email"`
	Codigo        string `json:"codigo"`
	PasswordNueva string `json:"passwordNueva"`
}

type cambiarEstadoRequest struct {
	Estado string `json:"estado"` // APROBADA | RECHAZADA | BAJA
}

type crearAdminRequest struct {
	Nombre   string `json:"nombre"`
	Apellido string `json:"apellido"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ── Responses ───────────────────────────────────────────────────────────

type loginResponse struct {
	Token               string `json:"token,omitempty"`
	DebeCambiarPassword bool   `json:"debeCambiarPassword"`
}

// configPublicaResponse es lo que la pantalla de login necesita saber
// antes de que haya alguien autenticado.
//
// El client ID de Google se sirve desde acá y no se compila dentro del
// bundle (VITE_…) a propósito: el frontend se construye una sola vez
// dentro de la imagen Docker, así que meterlo en el build obligaría a
// reconstruir la imagen para cambiarlo. No es un secreto — viaja en cada
// pedido a Google desde el navegador de todas formas.
type configPublicaResponse struct {
	// Vacío = este despliegue no tiene ingreso con Google configurado, y
	// el frontend no muestra el botón.
	GoogleClientID string `json:"googleClientId"`

	// RemitenteDeCorreo es la dirección desde la que salen los avisos.
	// Vacía si este despliegue no manda correos.
	//
	// La publica el servidor porque es lo único que convierte "revisá spam"
	// en algo accionable: lo que resuelve el problema de verdad es que la
	// persona marque ese remitente como conocido una vez. Escribirla a mano
	// en el frontend la dejaría desactualizada en la primera instalación
	// que use otra casilla.
	RemitenteDeCorreo string `json:"remitenteDeCorreo,omitempty"`

	// false = no hay SMTP configurado, así que el sistema no puede mandar
	// el código de recuperación a ningún lado y la pantalla de login no
	// muestra el enlace "olvidé mi contraseña". Mismo criterio que el botón
	// de Google: no ofrecer lo que este despliegue no puede hacer. La
	// salida en ese caso es que un Admin resetee la contraseña (RF-01.6).
	RecuperacionPorEmail bool `json:"recuperacionPorEmail"`
}

type usuarioResponse struct {
	ID                  string     `json:"id"`
	Nombre              string     `json:"nombre"`
	Apellido            string     `json:"apellido"`
	Email               string     `json:"email"`
	Rol                 string     `json:"rol"`
	Estado              string     `json:"estado"`
	FechaRegistro       time.Time  `json:"fechaRegistro"`
	FechaAprobacion     *time.Time `json:"fechaAprobacion,omitempty"`
	DebeCambiarPassword bool       `json:"debeCambiarPassword"`
	// Lo que declaró al registrarse (RF-01.3). Es lo que el Admin mira en la
	// pantalla de aprobación para saber a qué materia y curso asignarlo, y
	// con qué rol.
	CursoSolicitado   string `json:"cursoSolicitado,omitempty"`
	MateriaSolicitada string `json:"materiaSolicitada,omitempty"`
	RolSolicitado     string `json:"rolSolicitado,omitempty"`

	// Cómo puede entrar esta cuenta. Nunca se expone el google_sub en sí:
	// alcanza con saber si el vínculo existe, y el identificador de la
	// cuenta de Google de una persona no es asunto de nadie más.
	//
	// TienePassword es lo que le permite a la pantalla de perfil no
	// ofrecerle "cambiar contraseña" a quien entra con Google y no tiene
	// ninguna (el backend responde 409, pero es mejor no mostrar el
	// formulario que explicar el error después).
	TienePassword    bool `json:"tienePassword"`
	VinculadaAGoogle bool `json:"vinculadaAGoogle"`
}

func toUsuarioResponse(u *domain.Usuario) usuarioResponse {
	return usuarioResponse{
		ID:                  u.ID,
		Nombre:              u.Nombre,
		Apellido:            u.Apellido,
		Email:               u.Email,
		Rol:                 string(u.Rol),
		Estado:              string(u.Estado),
		FechaRegistro:       u.FechaRegistro,
		FechaAprobacion:     u.FechaAprobacion,
		DebeCambiarPassword: u.DebeCambiarPassword,
		CursoSolicitado:     u.CursoSolicitado,
		MateriaSolicitada:   u.MateriaSolicitada,
		RolSolicitado:       u.RolSolicitado,
		TienePassword:       u.PuedeIngresarConPassword(),
		VinculadaAGoogle:    u.PuedeIngresarConGoogle(),
	}
}

type resetPasswordResponse struct {
	PasswordTemporal string `json:"passwordTemporal"`
}

// El meta salía de un paginationMeta local que se llenaba con
// {Total: len(data), Page: 1, PageSize: len(data)} — o sea, describía la
// respuesta en vez de la colección, y decía "página 1 de 1" siempre. Ahora
// es el tipo compartido y lo completa la ventana real (ver
// internal/shared/paginacion).
type listarUsuariosResponse struct {
	Data []usuarioResponse `json:"data"`
	Meta paginacion.Meta   `json:"meta"`
}
