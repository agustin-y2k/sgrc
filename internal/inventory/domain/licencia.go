package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Topes de LicenciaSoftware. Los tres coinciden con los CHECK de la
// migración 012: se validan acá además de en la base para que un valor
// fuera de rango salga como un 400 con explicación y no como el 500 pelado
// de una violación de constraint.
const (
	MaxLargoNombreLicencia = 100
	// MaxDiasDuracion son diez años. No es un límite del negocio sino un
	// filtro de tipeos: nadie carga una licencia de 30000 días, pero un
	// cero de más en el formulario deja un vencimiento en el año 2108 que
	// nunca va a avisar nada.
	MaxDiasDuracion = 3650
	// MaxDiasAviso es un año. Avisar con más antelación que la duración de
	// la propia licencia es válido —y se permite: puede ser lo correcto si
	// conseguir la renovación lleva tiempo— pero un año ya es todo el
	// horizonte útil.
	MaxDiasAviso = 365
)

var (
	ErrNombreLicenciaVacio   = errors.New("el nombre del software no puede estar vacío")
	ErrNombreLicenciaLargo   = fmt.Errorf("el nombre del software no puede tener más de %d caracteres", MaxLargoNombreLicencia)
	ErrDiasDuracionInvalido  = fmt.Errorf("los días de duración deben ser un entero entre 1 y %d", MaxDiasDuracion)
	ErrDiasAvisoInvalido     = fmt.Errorf("los días de aviso deben ser un entero entre 0 y %d", MaxDiasAviso)
	ErrDiasRestantesInvalido = fmt.Errorf("los días que faltan deben ser un entero entre 0 y %d "+
		"(si la licencia ya venció, cargá la fecha de vencimiento)", MaxDiasDuracion)
	// ErrSinFechaDeVencimiento: se pidió renovar una licencia que todavía
	// no tiene fecha. Renovar es "correr el vencimiento desde una fecha
	// conocida", y sin fecha conocida no hay desde dónde correrlo — lo que
	// corresponde es cargarla, no renovarla.
	ErrSinFechaDeVencimiento = errors.New("la licencia todavía no tiene fecha de vencimiento cargada")
)

// NormalizarNombreLicencia recorta los bordes y nada más.
//
// A diferencia de NumeroSerie y de email, el nombre NO se pasa a mayúsculas
// ni a minúsculas: es un nombre propio que se muestra en la pantalla y en
// los correos, y "AUTOCAD 2027" o "autocad 2027" se leen mal. La unicidad
// sin distinguir mayúsculas la da el índice funcional de la migración 012
// —lower(nombre)— sin tocar lo que se ve.
func NormalizarNombreLicencia(s string) string {
	return strings.TrimSpace(s)
}

// EstadoLicencia es una lectura derivada de la fecha, nunca una columna. Se
// calcula acá y no en el frontend para que la pantalla, el correo y el job
// coincidan en qué cuenta como "por vencer" — que depende del dias_aviso de
// cada licencia y no de un umbral fijo.
type EstadoLicencia string

const (
	// LicenciaSinFecha: cargada pero todavía sin verificar contra la
	// máquina. No es "no vence nunca" y no genera ningún aviso.
	LicenciaSinFecha EstadoLicencia = "SIN_FECHA"
	// LicenciaVencida: el día de vencimiento ya pasó o es hoy.
	LicenciaVencida EstadoLicencia = "VENCIDA"
	// LicenciaPorVencer: entró en su ventana de aviso.
	LicenciaPorVencer EstadoLicencia = "POR_VENCER"
	LicenciaVigente   EstadoLicencia = "VIGENTE"
)

// LicenciaSoftware es una licencia con vencimiento periódico instalada en
// una PC puntual (RF-03.11).
//
// Hay una fila por (PC, software) aunque el mismo AutoCAD esté en las ocho
// PCs de un carro y se renueve todo junto. Un solo registro compartido sería
// menos filas, pero el caso que hay que cubrir es justamente el desfasaje:
// una máquina que quedó sin renovar mientras las demás sí. El alta y la
// renovación son masivas en la interfaz para que eso no cueste ocho clicks.
//
// El contador de días NO se guarda: ver DiasRestantes.
type LicenciaSoftware struct {
	ID           string
	EquipoID     string
	Nombre       string
	DiasDuracion int
	DiasAviso    int

	// FechaVencimiento nil = "a verificar", no "no vence nunca". Ver la
	// migración 012 para por qué existe ese estado.
	FechaVencimiento *time.Time
	// UltimaRenovacion es cuándo se renovó de verdad, que puede no ser
	// cuándo se cargó. Queda nil si el vencimiento se fijó por otro camino
	// (ver VenceEnDias y FijarVencimiento).
	UltimaRenovacion *time.Time

	VencimientoFijadoPor *string
	VencimientoFijadoEn  *time.Time

	AvisadoPrevioPara      *time.Time
	AvisadoVencimientoPara *time.Time

	CreadaEn time.Time
}

