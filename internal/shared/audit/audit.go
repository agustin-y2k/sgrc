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
	EquipoEstadoCambiado = "EQUIPO_ESTADO_CAMBIADO"
	EquipoDadoDeBaja     = "EQUIPO_DADO_DE_BAJA"
	EquipoMovidoDeCarro  = "EQUIPO_MOVIDO_DE_CARRO"
	// EquipoEditado cubre TODO lo demás que cambia en una edición. Hasta que
	// existió, lo único que dejaba rastro era el cambio de carro: el nombre, el
	// tipo, el número de serie y si el equipo es reservable cambiaban sin que
	// la auditoría se enterara, que es justo lo que RF-00.2 dice que no puede
	// pasar. El detalle lleva cada campo con su valor anterior y el nuevo,
	// porque la pregunta que contesta —«esta máquina apareció con algo
	// cambiado, ¿quién?»— necesita las dos puntas.
	EquipoEditado = "EQUIPO_EDITADO"
	// CarroDadoDeBaja: retirar un carro libera su nombre para el que lo
	// reemplace, así que sin esta entrada no quedaría rastro de que el «Carro 1»
	// de hoy no es el «Carro 1» del año pasado.
	CarroDadoDeBaja = "CARRO_DADO_DE_BAJA"
	// Las dos vueltas atrás. Se registran por lo mismo que la baja: entre las dos
	// entradas queda dicho por qué un nombre o un número de serie estuvo un
	// tiempo libre y después dejó de estarlo.
	EquipoReactivado = "EQUIPO_REACTIVADO"
	CarroReactivado  = "CARRO_REACTIVADO"

	// Cuentas de usuario de cada equipo (RF-03.22). PasswordDeEquipoRevelada
	// se registra cada vez que alguien MIRA una contraseña, también cuando la
	// cuenta es pública: sirve para reconstruir quién sabía qué el día que una
	// máquina aparece con algo cambiado. Ninguna de las cuatro guarda la
	// contraseña en el detalle — el registro de quién tocó qué no puede ser,
	// él mismo, otra copia de las contraseñas.
	CuentaDeEquipoCreada       = "CUENTA_DE_EQUIPO_CREADA"
	CuentaDeEquipoEditada      = "CUENTA_DE_EQUIPO_EDITADA"
	CuentaDeEquipoBorrada      = "CUENTA_DE_EQUIPO_BORRADA"
	PasswordDeEquipoRevelada   = "PASSWORD_DE_EQUIPO_REVELADA"
	CursoEliminado             = "CURSO_ELIMINADO"
	MateriaEliminada           = "MATERIA_ELIMINADA"
	CicloArchivadoReservasElim = "CICLO_ARCHIVADO_RESERVAS_ELIMINADAS"
	CicloClonado               = "CICLO_CLONADO"
	// Las dos correcciones de un ciclo. La del año se registra con el valor
	// viejo y el nuevo: el año es lo único que identifica a un ciclo en
	// pantalla, así que sin eso una entrada anterior sobre «el ciclo 2026»
	// pasaría a leerse como si hablara de otro.
	CicloAnioCorregido = "CICLO_ANIO_CORREGIDO"
	CicloEliminado     = "CICLO_ELIMINADO"
	// Las dos cargas masivas de cursos y materias (RF-02.12). Se registran
	// aunque no creen nada: lo que contestan es quién cargó la estructura del
	// año y cuándo, que es la pregunta que aparece meses después, cuando un
	// curso tiene una materia que nadie recuerda haber agregado.
	EstructuraImportada = "ESTRUCTURA_IMPORTADA"
	MateriasCopiadas    = "MATERIAS_COPIADAS_ENTRE_CURSOS"
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
