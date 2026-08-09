// Package audit persiste el registro de auditoría (ver
// docs/09-seguridad-rbac.md §5): quién hizo qué acción administrativa
// sensible, sobre qué entidad, y cuándo. Es un límite transversal como
// middleware o eventbus — no pertenece al dominio de ningún paquete
// feature en particular, así que auth/academic/inventory/reservation
// dependen de la interfaz Auditor, nunca entre sí.
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
	// PasswordRecuperadaPorEmail es la única acción de este catálogo cuyo
	// actor NO está autenticado: la persona probó ser dueña de la cuenta
	// con el código que le llegó al mail, no con un token. Por eso se
	// audita —quedan la cuenta y la IP desde la que se hizo—, que es lo que
	// permite reconstruir qué pasó si alguien reporta que no fue él.
	PasswordRecuperadaPorEmail = "PASSWORD_RECUPERADA_POR_EMAIL"
	DocenteRemovidoDeMateria   = "DOCENTE_REMOVIDO_DE_MATERIA"
	ReservaCanceladaPorAdmin   = "RESERVA_CANCELADA_POR_ADMIN"
	BloqueoEvaluacionCreado    = "BLOQUEO_EVALUACION_CREADO"
	// Las tres cadenas siguen diciendo PC aunque la entidad pase a llamarse
	// equipo. La auditoría es un registro histórico: estos valores ya
	// están escritos en filas de la base, y reescribirlos para que digan otra
	// cosa es exactamente lo que un log de auditoría no debe permitir. Lo que
	// se renombró es el identificador de Go, que sí es código.
	EquipoEstadoCambiado       = "PC_ESTADO_CAMBIADO"
	EquipoDadoDeBaja           = "PC_DADA_DE_BAJA"
	EquipoMovidoDeCarro        = "PC_MOVIDA_DE_CARRO"
	CursoEliminado             = "CURSO_ELIMINADO"
	MateriaEliminada           = "MATERIA_ELIMINADA"
	CicloArchivadoReservasElim = "CICLO_ARCHIVADO_RESERVAS_ELIMINADAS"
	CicloClonado               = "CICLO_CLONADO"
)

// Entrada es una fila de audit_log (ver migrations/001_esquema_inicial.sql).
// UsuarioID es siempre el actor que ejecutó la acción — nunca la entidad
// afectada (ej: en CUENTA_BAJA, UsuarioID es el Admin que dio de baja, y
// EntidadID es la cuenta dada de baja).
type Entrada struct {
	UsuarioID string
	Accion    string
	Entidad   string
	EntidadID *string
	Detalle   map[string]any
	IPOrigen  string // vacío si no se conoce
}

// Auditor persiste una Entrada. Un fallo de auditoría es una falla de
// infraestructura secundaria, no debe abortar la operación de negocio que
// ya se ejecutó — quien llama decide loguear el error y continuar (ver
// docs/09-seguridad-rbac.md §5).
type Auditor interface {
	Registrar(ctx context.Context, e Entrada) error
}
