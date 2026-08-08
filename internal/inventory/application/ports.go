package application

import (
	"context"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// Repo es el único contrato que este paquete necesita de infrastructure/.
type Repo interface {
	CrearCarro(ctx context.Context, c *domain.Carro) error
	BuscarCarroPorID(ctx context.Context, id string) (*domain.Carro, error)
	GuardarCarro(ctx context.Context, c *domain.Carro) error
	ListarCarros(ctx context.Context) ([]*domain.Carro, error)

	CrearPC(ctx context.Context, pc *domain.PC) error
	BuscarPCPorID(ctx context.Context, id string) (*domain.PC, error)
	GuardarPC(ctx context.Context, pc *domain.PC) error
	ListarPCsPorCarro(ctx context.Context, carroID string) ([]*domain.PC, error)
	// ListarEquiposSueltos: lo prestable que no está en ningún carro
	// (RF-03.15) — el proyector, los cargadores.
	ListarEquiposSueltos(ctx context.Context) ([]*domain.PC, error)

	CrearIncidencia(ctx context.Context, i *domain.Incidencia) error
	BuscarIncidenciaPorID(ctx context.Context, id string) (*domain.Incidencia, error)
	GuardarIncidencia(ctx context.Context, i *domain.Incidencia) error
	ListarIncidenciasPorPC(ctx context.Context, pcID string) ([]*domain.Incidencia, error)

	CrearLicencia(ctx context.Context, l *domain.LicenciaSoftware) error
	BuscarLicenciaPorID(ctx context.Context, id string) (*domain.LicenciaSoftware, error)
	GuardarLicencia(ctx context.Context, l *domain.LicenciaSoftware) error
	BorrarLicencia(ctx context.Context, id string) error
	ListarLicenciasPorPC(ctx context.Context, pcID string) ([]*domain.LicenciaSoftware, error)
	// ListarLicencias devuelve todas las del sistema con su ubicación. Sin
	// paginar a propósito: la cantidad está acotada por el inventario
	// (PCs × un puñado de programas), igual que ciclos, cursos y carros.
	ListarLicencias(ctx context.Context) ([]*LicenciaConUbicacion, error)

	// ListarCandidatasAAviso trae las que PODRÍAN necesitar aviso hoy: con
	// fecha cargada, de una PC que no esté dada de baja, ya dentro de su
	// ventana de antelación, y con alguna marca de aviso sin poner.
	//
	// Es un filtro grueso a propósito. Quién necesita aviso de verdad lo
	// decide el dominio (CorrespondeAvisoPrevio / CorrespondeAvisoDeVencimiento):
	// si esa regla viviera también en el WHERE, habría dos versiones de la
	// misma condición que alguien va a cambiar en un solo lado.
	ListarCandidatasAAviso(ctx context.Context, hoy time.Time) ([]*LicenciaConUbicacion, error)
	// MarcarAvisosEnviados persiste las dos marcas de una licencia. Es un
	// UPDATE acotado a esas columnas y no un GuardarLicencia completo para
	// que el job no pueda pisar una renovación que un Admin haya guardado
	// entre la lectura y el envío del correo.
	MarcarAvisosEnviados(ctx context.Context, l *domain.LicenciaSoftware) error
}

// LicenciaConUbicacion es una licencia más lo mínimo para saber DÓNDE está
// instalada. La pantalla y el correo necesitan las dos cosas siempre —
// "AutoCAD vence mañana" no sirve sin "en la PC 3 del Carro 1"—, así que
// se resuelve con un JOIN en vez de dejar que quien llama busque cada PC.
type LicenciaConUbicacion struct {
	Licencia *domain.LicenciaSoftware
	// Etiqueta es cómo se nombra al equipo: "PC 3" o "Notebook chica". Se
	// muestra esto y no PCIdentificador, que va en 0 —y CarroNombre vacío—
	// cuando el equipo no está en ningún carro (015).
	Etiqueta        string
	PCIdentificador int
	PCDadaDeBaja    bool
	CarroID         string
	CarroNombre     string
}

// ValidadorReservas es el puerto hacia reservation — todavía no existe ese
// paquete, así que se usa un stub que no cancela nada (ver
// infrastructure/validador_reservas_stub.go). Es la respuesta CORRECTA
// hoy — no puede haber ninguna reserva que cancelar si el paquete que las
// crea no está implementado. Mismo criterio que ValidadorReservas en
// academic.
//
// A diferencia del de academic (que solo consulta "¿hay reservas?"), acá
// la interfaz representa una ACCIÓN — cancelar en cascada y notificar —
// porque RF-03.8/03.9 no es una simple validación antes de bloquear una
// operación, es un efecto que debe dispararse cuando una PC cambia de
// estado o se da de baja.
type ValidadorReservas interface {
	CancelarReservasFuturasDePC(ctx context.Context, pcID string, motivo string) (canceladas int, docentesNotificados int, err error)

	// TieneReservasFuturas es la única lectura de este puerto, y existe por
	// la misma razón que TieneReservasDeCiclo en academic: la cascada de
	// RF-03.8/03.9 no puede ser atómica con el guardado de la PC (cruza a
	// reservation, que abre su propia transacción), así que lo que se puede
	// garantizar no es que nunca falle a la mitad, sino que se pueda
	// terminar. Esto es lo que distingue "esta PC ya se dio de baja" de
	// "esta PC se dio de baja pero la cascada quedó pendiente".
	TieneReservasFuturas(ctx context.Context, pcID string) (bool, error)
}

type IDGenerator func() string
