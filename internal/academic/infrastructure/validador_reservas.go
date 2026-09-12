package infrastructure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/academic/application"
)

var _ application.ValidadorReservas = (*ValidadorReservasPostgres)(nil)

// ValidadorReservasPostgres reemplaza al viejo ValidadorReservasStub ahora
// que internal/reservation existe.
type ValidadorReservasPostgres struct {
	pool *pgxpool.Pool
}

func NewValidadorReservasPostgres(pool *pgxpool.Pool) *ValidadorReservasPostgres {
	return &ValidadorReservasPostgres{pool: pool}
}

func (v *ValidadorReservasPostgres) TieneReservasCurso(ctx context.Context, cursoID string) (bool, error) {
	// Las dos tablas, igual que TieneReservasMateria: borrar un curso arrastra
	// sus materias en cascada, así que cualquier cosa que bloquee el borrado de
	// una materia bloquea el del curso entero. Mirar sólo `reserva_grupo`
	// dejaría que el DELETE reviente contra `regla_recurrencia` con un 500.
	var existe bool
	err := v.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM reserva_grupo rg
			JOIN materia m ON m.id = rg.materia_id
			WHERE m.curso_id = $1
		) OR EXISTS(
			SELECT 1 FROM regla_recurrencia rr
			JOIN materia m ON m.id = rr.materia_id
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
// pueden quedar colgadas si falla a mitad de camino: los reserva_grupo de las
// materias del ciclo, las regla_recurrencia de esas materias, y los bloqueos
// administrativos, que no tienen materia y se atan al ciclo por el año de su
// fecha (mismo criterio que EliminarReservasYGruposDeCiclo en
// reservation/infrastructure).
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
			WHERE tipo = 'BLOQUEO'
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

// HayBloqueosEnElAnio: la misma condición con la que EliminarReservasDeCiclo
// decide qué bloqueos son de un ciclo, pero preguntada por año suelto — porque
// el año que se quiere estrenar todavía no es de ningún ciclo.
func (v *ValidadorReservasPostgres) HayBloqueosEnElAnio(ctx context.Context, anio int) (bool, error) {
	var existe bool
	err := v.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM reserva
			WHERE tipo = 'BLOQUEO' AND EXTRACT(YEAR FROM fecha) = $1
		)`, anio).Scan(&existe)
	if err != nil {
		return false, fmt.Errorf("verificando bloqueos del año %d: %w", anio, err)
	}
	return existe, nil
}

func (v *ValidadorReservasPostgres) TieneReservasMateria(ctx context.Context, materiaID string) (bool, error) {
	// Las DOS tablas que apuntan a la materia con ON DELETE NO ACTION, no sólo
	// `reserva_grupo`. Mirar una sola dejaba una baranda a medias: una materia
	// con una regla de recurrencia y sin grupos pasaba el chequeo y la base la
	// rechazaba con un error crudo, o sea un 500 en vez del 409 que explica qué
	// pasó.
	//
	// Hoy no se puede llegar a ese estado —la recurrencia siempre crea sus
	// grupos, y el archivado borra los dos en la misma transacción—, pero el
	// chequeo previo existe justamente para que la respuesta no dependa de que
	// eso siga siendo cierto.
	var existe bool
	err := v.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM reserva_grupo     WHERE materia_id = $1)
		    OR EXISTS(SELECT 1 FROM regla_recurrencia WHERE materia_id = $1)`, materiaID,
	).Scan(&existe)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando reservas de la materia: %w", err)
	}
	return existe, nil
}
