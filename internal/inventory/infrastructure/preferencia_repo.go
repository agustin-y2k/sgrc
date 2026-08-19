package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// materia_norm no se escribe nunca desde acá: es una columna generada por la
// base a partir de materia_nombre (ver migrations/003).
const columnasPreferencia = `id, equipo_id, materia_nombre, anio, division, prioridad`

func (r *PostgresRepo) CrearPreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO equipo_preferencia (id, equipo_id, materia_nombre, anio, division, prioridad)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, p.ID, p.EquipoID, p.MateriaNombre, p.Anio, p.Division, p.Prioridad)
	if err != nil {
		if esViolacionUnica(err) {
			return domain.ErrPreferenciaDuplicada
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando preferencia: %w", err)
	}
	return nil
}

// GuardarPreferencia actualiza el alcance y la prioridad. La materia y el
// equipo no se editan: cambiar cualquiera de los dos es otra marca.
func (r *PostgresRepo) GuardarPreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE equipo_preferencia SET anio = $2, division = $3, prioridad = $4 WHERE id = $1
	`, p.ID, p.Anio, p.Division, p.Prioridad)
	if err != nil {
		if esViolacionUnica(err) {
			return domain.ErrPreferenciaDuplicada
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando preferencia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrPreferenciaNoEncontr
	}
	return nil
}

func (r *PostgresRepo) BuscarPreferenciaPorID(ctx context.Context, id string) (*domain.PreferenciaDeEquipo, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+columnasPreferencia+` FROM equipo_preferencia WHERE id = $1`, id)
	return escanearPreferencia(row)
}

func (r *PostgresRepo) BorrarPreferencia(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM equipo_preferencia WHERE id = $1`, id)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("borrando preferencia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrPreferenciaNoEncontr
	}
	return nil
}

// ListarPreferenciasPorEquipo devuelve las marcas de la más específica a la
// más general, que es el orden en que se leen: primero lo puntual ("3°B") y
// después lo que vale para todo.
func (r *PostgresRepo) ListarPreferenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.PreferenciaDeEquipo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+columnasPreferencia+` FROM equipo_preferencia
		WHERE equipo_id = $1
		ORDER BY prioridad, materia_nombre, anio NULLS LAST, division NULLS LAST
	`, equipoID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando preferencias del equipo: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.PreferenciaDeEquipo
	for rows.Next() {
		p, err := escanearPreferencia(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de preferencia: %w", err)
		}
		resultado = append(resultado, p)
	}
	return resultado, errorDeFilas(rows)
}

// NombresDeMateriaEnUso: los nombres distintos que existen, sin importar el
// curso ni el ciclo.
func (r *PostgresRepo) NombresDeMateriaEnUso(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (nombre_norm) nombre
		FROM materia
		ORDER BY nombre_norm, nombre
	`)
	if err != nil {
		return nil, fmt.Errorf("listando nombres de materia: %w", err)
	}
	defer rows.Close()

	var nombres []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("escaneando nombre de materia: %w", err)
		}
		nombres = append(nombres, n)
	}
	return nombres, errorDeFilas(rows)
}

func escanearPreferencia(row pgx.Row) (*domain.PreferenciaDeEquipo, error) {
	var p domain.PreferenciaDeEquipo
	if err := row.Scan(&p.ID, &p.EquipoID, &p.MateriaNombre, &p.Anio, &p.Division, &p.Prioridad); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPreferenciaNoEncontr
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando preferencia: %w", err)
	}
	return &p, nil
}
