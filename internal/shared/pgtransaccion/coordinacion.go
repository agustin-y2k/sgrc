package pgtransaccion

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Un lock por base, compartido por los módulos del monolito. Los cambios de
// inventario/jornada, asignaciones/bajas y la liberación automática son
// exclusivos; la reserva con desplazamientos también es exclusiva porque
// descubre nuevos grupos durante la reposición. Reservar sin tomar equipos
// ocupados y entregar conservan el modo compartido.
// Se toma antes de leer y antes de cualquier bloqueo de filas. Nunca se
// convierte un lock compartido a exclusivo dentro de una operación.
const claveOperacionInstitucional int64 = 0x534752430013

type claveCoordinacion struct{}

func Coordinar(ctx context.Context, pool *pgxpool.Pool, exclusivo bool, fn func(context.Context) error) error {
	if modo, ok := ctx.Value(claveCoordinacion{}).(bool); ok {
		if exclusivo && !modo {
			return fmt.Errorf("no se puede convertir una operación compartida a exclusiva")
		}
		return fn(ctx)
	}
	return Ejecutar(ctx, pool, func(ctxTx context.Context) error {
		consulta := `SELECT pg_advisory_xact_lock_shared($1)`
		if exclusivo {
			consulta = `SELECT pg_advisory_xact_lock($1)`
		}
		if _, err := DeContexto(ctxTx).Exec(ctxTx, consulta, claveOperacionInstitucional); err != nil {
			return fmt.Errorf("coordinando operación institucional: %w", err)
		}
		return fn(context.WithValue(ctxTx, claveCoordinacion{}, exclusivo))
	})
}
