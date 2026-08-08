package infrastructure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/academic/application"
)

var _ application.ValidadorReservas = (*ValidadorReservasPostgres)(nil)

// ValidadorReservasPostgres reemplaza al viejo ValidadorReservasStub ahora
// que internal/reservation existe. Consulta reserva_grupo/materia
// directamente — a propósito NO importa internal/reservation (mismo
// criterio que ValidadorUsuarioPostgres hacia auth): es una lectura simple
// de existencia, sin lógica de negocio que valga la pena centralizar en
// otro paquete.
//
// Cuenta CUALQUIER reserva_grupo existente, sin filtrar por estado — la
// misma razón por la que la propia FK (reserva_grupo.materia_id) bloquea
// el borrado sin importar si esas reservas ya están canceladas o
// finalizadas: son historial, y RF-02.11 pide dar un error de negocio
// claro (ErrMateriaConReservas/ErrCursoConReservas) antes de que el
// intento de DELETE choque contra esa constraint.
type ValidadorReservasPostgres struct {
	pool *pgxpool.Pool
}

func NewValidadorReservasPostgres(pool *pgxpool.Pool) *ValidadorReservasPostgres {
	return &ValidadorReservasPostgres{pool: pool}
}

func (v *ValidadorReservasPostgres) TieneReservasCurso(ctx context.Context, cursoID string) (bool, error) {
	var existe bool
	err := v.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM reserva_grupo rg
			JOIN materia m ON m.id = rg.materia_id
			WHERE m.curso_id = $1
		)
	`, cursoID).Scan(&existe)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando reservas del curso: %w", err)
	}
	return existe, nil
}

// TieneReservasDeCiclo cubre las TRES cosas que el archivado borra y que
// pueden quedar colgadas si falla a mitad de camino: los reserva_grupo de
// las materias del ciclo, las regla_recurrencia de esas materias, y los
// bloqueos por evaluación estatal, que no tienen materia y se atan al ciclo
// por el año de su fecha (mismo criterio que EliminarReservasYGruposDeCiclo
// en reservation/infrastructure).
//
// Las reglas faltaban acá y el borrado sí las tocaba. Con eso, un fallo justo
// entre borrar los grupos y borrar las reglas dejaba filas que el reintento
// ya no veía —decía que no faltaba nada— y quedaban para siempre. No hacían
// daño, porque ningún job las lee, pero el archivado prometía completar la
// limpieza y no la completaba.
func (v *ValidadorReservasPostgres) TieneReservasDeCiclo(ctx context.Context, cicloID string) (bool, error) {
	var existe bool
	err := v.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM reserva_grupo rg
			JOIN materia m ON m.id = rg.materia_id
			JOIN curso c ON c.id = m.curso_id
			WHERE c.ciclo_lectivo_id = $1
		) OR EXISTS(
			SELECT 1 FROM regla_recurrencia rr
			JOIN materia m ON m.id = rr.materia_id
			JOIN curso c ON c.id = m.curso_id
			WHERE c.ciclo_lectivo_id = $1
		) OR EXISTS(
			SELECT 1 FROM reserva
			WHERE tipo = 'EVALUACION_ESTATAL'
			  AND EXTRACT(YEAR FROM fecha) = (SELECT anio FROM ciclo_lectivo WHERE id = $1)
		)
	`, cicloID).Scan(&existe)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando reservas del ciclo: %w", err)
	}
	return existe, nil
}

func (v *ValidadorReservasPostgres) TieneReservasMateria(ctx context.Context, materiaID string) (bool, error) {
	var existe bool
	err := v.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM reserva_grupo WHERE materia_id = $1)`, materiaID,
	).Scan(&existe)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando reservas de la materia: %w", err)
	}
	return existe, nil
}
