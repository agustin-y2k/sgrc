package domain

import (
	"errors"
	"fmt"
)

// CategoriaEmail es uno de los avisos que el sistema puede mandar por correo
// (RF-05.13). Agrupa por "de qué me avisa el mail" y no por Tipo de
// notificación: es lo que la persona tilda en el panel, y un mismo correo
// puede resumir varios avisos internos.
//
// Que una categoría esté apagada NO apaga el aviso: la campana los muestra
// todos, para todo el mundo, siempre. Acá se elige el canal, no la
// información.
type CategoriaEmail string

// Las de la cuenta: no son copia de ningún aviso interno y NO se pueden
// apagar. Están en el panel para que se vea que existen, tildadas y sin
// casilla que tocar.
const (
	// CatRecuperacionDeCuenta: el código para restablecer la contraseña, o el
	// aviso de que esa cuenta entra con Google y no tiene contraseña propia.
	CatRecuperacionDeCuenta CategoriaEmail = "RECUPERACION_DE_CUENTA"
	// CatCuentaAprobada: un Admin aprobó la cuenta y ya se puede entrar.
	CatCuentaAprobada CategoriaEmail = "CUENTA_APROBADA"
)

// Las personales: le llegan a cualquiera por sus propias reservas y pedidos,
// tenga el rol que tenga.
const (
	// CatSoporteRespondido: le contestaron un pedido de ayuda. FIJA: quien
	// pidió ayuda está esperando, y un aviso que depende de que entre al
	// sistema a mirar no sirve para lo que se pidió.
	CatSoporteRespondido CategoriaEmail = "SOPORTE_RESPONDIDO"
	// CatReservaCancelada: le cancelaron una o varias computadoras de una
	// reserva (RF-05.1/05.2/05.3). Arranca encendida.
	CatReservaCancelada CategoriaEmail = "RESERVA_CANCELADA"
	// CatEquipoNoDisponible: una computadora que reservó puede no estar cuando
	// llegue. Arranca encendida — es lo único que le permite conseguir otra
	// antes de la clase.
	CatEquipoNoDisponible CategoriaEmail = "EQUIPO_NO_DISPONIBLE"
	// CatPedidoDeLiberacion: otro docente le pide un equipo que tiene
	// reservado (RF-04.12).
	CatPedidoDeLiberacion CategoriaEmail = "PEDIDO_DE_LIBERACION"
	// CatPedidoDeMateriaResuelto: le aprobaron o le rechazaron el pedido para
	// dictar una materia.
	CatPedidoDeMateriaResuelto CategoriaEmail = "PEDIDO_DE_MATERIA_RESUELTO"
	// CatPedidoSobreMiMateria: alguien pidió dictar una materia que esta
	// persona ya dicta.
	CatPedidoSobreMiMateria CategoriaEmail = "PEDIDO_SOBRE_MI_MATERIA"
	// CatSugerenciaRespondida: le contestaron lo que escribió en el buzón.
	CatSugerenciaRespondida CategoriaEmail = "SUGERENCIA_RESPONDIDA"
	// CatRecordatorioDeReserva: en un rato tiene clase.
	CatRecordatorioDeReserva CategoriaEmail = "RECORDATORIO_DE_RESERVA"
	// CatReservaSinRetirar: pasaron los minutos y todavía no retiró.
	CatReservaSinRetirar CategoriaEmail = "RESERVA_SIN_RETIRAR"
	// CatDevolucionPendiente: tiene un equipo que ya tenía que haber vuelto.
	CatDevolucionPendiente CategoriaEmail = "DEVOLUCION_PENDIENTE"
)

