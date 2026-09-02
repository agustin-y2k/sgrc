package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// MaxLargoNombreDestinatario coincide con el VARCHAR(200) de la migración 013
// — el mismo largo que nombre_docente_snapshot.
const MaxLargoNombreDestinatario = 200

var (
	ErrNombreDestinatarioVacio = errors.New("hay que anotar a quién se le entrega la computadora")
	ErrNombreDestinatarioLargo = fmt.Errorf("el nombre no puede tener más de %d caracteres", MaxLargoNombreDestinatario)
	ErrRetiradoPorLargo        = fmt.Errorf("el nombre de quien retira no puede tener más de %d caracteres", MaxLargoNombreDestinatario)

	// ErrPrestamoYaDevuelto: se intentó recibir dos veces la misma máquina.
	ErrPrestamoYaDevuelto = errors.New("esa computadora ya figura devuelta")
)

// NormalizarNombreDestinatario recorta los bordes y colapsa los espacios
// internos.
func NormalizarNombreDestinatario(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Prestamo es la custodia física de una PC: quién la tiene ahora.
type Prestamo struct {
	ID       string
	EquipoID string
	// ReservaID nil = préstamo espontáneo. Es un caso normal, no una
	// excepción.
	ReservaID *string

	// EntregadoANombre es quién RESPONDE por el equipo, y va siempre;
	// EntregadoAUsuarioID solo si esa persona tiene cuenta.
	EntregadoAUsuarioID *string
	EntregadoANombre    string

	// RetiradoPor es quién vino físicamente a buscarlo, cuando no fue quien
	// responde.
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

	// AvisadoCierrePara: marca del corte de fin de jornada (RF-08.13).
	AvisadoCierrePara *time.Time
}

// DatosDeEntrega son los datos de una entrega.
type DatosDeEntrega struct {
	EquipoID string
	// ReservaID nil para una entrega espontánea.
	ReservaID *string
	// UsuarioID nil si quien se la lleva no tiene cuenta en el sistema — una
	// preceptora, alguien de secretaría, un alumno.
	UsuarioID *string
	// Nombre es quien responde por el equipo.
	Nombre string
	// RetiradoPor es opcional: quién vino a buscarlo, si no fue quien responde.
	RetiradoPor string
	Motivo      string
	// DevolucionEstimada: en una entrega contra reserva sale del fin de esa
	// reserva; en una espontánea es opcional a propósito.
	DevolucionEstimada *time.Time
	EntregadoPor       string
}

// NuevoPrestamo registra que una máquina salió.
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

	// Mismo tratamiento para quien retira: es un nombre tipeado en el mostrador
	// y comparte columna de largo con el otro.
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
