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
	CuentaAprobada             = "CUENTA_APROBADA"
	CuentaRechazada            = "CUENTA_RECHAZADA"
	CuentaBaja                 = "CUENTA_BAJA"
	CuentaEliminadaDefinitiva  = "CUENTA_ELIMINADA_DEFINITIVAMENTE"
	AdminCreado                = "ADMIN_CREADO"
	RolPromovidoAAdmin         = "ROL_PROMOVIDO_A_ADMIN"
	PasswordReseteada          = "PASSWORD_RESETEADA"
	DocenteRemovidoDeMateria   = "DOCENTE_REMOVIDO_DE_MATERIA"
	ReservaCanceladaPorAdmin   = "RESERVA_CANCELADA_POR_ADMIN"
	BloqueoEvaluacionCreado    = "BLOQUEO_EVALUACION_CREADO"
	PCEstadoCambiado           = "PC_ESTADO_CAMBIADO"
	PCDadaDeBaja               = "PC_DADA_DE_BAJA"
	PCMovidaDeCarro            = "PC_MOVIDA_DE_CARRO"
	CursoEliminado             = "CURSO_ELIMINADO"
	MateriaEliminada           = "MATERIA_ELIMINADA"
	CicloArchivadoReservasElim = "CICLO_ARCHIVADO_RESERVAS_ELIMINADAS"
	CicloClonado               = "CICLO_CLONADO"
)

// Entrada es una fila de audit_log (ver migrations/001_init.sql).
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