// Las de administración: los avisos que van a TODOS los Admin.
const (
	// CatSoporte: alguien pidió ayuda por el buzón de soporte. FIJA: es el
	// único canal por el que un docente puede pedir auxilio, y si el aviso
	// se puede apagar deja de serlo.
	CatSoporte CategoriaEmail = "SOPORTE"
	// CatCuentaPendiente: alguien se registró y espera aprobación (RF-05.6).
	CatCuentaPendiente CategoriaEmail = "CUENTA_PENDIENTE"
	// CatLicenciaPorVencer: licencias de software a renovar (RF-05.9).
	CatLicenciaPorVencer CategoriaEmail = "LICENCIA_POR_VENCER"
	// CatSugerencia: alguien escribió en el buzón.
	CatSugerencia CategoriaEmail = "SUGERENCIA"
	// CatPedidoDeMateria: alguien pidió dictar una materia.
	CatPedidoDeMateria CategoriaEmail = "PEDIDO_DE_MATERIA"
	// CatDevolucionDemorada: un equipo entregado no volvió a horario.
	CatDevolucionDemorada CategoriaEmail = "DEVOLUCION_DEMORADA"
	// CatCierreSinDevolver: qué quedó afuera al cerrar la jornada.
	CatCierreSinDevolver CategoriaEmail = "CIERRE_SIN_DEVOLVER"
)

// Grupo separa las tres familias del panel. No es decorativo: decide qué ve
// cada persona y qué puede tocar.
type Grupo string

const (
	// GrupoCuenta son los correos de la cuenta, que no se apagan.
	GrupoCuenta Grupo = "CUENTA"
	// GrupoPersonal es lo que le llega a cualquiera por lo suyo.
	GrupoPersonal Grupo = "PERSONAL"
	// GrupoAdministracion son los avisos que van a todos los Admin.
	GrupoAdministracion Grupo = "ADMINISTRACION"
)

func (c CategoriaEmail) Grupo() Grupo {
	switch c {
	case CatRecuperacionDeCuenta, CatCuentaAprobada:
		return GrupoCuenta
	case CatSoporte, CatCuentaPendiente, CatDevolucionDemorada, CatCierreSinDevolver,
		CatLicenciaPorVencer, CatPedidoDeMateria, CatSugerencia:
		return GrupoAdministracion
	default:
		return GrupoPersonal
	}
}

// EsFija dice si este correo sale siempre y no admite preferencia.
//
// Son cuatro, y por dos razones distintas. Los de la cuenta, porque no son
// copia de nada: el resto de los correos duplica un aviso que igual está en la
// campana, y el del código de recuperación es además el único canal posible
// para alguien que justamente no puede entrar a leerla.
//
// Los dos de soporte, porque del otro lado hay alguien esperando. Un pedido de
// ayuda que espera a que un Admin entre al sistema llega después de la clase
// para la que se pidió, y una respuesta que espera a que el docente entre a
// mirar convierte una conversación en un monólogo. Las sugerencias y los
// "algo no anda" siguen siendo optativos: esos pueden esperar.
func (c CategoriaEmail) EsFija() bool {
	switch c {
	case CatRecuperacionDeCuenta, CatCuentaAprobada, CatSoporte, CatSoporteRespondido:
		return true
	default:
		return false
	}
}

// ActivaPorDefecto dice qué recibe quien nunca abrió el panel.
//
// La regla que separa una lista de la otra: arranca encendido lo que trae
// noticias de algo que hizo OTRO —le pidieron un equipo, le contestaron, le
// resolvieron un pedido, o la computadora que tenía reservada dejó de estar—
// porque quien lo recibe no tiene forma de enterarse a tiempo por su cuenta.
// Arranca apagado lo que le cuenta a alguien algo que ya sabe: que tiene
// clase, que no retiró, que no devolvió.
//
// Y arranca encendido el aviso de cuenta esperando aprobación, aunque sea de
// administración, porque su demora la sufre un tercero: un docente que no
// puede entrar hasta que alguien lo mire.
func (c CategoriaEmail) ActivaPorDefecto() bool {
	if c.EsFija() {
		return true
	}
	switch c {
	case CatReservaCancelada, CatEquipoNoDisponible, CatPedidoDeLiberacion,
		CatPedidoDeMateriaResuelto, CatPedidoSobreMiMateria, CatSugerenciaRespondida,
		CatCuentaPendiente:
		return true
	default:
		return false
	}
}

