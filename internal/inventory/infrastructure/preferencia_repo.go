package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// Las columnas `*_norm` no se escriben nunca desde acá: son generadas por la
// base a partir de materia_nombre, modalidad y curso_nombre.
const columnasPreferencia = `id, equipo_id, materia_nombre, modalidad, anio, division, prioridad`

// CrearPreferencia devuelve `false` —sin error— cuando ese equipo ya tenía esa
// marca. Por el mismo motivo que CrearLicencia: una sentencia fallida aborta la
// transacción entera, y el marcado de un lote de equipos tiene que poder
// saltear los que ya la tienen y seguir.
//
// El ON CONFLICT nombra las cinco columnas del índice `ux_equipo_preferencia`,
// que es NULLS NOT DISTINCT: dos marcas sin año y sin división son la misma
// marca, y así es como se comparan.
func (r *PostgresRepo) CrearPreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO equipo_preferencia (`+columnasPreferencia+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (equipo_id, materia_norm, modalidad_norm, anio, division_norm) DO NOTHING
	`, p.ID, p.EquipoID, p.MateriaNombre, p.Modalidad, p.Anio, p.Division, p.Prioridad)
	if err != nil {
		if esViolacionFK(err) {
			return false, application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("creando preferencia: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// GuardarPreferencia actualiza el alcance y la prioridad. La materia y el
// equipo no se editan: cambiar cualquiera de los dos es otra marca.
func (r *PostgresRepo) GuardarPreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE equipo_preferencia
		   SET modalidad = $2, anio = $3, division = $4, prioridad = $5
		 WHERE id = $1
	`, p.ID, p.Modalidad, p.Anio, p.Division, p.Prioridad)
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
	row := r.db.QueryRow(ctx, `SELECT `+columnasPreferencia+` FROM equipo_preferencia WHERE id = $1`, id)
	return escanearPreferencia(row)
}

func (r *PostgresRepo) BorrarPreferencia(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM equipo_preferencia WHERE id = $1`, id)
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
	rows, err := r.db.Query(ctx, `
		SELECT `+columnasPreferencia+` FROM equipo_preferencia
		WHERE equipo_id = $1
		ORDER BY prioridad, materia_nombre, modalidad NULLS LAST, anio NULLS LAST, division NULLS LAST
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
	rows, err := r.db.Query(ctx, `
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
	if err := row.Scan(&p.ID, &p.EquipoID, &p.MateriaNombre, &p.Modalidad, &p.Anio,
		&p.Division, &p.Prioridad); err != nil {
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

// ListarPreferenciasHuerfanas: las marcas que ya no cruzan con ninguna materia
// cargada.
//
// Existen porque el vínculo entre una marca y su materia es **por nombre y no
// por referencia** (RF-03.21), y eso es deliberado: una referencia dejaría el
// inventario sin marcas cada 31 de diciembre, cuando el ciclo se clona y cada
// materia nace con otro identificador.
//
// Lo que ese diseño no resuelve es qué pasa al RENOMBRAR o borrar una materia:
// la marca deja de aplicar en silencio y sigue viéndose en la ficha del equipo
// como si valiera. Esta consulta es la única forma de encontrarlas.
//
// El cruce usa `clave_texto` —la misma función que sostiene la unicidad del
// sistema (RF-00.1)— para no marcar como huérfana una que sólo difiere en una
// tilde.
func (r *PostgresRepo) ListarPreferenciasHuerfanas(ctx context.Context) ([]*application.PreferenciaHuerfana, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ep.id, ep.materia_nombre, ep.modalidad, ep.anio, ep.division, ep.prioridad,
		       COALESCE(e.nombre, 'PC ' || e.identificador), COALESCE(c.nombre, ''),
		       e.dado_de_baja
		  FROM equipo_preferencia ep
		  JOIN equipo e ON e.id = ep.equipo_id
		  LEFT JOIN carro c ON c.id = e.carro_id
		 WHERE NOT EXISTS (
		       SELECT 1 FROM materia m
		        WHERE clave_texto(m.nombre) = clave_texto(ep.materia_nombre))
		 ORDER BY ep.materia_nombre, 7`)
	if err != nil {
		return nil, fmt.Errorf("listando preferencias huérfanas: %w", err)
	}
	defer rows.Close()

	// Vacío y no nil: quien lo consume recorre sin preguntar.
	huerfanas := []*application.PreferenciaHuerfana{}
	for rows.Next() {
		var h application.PreferenciaHuerfana
		var modalidad, division *string
		var anio *int
		if err := rows.Scan(&h.ID, &h.MateriaNombre, &modalidad, &anio, &division,
			&h.Prioridad, &h.EquipoEtiqueta, &h.CarroNombre, &h.EquipoDadoDeBaja); err != nil {
			return nil, fmt.Errorf("escaneando preferencia huérfana: %w", err)
		}
		if modalidad != nil {
			h.Modalidad = *modalidad
		}
		if division != nil {
			h.Division = *division
		}
		h.Anio = anio
		huerfanas = append(huerfanas, &h)
	}
	return huerfanas, rows.Err()
}

// ContarPreferenciasQueDejarianDeAplicar: cuántas marcas apuntan a ese nombre
// de materia y dejarían de cruzar si se renombrara.
//
// Es la pregunta que hay que poder contestar ANTES de renombrar, para avisar en
// vez de romper en silencio.
func (r *PostgresRepo) ContarPreferenciasQueDejarianDeAplicar(ctx context.Context, materiaNombre string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM equipo_preferencia
		 WHERE clave_texto(materia_nombre) = clave_texto($1)`, materiaNombre).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contando las marcas que dejarían de aplicar: %w", err)
	}
	return n, nil
}
