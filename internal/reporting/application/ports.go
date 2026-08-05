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
	GuardarHistoricoUsoPC(ctx context.Context, h *domain.HistoricoUsoPC) error
	GuardarHistoricoUsoDocente(ctx context.Context, h *domain.HistoricoUsoDocente) error
	// Los históricos se listan por año (no por ciclo — ver el comentario
	// en domain/historico.go sobre por qué esa tabla usa `anio`).
	ListarHistoricoUsoPCPorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoPC, error)
	ListarHistoricoUsoDocentePorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoDocente, error)

	// CalcularUsoPCsDeCiclo acepta un rango de fechas opcional (RF-06.1:
	// "filtrable por rango de fechas"). nil en cualquiera de los dos
	// extremos significa "sin ese límite".
	CalcularUsoPCsDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoPC, error)
	CalcularUsoDocentesDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoDocente, error)

	// RF-06.3: incidencias por equipo y por carro. No dependen del ciclo
	// lectivo (Incidencia sobrevive al archivado, ver RF-02.4), así que se
	// resuelven siempre con una query directa, sin snapshot.
	CalcularIncidenciasPorPC(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasPC, error)
	CalcularIncidenciasPorCarro(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasCarro, error)
}

// InfoPCParaSnapshot es el puerto hacia inventory — necesario para
// "congelar" el identificador y el nombre del carro de una PC al momento
// de archivar (IdentificadorSnapshot/CarroNombreSnapshot). Nunca se
// importa internal/inventory directamente.
type InfoPCParaSnapshot interface {
	IdentificadorYCarroDe(ctx context.Context, pcID string) (identificador int, carroNombre string, err error)
}

// InfoUsuarioParaSnapshot es el puerto hacia auth — para
// NombreDocenteSnapshot. Nunca se importa internal/auth directamente.
type InfoUsuarioParaSnapshot interface {
	NombreCompletoDe(ctx context.Context, usuarioID string) (string, error)
}

type IDGenerator func() string