// CategoriasDeEmail son todas, en el orden en que se muestran: primero las de
// la cuenta, después las personales —que las tiene cualquiera— y al final las
// de administración. Dentro de cada grupo van primero las que vienen
// encendidas, que son las que alguien puede querer apagar.
func CategoriasDeEmail() []CategoriaEmail {
	return []CategoriaEmail{
		// De la cuenta.
		CatRecuperacionDeCuenta,
		CatCuentaAprobada,
		// Personales.
		CatSoporteRespondido,
		CatReservaCancelada,
		CatEquipoNoDisponible,
		CatPedidoDeLiberacion,
		CatPedidoDeMateriaResuelto,
		CatPedidoSobreMiMateria,
		CatSugerenciaRespondida,
		CatRecordatorioDeReserva,
		CatReservaSinRetirar,
		CatDevolucionPendiente,
		// De administración.
		CatSoporte,
		CatCuentaPendiente,
		CatDevolucionDemorada,
		CatCierreSinDevolver,
		CatLicenciaPorVencer,
		CatPedidoDeMateria,
		CatSugerencia,
	}
}

// CategoriasPara son las que se le muestran a esa persona, fijas incluidas.
// Un docente no ve las de administración porque no recibe esos correos.
func CategoriasPara(esAdmin bool) []CategoriaEmail {
	var suyas []CategoriaEmail
	for _, c := range CategoriasDeEmail() {
		if esAdmin || c.Grupo() != GrupoAdministracion {
			suyas = append(suyas, c)
		}
	}
	return suyas
}

// Configurables son las que esa persona puede efectivamente cambiar: las
// suyas menos las fijas. Es la lista sobre la que se guarda una decisión.
func Configurables(esAdmin bool) []CategoriaEmail {
	var suyas []CategoriaEmail
	for _, c := range CategoriasPara(esAdmin) {
		if !c.EsFija() {
			suyas = append(suyas, c)
		}
	}
	return suyas
}

// PuedeElegir dice si esa persona puede pronunciarse sobre esta categoría.
func (c CategoriaEmail) PuedeElegir(esAdmin bool) bool {
	if c.EsFija() {
		return false
	}
	return esAdmin || c.Grupo() != GrupoAdministracion
}

var ErrCategoriaEmailInvalida = errors.New("categoría de correo inválida")

func ParseCategoriaEmail(s string) (CategoriaEmail, error) {
	for _, c := range CategoriasDeEmail() {
		if CategoriaEmail(s) == c {
			return c, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrCategoriaEmailInvalida, s)
}

// EfectivasPara resuelve qué recibe alguien combinando lo que eligió con los
// valores por defecto: una categoría sobre la que no se pronunció no está
// apagada, está sin elegir. Las fijas están siempre.
func EfectivasPara(elegidas map[CategoriaEmail]bool, esAdmin bool) []CategoriaEmail {
	var activas []CategoriaEmail
	for _, c := range CategoriasPara(esAdmin) {
		if c.EsFija() {
			activas = append(activas, c)
			continue
		}
		activa, eligio := elegidas[c]
		if !eligio {
			activa = c.ActivaPorDefecto()
		}
		if activa {
			activas = append(activas, c)
		}
	}
	return activas
}

// Decisiones convierte la selección de casillas en una decisión explícita
// sobre CADA categoría configurable de esa persona: las tildadas en true y las
// demás en false.
//
// Solo las suyas: escribirle a un docente una fila en false por cada categoría
// de administración lo dejaría, el día que lo asciendan a Admin, sin el aviso
// de cuentas pendientes que debería venir encendido. Y ninguna fija, que no
// tienen nada que decidir.
func Decisiones(elegidas []CategoriaEmail, esAdmin bool) map[CategoriaEmail]bool {
	encendidas := make(map[CategoriaEmail]bool, len(elegidas))
	for _, c := range elegidas {
		encendidas[c] = true
	}

	decisiones := map[CategoriaEmail]bool{}
	for _, c := range Configurables(esAdmin) {
		decisiones[c] = encendidas[c]
	}
	return decisiones
}