// Dia recorta un instante a su fecha, a medianoche UTC.
//
// Todas las fechas de este archivo pasan por acá, incluidas las que vuelven
// de Postgres (pgx entrega un DATE como medianoche UTC). Sin una forma
// única, comparar "hoy" —que sale de la hora de la escuela, con offset
// -03:00— contra un vencimiento leído de la base daría diferencias de
// horas que al dividir por 24 se convierten en un día de más o de menos.
//
// UTC y no la zona de la escuela porque UTC no tiene horario de verano: la
// resta entre dos medianoches siempre da múltiplos exactos de 24 h.
func Dia(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// diasEntre cuenta días calendario de desde a hasta. Exacto porque los dos
// extremos pasan por Dia.
func diasEntre(desde, hasta time.Time) int {
	return int(Dia(hasta).Sub(Dia(desde)).Hours() / 24)
}

// NuevaLicencia crea la licencia SIN fecha de vencimiento. Fijarla es un
// paso aparte —RenovadaEl, VenceEnDias o FijarVencimiento— porque el dato
// llega de tres formas distintas según lo que el Admin tenga a mano, y
// ninguna de las tres es más "la normal" que las otras.
func NuevaLicencia(id, equipoID, nombre string, diasDuracion, diasAviso int, creadaEn time.Time) (*LicenciaSoftware, error) {
	// Normalizar antes de validar, igual que NuevaPC: un nombre de puros
	// espacios pasaría el "no vacío" y chocaría contra el CHECK de la 012
	// como un 500.
	nombre = NormalizarNombreLicencia(nombre)
	if nombre == "" {
		return nil, ErrNombreLicenciaVacio
	}
	if len([]rune(nombre)) > MaxLargoNombreLicencia {
		return nil, ErrNombreLicenciaLargo
	}
	if diasDuracion <= 0 || diasDuracion > MaxDiasDuracion {
		return nil, ErrDiasDuracionInvalido
	}
	if diasAviso < 0 || diasAviso > MaxDiasAviso {
		return nil, ErrDiasAvisoInvalido
	}

	return &LicenciaSoftware{
		ID:           id,
		EquipoID:     equipoID,
		Nombre:       nombre,
		DiasDuracion: diasDuracion,
		DiasAviso:    diasAviso,
		CreadaEn:     creadaEn,
	}, nil
}

// RenombrarA cambia el nombre del software validando lo mismo que el alta.
func (l *LicenciaSoftware) RenombrarA(nombre string) error {
	nombre = NormalizarNombreLicencia(nombre)
	if nombre == "" {
		return ErrNombreLicenciaVacio
	}
	if len([]rune(nombre)) > MaxLargoNombreLicencia {
		return ErrNombreLicenciaLargo
	}
	l.Nombre = nombre
	return nil
}

// CambiarDuracion ajusta el paso de las próximas renovaciones (de 30 a 60
// días, por ejemplo).
//
// NO recalcula el vencimiento vigente a propósito. La licencia que está
// instalada hoy se compró bajo las condiciones viejas y sigue venciendo
// cuando vencía; lo que cambia es cuánto va a durar la próxima. Recalcular
// en silencio movería un vencimiento real por un cambio de configuración —
// la pantalla ofrece hacerlo explícitamente cuando corresponde, llamando a
// RenovadaEl con la última renovación conocida.
func (l *LicenciaSoftware) CambiarDuracion(dias int) error {
	if dias <= 0 || dias > MaxDiasDuracion {
		return ErrDiasDuracionInvalido
	}
	l.DiasDuracion = dias
	return nil
}

func (l *LicenciaSoftware) CambiarDiasAviso(dias int) error {
	if dias < 0 || dias > MaxDiasAviso {
		return ErrDiasAvisoInvalido
	}
	l.DiasAviso = dias
	return nil
}

// ══════════════════════════════════════════════════════════════════
// Las tres formas de fijar el vencimiento
// ══════════════════════════════════════════════════════════════════
//
// Son tres porque el dato llega de tres formas según dónde esté parado el
// Admin, y forzarlo a convertir de una a otra en la cabeza es cómo se
// cargan fechas equivocadas:
//
//   - RenovadaEl:       "la renové el martes"      (lo habitual)
//   - VenceEnDias:      "quedan 12 días"           (lo que muestra la máquina)
//   - FijarVencimiento: "vence el 3 de septiembre" (corrección directa)
//
// Las tres registran quién y cuándo lo escribió en el sistema, y las tres
// dejan las marcas de aviso obsoletas por construcción: al cambiar
// FechaVencimiento dejan de coincidir con ella, así que el ciclo nuevo
// vuelve a avisar sin que haya que resetear nada (ver CorrespondeAviso*).

// RenovadaEl registra una renovación hecha el día indicado: el vencimiento
// pasa a ser esa fecha más los días de duración.
//
// Acepta una fecha pasada porque ese es el caso que hay que cubrir — el
// Admin renovó el martes y se acordó de cargarlo el jueves.
func (l *LicenciaSoftware) RenovadaEl(fechaRenovacion time.Time, porUsuario string, ahora time.Time) {
	renovacion := Dia(fechaRenovacion)
	l.UltimaRenovacion = &renovacion
	l.fijarVencimiento(renovacion.AddDate(0, 0, l.DiasDuracion), porUsuario, ahora)
}

// VenceEnDias fija el vencimiento a "hoy más N días", que es la forma en
// que el dato aparece en la máquina: AutoCAD no dice "vence el 3/9", dice
// "quedan 12 días".
//
// No acepta negativos: para cargar una licencia que ya venció está
// FijarVencimiento con la fecha. Un "-5" acá casi siempre es un tipeo, y el
// resultado sería una licencia que nace vencida sin que nadie lo haya
// querido.
//
// Borra UltimaRenovacion: no se sabe cuándo se renovó, y deducirlo como
// vencimiento - DiasDuracion sería inventar un dato apoyado en otro que
// puede estar mal (el propio DiasDuracion es una estimación del Admin hasta
// que se confirma). Mejor vacío que falso.
func (l *LicenciaSoftware) VenceEnDias(dias int, hoy time.Time, porUsuario string, ahora time.Time) error {
	if dias < 0 || dias > MaxDiasDuracion {
		return ErrDiasRestantesInvalido
	}
	l.UltimaRenovacion = nil
	l.fijarVencimiento(Dia(hoy).AddDate(0, 0, dias), porUsuario, ahora)
	return nil
}

// FijarVencimiento escribe la fecha directamente — la corrección "a mano"
// que puede hacer cualquier Admin en cualquier momento.
//
// Borra UltimaRenovacion por el mismo motivo que VenceEnDias: una vez que
// el vencimiento se movió por fuera de una renovación, la fecha de
// renovación guardada ya no explica el vencimiento que se está mostrando, y
// dejarla ahí haría que la pantalla afirme algo que no se sostiene.
func (l *LicenciaSoftware) FijarVencimiento(fecha time.Time, porUsuario string, ahora time.Time) {
	l.UltimaRenovacion = nil
	l.fijarVencimiento(Dia(fecha), porUsuario, ahora)
}

func (l *LicenciaSoftware) fijarVencimiento(vencimiento time.Time, porUsuario string, ahora time.Time) {
	l.FechaVencimiento = &vencimiento
	l.VencimientoFijadoEn = &ahora
	if porUsuario != "" {
		l.VencimientoFijadoPor = &porUsuario
	}
}

// Renovar es RenovadaEl con una guarda: exige que la licencia ya tenga un
// vencimiento cargado.
//
// La distinción es deliberada. Renovar mueve un contador que ya existe;
// cargar la fecha por primera vez es otra cosa y se hace editando la
// licencia, donde hay que decir CÓMO se sabe (la renové tal día, quedan N
// días, vence tal fecha). Sin la guarda, el botón "Renovar" sería una forma
// de sacarse de encima una licencia sin verificar poniéndole treinta días
// que nadie confirmó — que es exactamente el dato falso que este diseño
// evita.
func (l *LicenciaSoftware) Renovar(fechaRenovacion time.Time, porUsuario string, ahora time.Time) error {
	if l.FechaVencimiento == nil {
		return ErrSinFechaDeVencimiento
	}
	l.RenovadaEl(fechaRenovacion, porUsuario, ahora)
	return nil
}

// ══════════════════════════════════════════════════════════════════
// Lecturas derivadas
// ══════════════════════════════════════════════════════════════════

// DiasRestantes es el contador. Se calcula, no se guarda: no hay ninguna
// columna que decrementar todos los días, así que el sistema no puede
// "perder" un día porque el servidor haya estado apagado.
//
// Devuelve false si la licencia todavía no tiene fecha. Negativo significa
// vencida hace esos días.
func (l *LicenciaSoftware) DiasRestantes(hoy time.Time) (int, bool) {
	if l.FechaVencimiento == nil {
		return 0, false
	}
	return diasEntre(hoy, *l.FechaVencimiento), true
}

// Estado clasifica la licencia para la pantalla y para el texto del aviso.
func (l *LicenciaSoftware) Estado(hoy time.Time) EstadoLicencia {
	dias, tiene := l.DiasRestantes(hoy)
	switch {
	case !tiene:
		return LicenciaSinFecha
	case dias <= 0:
		return LicenciaVencida
	case dias <= l.DiasAviso:
		return LicenciaPorVencer
	default:
		return LicenciaVigente
	}
}

// CorrespondeAvisoPrevio dice si hoy toca mandar el aviso de "está por
// vencer" y todavía no salió para este vencimiento.
//
// La comparación de la marca es contra la FECHA DE VENCIMIENTO, no un
// booleano: es lo que hace idempotente al job sin estado extra. Si el
// contenedor reinicia cinco veces en el día, las cinco veces encuentra la
// marca ya puesta y no manda nada; si el Admin renueva, el vencimiento
// cambia, la marca vieja deja de coincidir y el ciclo siguiente vuelve a
// avisar solo.
func (l *LicenciaSoftware) CorrespondeAvisoPrevio(hoy time.Time) bool {
	dias, tiene := l.DiasRestantes(hoy)
	if !tiene || dias <= 0 || dias > l.DiasAviso {
		return false
	}
	return !mismaFecha(l.AvisadoPrevioPara, l.FechaVencimiento)
}

// CorrespondeAvisoDeVencimiento dice si hoy toca el aviso del día en que
// vence — y también el de una que ya venció sin que nadie la renovara.
//
// Es >= y no ==, igual que la consulta del job: si el proceso estuvo caído
// justo ese día, el aviso sale tarde en vez de no salir nunca. Sale UNA
// vez: al mandarlo, la marca queda apuntando a este vencimiento y no vuelve
// a dispararse hasta que alguien lo mueva. El sistema avisa que venció, no
// insiste todos los días.
func (l *LicenciaSoftware) CorrespondeAvisoDeVencimiento(hoy time.Time) bool {
	dias, tiene := l.DiasRestantes(hoy)
	if !tiene || dias > 0 {
		return false
	}
	return !mismaFecha(l.AvisadoVencimientoPara, l.FechaVencimiento)
}

// MarcarAvisoPrevioEnviado apunta la marca al vencimiento para el que acaba
// de salir el aviso.
func (l *LicenciaSoftware) MarcarAvisoPrevioEnviado() {
	l.AvisadoPrevioPara = l.FechaVencimiento
}

// MarcarAvisoDeVencimientoEnviado cierra las DOS marcas del ciclo.
//
// La previa también, aunque nunca haya salido: una vez que la licencia
// venció, el aviso de "está por vencer" ya no puede corresponder para este
// vencimiento. Si no se cerrara, la fila quedaría para siempre entre las
// candidatas del job —marca previa vacía, vencimiento pasado— haciéndose
// leer en cada barrida para descartarse en el dominio.
func (l *LicenciaSoftware) MarcarAvisoDeVencimientoEnviado() {
	l.AvisadoVencimientoPara = l.FechaVencimiento
	l.AvisadoPrevioPara = l.FechaVencimiento
}

func mismaFecha(a, b *time.Time) bool {
	if a == nil || b == nil {
		return false
	}
	return Dia(*a).Equal(Dia(*b))
}
