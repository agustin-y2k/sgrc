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

	CrearEquipo(ctx context.Context, pc *domain.Equipo) error
	BuscarEquipoPorID(ctx context.Context, id string) (*domain.Equipo, error)
	GuardarEquipo(ctx context.Context, pc *domain.Equipo) error
	ListarEquiposPorCarro(ctx context.Context, carroID string) ([]*domain.Equipo, error)
	// ListarEquipos: el inventario. Con soloSueltos, únicamente lo prestable
	// que no está en ningún carro (RF-03.15) — el proyector, los cargadores.
	ListarEquipos(ctx context.Context, soloSueltos bool) ([]*domain.Equipo, error)

	// Cuentas de usuario de cada equipo (RF-03.22). Cargarlas es opcional: un
	// equipo sin ninguna es un equipo del que no anotamos nada.
	CrearCuentaDeEquipo(ctx context.Context, c *domain.CuentaDeEquipo) error
	BuscarCuentaDeEquipoPorID(ctx context.Context, id string) (*domain.CuentaDeEquipo, error)
	GuardarCuentaDeEquipo(ctx context.Context, c *domain.CuentaDeEquipo) error
	BorrarCuentaDeEquipo(ctx context.Context, id string) error
	ListarCuentasDeEquipo(ctx context.Context, equipoID string) ([]*domain.CuentaDeEquipo, error)
	// ClasesDeCuentaUsadas alimenta las sugerencias del formulario: la clase es
	// texto libre y esto evita que convivan "Microsoft" y "MICROSOFT".
	ClasesDeCuentaUsadas(ctx context.Context) ([]string, error)

	CrearIncidencia(ctx context.Context, i *domain.Incidencia) error
	BuscarIncidenciaPorID(ctx context.Context, id string) (*domain.Incidencia, error)
	GuardarIncidencia(ctx context.Context, i *domain.Incidencia) error
	ListarIncidenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.Incidencia, error)

	// CategoriasDeFallaUsadas alimenta las sugerencias del formulario.
	CategoriasDeFallaUsadas(ctx context.Context) ([]string, error)

	CrearLicencia(ctx context.Context, l *domain.LicenciaSoftware) error
	BuscarLicenciaPorID(ctx context.Context, id string) (*domain.LicenciaSoftware, error)
	GuardarLicencia(ctx context.Context, l *domain.LicenciaSoftware) error
	BorrarLicencia(ctx context.Context, id string) error
	ListarLicenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.LicenciaSoftware, error)
	// ListarLicencias devuelve todas las del sistema con su ubicación.
	ListarLicencias(ctx context.Context) ([]*LicenciaConUbicacion, error)

	// ListarCandidatasAAviso trae las que PODRÍAN necesitar aviso hoy: con fecha
	// cargada, de una PC que no esté dada de baja, ya dentro de su ventana de
	// antelación, y con alguna marca de aviso sin poner.
	ListarCandidatasAAviso(ctx context.Context, hoy time.Time) ([]*LicenciaConUbicacion, error)
	// MarcarAvisosEnviados persiste las dos marcas de una licencia.
	MarcarAvisosEnviados(ctx context.Context, l *domain.LicenciaSoftware) error

	// Preferencias de materia por equipo (RF-03.21).
	CrearPreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) error
	GuardarPreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) error
	BuscarPreferenciaPorID(ctx context.Context, id string) (*domain.PreferenciaDeEquipo, error)
	BorrarPreferencia(ctx context.Context, id string) error
	ListarPreferenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.PreferenciaDeEquipo, error)

	// NombresDeMateriaEnUso alimenta el selector del inventario: los nombres
	// distintos de materia que existen en el sistema.
	NombresDeMateriaEnUso(ctx context.Context) ([]string, error)
}

// LicenciaConUbicacion es una licencia más lo mínimo para saber DÓNDE está
// instalada.
type LicenciaConUbicacion struct {
	Licencia *domain.LicenciaSoftware
	// Etiqueta es cómo se nombra al equipo: "PC 3" o "Notebook chica".
	Etiqueta         string
	Identificador    int
	EquipoDadoDeBaja bool
	CarroID          string
	CarroNombre      string
}

// ValidadorReservas es el puerto hacia reservation — todavía no existe ese
// paquete, así que se usa un stub que no cancela nada (ver
// infrastructure/validador_reservas_stub.go).
type ValidadorReservas interface {
	CancelarReservasFuturasDeEquipo(ctx context.Context, equipoID string, motivo string) (canceladas int, docentesNotificados int, err error)

	// TieneReservasFuturas es la única lectura de este puerto, y existe por la
	// misma razón que TieneReservasDeCiclo en academic: la cascada de
	// RF-03.8/03.9 no puede ser atómica con el guardado de la PC (cruza a
	// reservation, que abre su propia transacción), así que lo que se puede
	// garantizar no es que nunca falle a la mitad, sino que se pueda terminar.
	TieneReservasFuturas(ctx context.Context, equipoID string) (bool, error)

	// EstaPrestado dice si el equipo está afuera del laboratorio ahora mismo.
	EstaPrestado(ctx context.Context, equipoID string) (bool, error)
}

type IDGenerator func() string
