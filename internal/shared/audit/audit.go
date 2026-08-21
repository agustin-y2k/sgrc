// Package audit persiste el registro de auditoría (ver
// docs/09-seguridad-rbac.md §5): quién hizo qué acción administrativa
// sensible, sobre qué entidad, y cuándo.
package audit

import "context"

// Acciones auditadas — mismo catálogo que docs/09-seguridad-rbac.md §5.
const (
	CuentaAprobada            = "CUENTA_APROBADA"
	CuentaRechazada           = "CUENTA_RECHAZADA"
	CuentaBaja                = "CUENTA_BAJA"
	CuentaEliminadaDefinitiva = "CUENTA_ELIMINADA_DEFINITIVAMENTE"
	AdminCreado               = "ADMIN_CREADO"
	RolPromovidoAAdmin        = "ROL_PROMOVIDO_A_ADMIN"
	RolDegradadoADocente      = "ROL_DEGRADADO_A_DOCENTE"
	PasswordReseteada         = "PASSWORD_RESETEADA"
	// NombreCambiado es la única acción del catálogo que alguien hace sobre su
	// propia cuenta sin ser Admin: cambiar el nombre cambia lo que el resto de
	// la escuela ve en las reservas y en las entregas.
	NombreCambiado = "NOMBRE_CAMBIADO"
	// PasswordRecuperadaPorEmail es la única acción de este catálogo cuyo actor
	// NO está autenticado: la persona probó ser dueña de la cuenta con el código
	// que le llegó al mail, no con un token.
	PasswordRecuperadaPorEmail = "PASSWORD_RECUPERADA_POR_EMAIL"
	DocenteRemovidoDeMateria   = "DOCENTE_REMOVIDO_DE_MATERIA"
	DocenteRolCambiado         = "DOCENTE_ROL_CAMBIADO"
	ReservaCanceladaPorAdmin   = "RESERVA_CANCELADA_POR_ADMIN"
	BloqueoCreado              = "BLOQUEO_CREADO"
	// Los VALORES de estas constantes no se renombran nunca, aunque el sistema
	// renombre la entidad: lo guardado es el nombre que la operación tenía
	// cuando ocurrió, y reescribir un registro de auditoría es precisamente lo
	// que un registro de auditoría no debe permitir.
	EquipoEstadoCambiado       = "EQUIPO_ESTADO_CAMBIADO"
	EquipoDadoDeBaja           = "EQUIPO_DADO_DE_BAJA"
	EquipoMovidoDeCarro        = "EQUIPO_MOVIDO_DE_CARRO"
	CursoEliminado             = "CURSO_ELIMINADO"
	MateriaEliminada           = "MATERIA_ELIMINADA"
	CicloArchivadoReservasElim = "CICLO_ARCHIVADO_RESERVAS_ELIMINADAS"
	CicloClonado               = "CICLO_CLONADO"
	// Un pedido para dictar una materia se resolvió.
	PedidoDeMateriaAprobado  = "PEDIDO_DE_MATERIA_APROBADO"
	PedidoDeMateriaRechazado = "PEDIDO_DE_MATERIA_RECHAZADO"
)

// Entrada es una fila de audit_log (ver migrations/001_esquema_inicial.sql).
type Entrada struct {
	UsuarioID string
	Accion    string
	Entidad   string
	EntidadID *string
	Detalle   map[string]any
	IPOrigen  string // vacío si no se conoce
}

// Auditor persiste una Entrada.
type Auditor interface {
	Registrar(ctx context.Context, e Entrada) error
}
