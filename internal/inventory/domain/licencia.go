package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Topes de LicenciaSoftware.
const (
	MaxLargoNombreLicencia = 100
	// MaxDiasDuracion son diez años.
	MaxDiasDuracion = 3650
	// MaxDiasAviso es un año.
	MaxDiasAviso = 365
)

var (
	ErrNombreLicenciaVacio   = errors.New("el nombre del software no puede estar vacío")
	ErrNombreLicenciaLargo   = fmt.Errorf("el nombre del software no puede tener más de %d caracteres", MaxLargoNombreLicencia)
	ErrDiasDuracionInvalido  = fmt.Errorf("los días de duración deben ser un entero entre 1 y %d", MaxDiasDuracion)
	ErrDiasAvisoInvalido     = fmt.Errorf("los días de aviso deben ser un entero entre 0 y %d", MaxDiasAviso)
	ErrDiasRestantesInvalido = fmt.Errorf("los días que faltan deben ser un entero entre 0 y %d "+
		"(si la licencia ya venció, cargá la fecha de vencimiento)", MaxDiasDuracion)
	// ErrSinFechaDeVencimiento: se pidió renovar una licencia que todavía no
	// tiene fecha.
	ErrSinFechaDeVencimiento = errors.New("la licencia todavía no tiene fecha de vencimiento cargada")
)

// NormalizarNombreLicencia recorta los bordes y nada más.
func NormalizarNombreLicencia(s string) string {
	return strings.TrimSpace(s)
}

// EstadoLicencia es una lectura derivada de la fecha, nunca una columna.
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

// LicenciaSoftware es una licencia con vencimiento periódico instalada en una
// PC puntual (RF-03.11).
type LicenciaSoftware struct {
	ID           string
	EquipoID     string
	Nombre       string
	DiasDuracion int
	DiasAviso    int

	// FechaVencimiento nil = "a verificar", no "no vence nunca": es el estado
	// real de una licencia cargada antes de poder mirar la máquina (RF-03.13).
	FechaVencimiento *time.Time
	// UltimaRenovacion es cuándo se renovó de verdad, que puede no ser cuándo se
	// cargó.
	UltimaRenovacion *time.Time

	VencimientoFijadoPor *string
	VencimientoFijadoEn  *time.Time

	AvisadoPrevioPara      *time.Time
	AvisadoVencimientoPara *time.Time

	CreadaEn time.Time
}

// Dia recorta un instante a su fecha, a medianoche UTC. Todas las fechas de
// este archivo pasan por acá, incluidas las que vuelven de Postgres (pgx
// entrega un DATE como medianoche UTC).
func Dia(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// diasEntre cuenta días calendario de desde a hasta. Exacto porque los dos
// extremos pasan por Dia.
func diasEntre(desde, hasta time.Time) int {
	return int(Dia(hasta).Sub(Dia(desde)).Hours() / 24)
}

// NuevaLicencia crea la licencia SIN fecha de vencimiento.
func NuevaLicencia(id, equipoID, nombre string, diasDuracion, diasAviso int, creadaEn time.Time) (*LicenciaSoftware, error) {
	// Normalizar antes de validar, igual que NuevoEquipoDeCarro: un nombre de
	// puros espacios pasaría el "no vacío" y chocaría contra el CHECK de la base
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

// ══════════════════════════════════════════════════════════════════ Las tres
// formas de fijar el vencimiento
// ══════════════════════════════════════════════════════════════════ Son tres
// porque el dato llega de tres formas según dónde esté parado el Admin, y
// forzarlo a convertir de una a otra en la cabeza es cómo se cargan fechas
// equivocadas: - RenovadaEl: "la renové el martes" (lo habitual) -
// VenceEnDias: "quedan 12 días" (lo que muestra la máquina) -
// FijarVencimiento: "vence el 3 de septiembre" (corrección directa) Las tres
// registran quién y cuándo lo escribió en el sistema, y las tres dejan las
// marcas de aviso obsoletas por construcción: al cambiar FechaVencimiento
// dejan de coincidir con ella, así que el ciclo nuevo vuelve a avisar sin que
// haya que resetear nada (ver CorrespondeAviso*).

// RenovadaEl registra una renovación hecha el día indicado: el vencimiento
// pasa a ser esa fecha más los días de duración.
func (l *LicenciaSoftware) RenovadaEl(fechaRenovacion time.Time, porUsuario string, ahora time.Time) {
	renovacion := Dia(fechaRenovacion)
	l.UltimaRenovacion = &renovacion
	l.fijarVencimiento(renovacion.AddDate(0, 0, l.DiasDuracion), porUsuario, ahora)
}

// VenceEnDias fija el vencimiento a "hoy más N días", que es la forma en que
// el dato aparece en la máquina: AutoCAD no dice "vence el 3/9", dice "quedan
// 12 días".
func (l *LicenciaSoftware) VenceEnDias(dias int, hoy time.Time, porUsuario string, ahora time.Time) error {
	if dias < 0 || dias > MaxDiasDuracion {
		return ErrDiasRestantesInvalido
	}
	l.UltimaRenovacion = nil
	l.fijarVencimiento(Dia(hoy).AddDate(0, 0, dias), porUsuario, ahora)
	return nil
}

// FijarVencimiento escribe la fecha directamente — la corrección "a mano" que
// puede hacer cualquier Admin en cualquier momento.
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
func (l *LicenciaSoftware) Renovar(fechaRenovacion time.Time, porUsuario string, ahora time.Time) error {
	if l.FechaVencimiento == nil {
		return ErrSinFechaDeVencimiento
	}
	l.RenovadaEl(fechaRenovacion, porUsuario, ahora)
	return nil
}

// ══════════════════════════════════════════════════════════════════ Lecturas
// derivadas
// ══════════════════════════════════════════════════════════════════

// DiasRestantes es el contador.
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
func (l *LicenciaSoftware) CorrespondeAvisoPrevio(hoy time.Time) bool {
	dias, tiene := l.DiasRestantes(hoy)
	if !tiene || dias <= 0 || dias > l.DiasAviso {
		return false
	}
	return !mismaFecha(l.AvisadoPrevioPara, l.FechaVencimiento)
}

// CorrespondeAvisoDeVencimiento dice si hoy toca el aviso del día en que
// vence — y también el de una que ya venció sin que nadie la renovara.
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
