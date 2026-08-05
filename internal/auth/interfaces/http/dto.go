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
	// Qué va a dictar. Opcionales: quien todavía no lo sepa se registra
	// igual y lo arregla con el Admin. Ver RF-01.3 / RF-02.6.
	CursoSolicitado   string `json:"cursoSolicitado,omitempty"`
	MateriaSolicitada string `json:"materiaSolicitada,omitempty"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
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
	// pantalla de aprobación para saber a qué materia y curso asignarlo.
	CursoSolicitado   string `json:"cursoSolicitado,omitempty"`
	MateriaSolicitada string `json:"materiaSolicitada,omitempty"`
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
