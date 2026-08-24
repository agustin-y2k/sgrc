package application

import (
	"context"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// Repo es el único contrato que este paquete necesita de infrastructure/ —
// nunca importar pgx directamente desde acá (ver docs/06-arquitectura.md §3).
type Repo interface {
	// EnTransaccion corre fn de forma atómica.
	EnTransaccion(ctx context.Context, fn func(Repo) error) error

	// Foto de perfil (tabla aparte: pesa cientos de veces más que el resto
	// de la fila y se lee en una sola pantalla, ver la migración 002).
	GuardarFoto(ctx context.Context, f *domain.FotoDePerfil) error
	BuscarFoto(ctx context.Context, usuarioID string) (*domain.FotoDePerfil, error)
	EliminarFoto(ctx context.Context, usuarioID string) error
	// UsuariosConFoto filtra, de una lista de ids, los que tienen foto.
	UsuariosConFoto(ctx context.Context, usuarioIDs []string) (map[string]bool, error)

	BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error)
	BuscarPorID(ctx context.Context, id string) (*domain.Usuario, error)
	// BuscarPorGoogleSub busca por el identificador estable de la cuenta de
	// Google. Devuelve ErrUsuarioNoEncontrado igual que las otras dos.
	BuscarPorGoogleSub(ctx context.Context, sub string) (*domain.Usuario, error)
	// Listar devuelve una página de usuarios filtrados por estado/rol (nil = sin
	// ese filtro) y el total que matchean.
	Listar(ctx context.Context, filtroEstado *domain.Estado, filtroRol *domain.Rol, pagina paginacion.Pagina) ([]*domain.Usuario, int, error)
	Crear(ctx context.Context, u *domain.Usuario) error
	Guardar(ctx context.Context, u *domain.Usuario) error
	ContarAdminsAprobados(ctx context.Context) (int, error)
	Eliminar(ctx context.Context, id string) error

	// ── Recuperación de contraseña ────────────────── CrearCodigoRecuperacion
	// invalida los códigos anteriores de esa persona y guarda el nuevo, de forma
	// atómica.
	CrearCodigoRecuperacion(ctx context.Context, c *domain.CodigoRecuperacion) error
	// BuscarCodigoVigenteDe devuelve el último código sin usar de esa persona, o
	// ErrCodigoNoEncontrado si no tiene ninguno.
	BuscarCodigoVigenteDe(ctx context.Context, usuarioID string) (*domain.CodigoRecuperacion, error)
	GuardarCodigoRecuperacion(ctx context.Context, c *domain.CodigoRecuperacion) error
}

// Estas funciones se inyectan (no se llaman directamente a paquetes externos)
// para que Service sea testeable con fakes, sin argon2 ni JWT reales — mismo
// patrón que internal/shared/adminseed.
type (
	HashFunc   func(password string) (string, error)
	VerifyFunc func(password, hash string) (bool, error)
	// TokenSigner firma la sesión. `recordarme` viene de la casilla del
	// ingreso y elige entre la vigencia normal y la larga (RF-01.13).
	TokenSigner         func(u *domain.Usuario, recordarme bool) (string, error)
	IDGenerator         func() string
	GenerarTemporalFunc func() (string, error)
	// GenerarCodigoFunc produce el código de recuperación que se manda por
	// mail: dígitos, corto, para tipear a mano desde el celular.
	GenerarCodigoFunc func() (string, error)
)

// IdentidadGoogle es lo que un ID token ya verificado afirma sobre quien lo
// presenta.
type IdentidadGoogle struct {
	// Sub es el identificador estable de la cuenta (claim `sub`). Es lo
	// que se guarda en usuario.google_sub.
	Sub string
	// Email tal como lo declara Google, sin normalizar — normalizarlo es
	// responsabilidad de este paquete, igual que con el registro común.
	Email string
	// EmailVerificado es el claim `email_verified`.
	EmailVerificado bool
	// Nombre y Apellido salen de given_name / family_name. Pueden venir
	// vacíos: no son claims obligatorios del estándar.
	Nombre   string
	Apellido string
}

// VerificadorGoogle valida un ID token contra las claves públicas de Google y
// devuelve lo que el token afirma.
type VerificadorGoogle interface {
	Verificar(ctx context.Context, idToken string) (*IdentidadGoogle, error)
}

// GestorMateriasDocente es el puerto hacia academic — necesario para la
// cascada de RF-02.8 (dar de baja al docente).
type GestorMateriasDocente interface {
	// MateriasDeDocente devuelve los IDs de materia a los que el usuario
	// está asignado.
	MateriasDeDocente(ctx context.Context, usuarioID string) ([]string, error)
	// QuedaOtroDocenteActivo indica si, excluyendo a usuarioIDExcluido, sigue
	// habiendo al menos otro docente APROBADA asignado a esa materia.
	QuedaOtroDocenteActivo(ctx context.Context, materiaID, usuarioIDExcluido string) (bool, error)
	// RemoverAsignacionesDeDocente elimina todas las filas docente_materia de
	// ese usuario (RF-02.8: se hace después de identificar las materias
	// huérfanas, nunca antes).
	RemoverAsignacionesDeDocente(ctx context.Context, usuarioID string) error
}

// CanceladorReservasDeMateria es el puerto hacia reservation — a diferencia
// de GestorMateriasDocente (una lectura/escritura simple sin máquina de
// estados, que sí va directo por SQL), esto es una ACCIÓN con reglas de
// negocio reales (cancelar una reserva, recalcular el estado de su
// ReservaGrupo padre), así que la implementación real vive en cmd/main.go
// envolviendo reservation.Service — nunca se reimplementa acá ni en
// infrastructure/ de este paquete.
type CanceladorReservasDeMateria interface {
	CancelarReservasFuturasDeMateria(ctx context.Context, materiaID, motivo string) (canceladas int, err error)
}
