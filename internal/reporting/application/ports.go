package application

import (
	"context"
	"time"

	"github.com/ramiro/sgrc/internal/reporting/domain"
)

// Repo es el único contrato que este paquete necesita de infrastructure/.
// Las agregaciones "en vivo" (Calcular...) consultan reserva/materia/curso
// directamente por SQL — sin importar internal/reservation ni
// internal/academic, mismo criterio que cualquier otro validador de solo
// lectura del proyecto.
type Repo interface {
	GuardarHistoricoUsoEquipo(ctx context.Context, h *domain.HistoricoUsoEquipo) error
	GuardarHistoricoUsoDocente(ctx context.Context, h *domain.HistoricoUsoDocente) error
	// Los históricos se listan por año (no por ciclo — ver el comentario
	// en domain/historico.go sobre por qué esa tabla usa `anio`).
	ListarHistoricoUsoEquipoPorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoEquipo, error)
	ListarHistoricoUsoDocentePorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoDocente, error)

	// CalcularUsoEquiposDeCiclo acepta un rango de fechas opcional (RF-06.1:
	// "filtrable por rango de fechas"). nil en cualquiera de los dos
	// extremos significa "sin ese límite".
	CalcularUsoEquiposDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoEquipo, error)
	CalcularUsoDocentesDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoDocente, error)

	// RF-06.3: incidencias por equipo y por carro. No dependen del ciclo
	// lectivo (Incidencia sobrevive al archivado, ver RF-02.4), así que se
	// resuelven siempre con una query directa, sin snapshot.
	CalcularIncidenciasPorEquipo(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasEquipo, error)
	CalcularIncidenciasPorCarro(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasCarro, error)

	// Los tres de RF-06.5 no dependen del ciclo lectivo: son una foto del
	// parque HOY. Una máquina rota lo está ahora, sin importar en qué año se
	// reportó la falla.
	EstadoDelInventario(ctx context.Context) ([]domain.EstadoDelInventario, error)
	EquiposFueraDeCirculacion(ctx context.Context) ([]domain.EquipoFueraDeCirculacion, error)
	CalcularIncidenciasPorCategoria(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenPorCategoriaDeFalla, error)
}

// InfoEquipoParaSnapshot es el puerto hacia inventory — necesario para
// "congelar" cómo se llamaba el equipo y dónde estaba al momento de archivar
// (EtiquetaSnapshot/IdentificadorSnapshot/CarroNombreSnapshot). Nunca se
// importa internal/inventory directamente.
type InfoEquipoParaSnapshot interface {
	// Devuelve la etiqueta siempre; identificador en 0 y carro vacío si el
	// equipo no está en ningún carro (015).
	EtiquetaYCarroDe(ctx context.Context, equipoID string) (etiqueta string, identificador int, carroNombre string, err error)
}

// InfoUsuarioParaSnapshot es el puerto hacia auth — para
// NombreDocenteSnapshot. Nunca se importa internal/auth directamente.
type InfoUsuarioParaSnapshot interface {
	NombreCompletoDe(ctx context.Context, usuarioID string) (string, error)
}

type IDGenerator func() string
