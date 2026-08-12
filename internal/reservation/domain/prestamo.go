package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// MaxLargoNombreDestinatario coincide con el VARCHAR(200) de la migración
// 013 — el mismo largo que nombre_docente_snapshot. Se valida acá además de
// en la base para que un pegado accidental salga como 400 con explicación y
// no como un 500 de Postgres.
const MaxLargoNombreDestinatario = 200

var (
	ErrNombreDestinatarioVacio = errors.New("hay que anotar a quién se le entrega la computadora")
	ErrNombreDestinatarioLargo = fmt.Errorf("el nombre no puede tener más de %d caracteres", MaxLargoNombreDestinatario)
	ErrRetiradoPorLargo        = fmt.Errorf("el nombre de quien retira no puede tener más de %d caracteres", MaxLargoNombreDestinatario)

	// ErrPrestamoYaDevuelto: se intentó recibir dos veces la misma máquina.
	// Pasa de verdad —dos Admin en el mostrador, o un doble clic— y tiene
	// que distinguirse de "este equipo nunca salió", porque son dos confusiones
	// distintas para quien está atendiendo.
	ErrPrestamoYaDevuelto = errors.New("esa computadora ya figura devuelta")
)

// NormalizarNombreDestinatario recorta los bordes y colapsa los espacios
// internos.
//
// Lo segundo no es cosmético: el nombre se tipea a las apuradas en el
// mostrador, y "Ana  Pérez" con dos espacios es la misma persona que "Ana
// Pérez" pero no matchea en ninguna búsqueda ni se agrupa en ningún listado.
// No se toca la caja: es un nombre propio y se muestra tal como se escribió.
func NormalizarNombreDestinatario(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Prestamo es la custodia física de una PC: quién la tiene ahora.
//
// NO es una reserva, y la diferencia es la razón de ser de todo esto. La
// reserva es el derecho a usar una PC en una franja; el préstamo es dónde
// está la máquina. Los dos existen por separado:
//
//   - reserva sin préstamo: el docente que reservó y no vino a buscarlas;
//   - préstamo sin reserva: "necesito una compu para un trámite";
//   - préstamo que sobrevive a su reserva: la clase terminó a las 9:00 y a
//     las 9:20 las máquinas siguen afuera.
//
// Un préstamo está ABIERTO mientras DevueltoEn sea nil. Que un equipo no
// pueda tener dos abiertos a la vez lo garantiza el índice único parcial de
// la base, no este tipo: el dominio no ve las demás filas.
type Prestamo struct {
	ID       string
	EquipoID string
	// ReservaID nil = préstamo espontáneo. Es un caso normal, no una
	// excepción.
	ReservaID *string

	// EntregadoANombre es quién RESPONDE por el equipo, y va siempre;
	// EntregadoAUsuarioID solo si esa persona tiene cuenta. Ver
	// DatosDeEntrega.
	EntregadoAUsuarioID *string
	EntregadoANombre    string

	// RetiradoPor es quién vino físicamente a buscarlo, cuando no fue quien
	// responde. Vacío es el caso normal y significa que lo retiró esa misma
	// persona — no un dato que falte.
	RetiradoPor string

	Motivo string
	// DevolucionEstimada nil = no se pactó hora. Sin ella no se le puede
	// reclamar nada hasta el cierre de la jornada.
	DevolucionEstimada *time.Time

	EntregadoPor *string
	EntregadoEn  time.Time

	DevueltoEn    *time.Time
	RecibidoPor   *string
	Observaciones string

	// Marcas del barrido (RF-08.12/08.13). Guardan CUÁNDO salió cada aviso,
	// no un booleano: lo primero que se pregunta cuando alguien dice "a mí
	// no me llegó" es a qué hora fue.
	//
	// AvisadoCierrePara es una FECHA y no un instante porque el corte de fin
	// de jornada se repite mientras la máquina siga afuera: lo que hay que
	// recordar es "de este día ya avisé".
	AvisadoDemoraEn   *time.Time
	AvisadoCierrePara *time.Time
}

// DatosDeEntrega son los datos de una entrega. Es un struct y no una lista
// de parámetros porque son ocho y la mitad opcionales: posicionales, nadie
// puede leer la llamada.
type DatosDeEntrega struct {
	EquipoID string
	// ReservaID nil para una entrega espontánea.
	ReservaID *string
	// UsuarioID nil si quien se la lleva no tiene cuenta en el sistema —
	// una preceptora, alguien de secretaría, un alumno. Es el caso normal
	// de un préstamo para un trámite.
	UsuarioID *string
	// Nombre es quien responde por el equipo. Contra una reserva es el
	// docente y no se elige: es quien tiene que devolverlo y a quien se le
	// reclama, sin importar por manos de quién salió del laboratorio.
	Nombre string
	// RetiradoPor es opcional: quién vino a buscarlo, si no fue quien
	// responde. Anotar al alumno le sirve a una institución y a otra le
	// sobra, así que se ofrece y no se exige.
	RetiradoPor string
	Motivo      string
	// DevolucionEstimada: en una entrega contra reserva sale del fin de esa
	// reserva; en una espontánea es opcional a propósito. "Vengo en un
	// rato" es la respuesta honesta, y una hora inventada solo generaría
	// reclamos falsos.
	DevolucionEstimada *time.Time
	EntregadoPor       string
}

// NuevoPrestamo registra que una máquina salió.
//
// No valida que DevolucionEstimada sea futura: una entrega puede anotarse
// después de que la reserva haya terminado —el docente que llegó tarde y se
// las llevó igual— y en ese caso la hora de devolución ya pasó al momento de
// registrarla. Eso es un dato correcto, no un error.
func NuevoPrestamo(id string, d DatosDeEntrega, ahora time.Time) (*Prestamo, error) {
	// Normalizar antes de validar: un nombre de puros espacios pasaría el
	// "no vacío" y chocaría contra el CHECK de la base como un 500.
	nombre := NormalizarNombreDestinatario(d.Nombre)
	if nombre == "" {
		return nil, ErrNombreDestinatarioVacio
	}
	if len([]rune(nombre)) > MaxLargoNombreDestinatario {
		return nil, ErrNombreDestinatarioLargo
	}

	// Mismo tratamiento para quien retira: es un nombre tipeado en el
	// mostrador y comparte columna de largo con el otro. Vacío no es un
	// error — es el caso normal.
	retiradoPor := NormalizarNombreDestinatario(d.RetiradoPor)
	if len([]rune(retiradoPor)) > MaxLargoNombreDestinatario {
		return nil, ErrRetiradoPorLargo
	}

	p := &Prestamo{
		ID:                  id,
		EquipoID:            d.EquipoID,
		ReservaID:           d.ReservaID,
		EntregadoAUsuarioID: d.UsuarioID,
		EntregadoANombre:    nombre,
		RetiradoPor:         retiradoPor,
		Motivo:              strings.TrimSpace(d.Motivo),
		DevolucionEstimada:  d.DevolucionEstimada,
		EntregadoEn:         ahora,
	}
	if d.EntregadoPor != "" {
		p.EntregadoPor = &d.EntregadoPor
	}
	return p, nil
}

// Devolver registra que la máquina volvió.
//
// Las observaciones son el renglón al margen del papel: "volvió sin el
// cargador", "la pantalla tiene una marca". Se guardan tal cual, sin
// interpretarlas — si hay que abrir una incidencia, eso lo decide una
// persona (RF-03.5), no este método.
func (p *Prestamo) Devolver(recibidoPor, observaciones string, ahora time.Time) error {
	if p.DevueltoEn != nil {
		return ErrPrestamoYaDevuelto
	}
	p.DevueltoEn = &ahora
	p.Observaciones = strings.TrimSpace(observaciones)
	if recibidoPor != "" {
		p.RecibidoPor = &recibidoPor
	}
	return nil
}

// EstaAbierto: la máquina todavía está afuera.
func (p *Prestamo) EstaAbierto() bool {
	return p.DevueltoEn == nil
}

// Demorado dice si ya pasó la hora en que debía volver y no volvió.
//
// Un préstamo sin hora pactada NUNCA está demorado: no hay contra qué
// comparar, y tratarlo como vencido convertiría cada "vengo en un rato" en
// un reclamo. Esos aparecen recién en el corte de fin de jornada.
func (p *Prestamo) Demorado(ahora time.Time) bool {
	if !p.EstaAbierto() || p.DevolucionEstimada == nil {
		return false
	}
	return ahora.After(*p.DevolucionEstimada)
}

// MinutosDeDemora es cuánto hace que debería haber vuelto. 0 si no está
// demorado — así quien lo muestra no tiene que preguntar dos cosas.
func (p *Prestamo) MinutosDeDemora(ahora time.Time) int {
	if !p.Demorado(ahora) {
		return 0
	}
	return int(ahora.Sub(*p.DevolucionEstimada).Minutes())
}

// ExcedioLaDemora dice si ya pasó la hora de devolución MÁS el margen que
// la escuela tolera antes de reclamar.
//
// No es lo mismo que Demorado: esa marca la pantalla en rojo apenas se pasa
// la hora, y esto dispara un correo. Diez minutos de diferencia entre las
// dos es a propósito — quien está guardando las cosas no tiene por qué
// recibir un reclamo por llegar un minuto tarde al mostrador.
func (p *Prestamo) ExcedioLaDemora(margen time.Duration, ahora time.Time) bool {
	if !p.EstaAbierto() || p.DevolucionEstimada == nil {
		return false
	}
	return !ahora.Before(p.DevolucionEstimada.Add(margen))
}
