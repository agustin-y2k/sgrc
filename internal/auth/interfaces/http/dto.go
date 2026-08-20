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
type googleLoginRequest struct {
	Credential string `json:"credential"`
}

// googleRegistroRequest es el login con Google más lo único que el token no
// puede traer: qué va a dictar la persona (RF-01.3) y, si quiere corregirlo,
// su nombre tal como figura en la escuela.
type googleRegistroRequest struct {
	Credential string `json:"credential"`
	// Opcionales: vacíos, se usan los del token (given_name/family_name).
	Nombre   string `json:"nombre,omitempty"`
	Apellido string `json:"apellido,omitempty"`

	CursoSolicitado   string `json:"cursoSolicitado,omitempty"`
	MateriaSolicitada string `json:"materiaSolicitada,omitempty"`
	RolSolicitado     string `json:"rolSolicitado,omitempty"` // TITULAR | SUPLENTE
}

// actualizarMisDatosRequest es la edición del propio nombre desde Mi perfil.
type actualizarMisDatosRequest struct {
	Nombre   string `json:"nombre"`
	Apellido string `json:"apellido"`
}

// actualizarMisDatosResponse trae el usuario ya actualizado y, además, un
// token nuevo: el anterior lleva el nombre viejo en los claims y el cliente
// tiene que reemplazarlo para que deje de mentir.
type actualizarMisDatosResponse struct {
	Usuario usuarioResponse `json:"usuario"`
	Token   string          `json:"token"`
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

// restablecerPasswordRequest es el segundo: el código que llegó al mail más
// la contraseña elegida.
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

// configPublicaResponse es lo que la pantalla de login necesita saber antes
// de que haya alguien autenticado.
type configPublicaResponse struct {
	// Vacío = este despliegue no tiene ingreso con Google configurado, y
	// el frontend no muestra el botón.
	GoogleClientID string `json:"googleClientId"`

	// RemitenteDeCorreo es la dirección desde la que salen los avisos.
	RemitenteDeCorreo string `json:"remitenteDeCorreo,omitempty"`

	// false = no hay SMTP configurado, así que el sistema no puede mandar el
	// código de recuperación a ningún lado y la pantalla de login no muestra el
	// enlace "olvidé mi contraseña".
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
	// Lo que declaró al registrarse (RF-01.3).
	CursoSolicitado   string `json:"cursoSolicitado,omitempty"`
	MateriaSolicitada string `json:"materiaSolicitada,omitempty"`
	RolSolicitado     string `json:"rolSolicitado,omitempty"`

	// Cómo puede entrar esta cuenta.
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

// El meta salía de un paginationMeta local que se llenaba con {Total:
// len(data), Page: 1, PageSize: len(data)} — o sea, describía la respuesta en
// vez de la colección, y decía "página 1 de 1" siempre.
type listarUsuariosResponse struct {
	Data []usuarioResponse `json:"data"`
	Meta paginacion.Meta   `json:"meta"`
}
