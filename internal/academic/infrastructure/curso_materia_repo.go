package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
)

// ── Curso ───────────────────────────────────────────────────────────────

// columnasCurso incluye `nombre`, que es GENERADA por la base (año + grado +
// división): se lee pero nunca se escribe.
//
// `division` y `modalidad` viajan como cadena vacía cuando no hay ninguna y la
// base las guarda como NULL. Los dos significan lo mismo —este curso no tiene
// ese dato— y el NULL es lo que hace que los índices los comparen entre sí por
// las columnas generadas.
const columnasCurso = `id, ciclo_lectivo_id, nombre, anio, division, modalidad, archivado`

// ordenDeCurso: por AÑO primero, y recién después por división y modalidad.
//
// Antes ordenaba por modalidad primero, y con una institución de verdad
// adentro eso es ilegible: agrupar por modalidad deja los años salteados
// —3°, 6°, 3°, 4°, 5°, 1°…— porque cada modalidad recorre los suyos de nuevo.
// El año es el eje por el que una escuela mira sus cursos, y las modalidades
// quedan igual de juntas dentro de cada año.
//
// La división se ordena como NÚMERO cuando lo es. Ordenada como texto, "10"
// cae entre "1" y "2", y una institución con diez divisiones por año no es un
// caso raro. Las divisiones sin dígitos ("A", "1ra" ordena por el 1) caen en
// el NULL y las desempata el texto.
const ordenDeCurso = `anio,
	 nullif(regexp_replace(coalesce(division, ''), '\D', '', 'g'), '')::int NULLS FIRST,
	 division NULLS FIRST,
	 modalidad NULLS FIRST`

func (r *PostgresRepo) CrearCurso(ctx context.Context, c *domain.Curso) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO curso (id, ciclo_lectivo_id, anio, division, modalidad, archivado)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, c.CicloLectivoID, c.Anio, nullSiVacio(c.Division), nullSiVacio(c.Modalidad),
		c.Archivado)
	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrCursoNombreDuplicado
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando curso: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarCursoPorID(ctx context.Context, id string) (*domain.Curso, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+columnasCurso+` FROM curso WHERE id = $1`, id)
	return escanearCurso(row)
}

func escanearCurso(row pgx.Row) (*domain.Curso, error) {
	var c domain.Curso
	var division, modalidad *string
	if err := row.Scan(&c.ID, &c.CicloLectivoID, &c.Nombre, &c.Anio, &division, &modalidad,
		&c.Archivado); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrCursoNoEncontrado
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando curso: %w", err)
	}
	if division != nil {
		c.Division = *division
	}
	if modalidad != nil {
		c.Modalidad = *modalidad
	}
	return &c, nil
}

func (r *PostgresRepo) GuardarCurso(ctx context.Context, c *domain.Curso) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE curso SET anio=$2, division=$3, modalidad=$4, archivado=$5 WHERE id=$1`,
		c.ID, c.Anio, nullSiVacio(c.Division), nullSiVacio(c.Modalidad), c.Archivado)
	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrCursoNombreDuplicado
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando curso: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrCursoNoEncontrado
	}
	return nil
}

// EliminarCurso hace cascade a materia y docente_materia por las FK
// ON DELETE CASCADE ya definidas en la migración (ver docs/07-modelo-datos.md).
//
// La violación de foránea se traduce al MISMO error que devuelve la
// comprobación previa del servicio, y no es redundante: entre esa comprobación
// y este DELETE hay una ventana en la que alguien puede crear una reserva sobre
// una materia del curso. Sin esta traducción, esa carrera —rara pero real—
// devolvía un 500 en vez del 409 que explica qué pasó. La comprobación previa
// sigue existiendo porque da el mejor mensaje y no depende de qué foránea
// saltó.
func (r *PostgresRepo) EliminarCurso(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM curso WHERE id = $1`, id)
	if err != nil {
		if esViolacionFK(err) {
			return application.ErrCursoConReservas
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("eliminando curso: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrCursoNoEncontrado
	}
	return nil
}

func (r *PostgresRepo) ListarCursosPorCiclo(ctx context.Context, cicloID string) ([]*domain.Curso, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+columnasCurso+` FROM curso WHERE ciclo_lectivo_id = $1
		 ORDER BY `+ordenDeCurso,
		cicloID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando cursos: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Curso
	for rows.Next() {
		c, err := escanearCurso(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de curso: %w", err)
		}
		resultado = append(resultado, c)
	}
	return resultado, errorDeFilas(rows)
}

// ── Materia ─────────────────────────────────────────────────────────────

func (r *PostgresRepo) CrearMateria(ctx context.Context, m *domain.Materia) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO materia (id, curso_id, nombre, archivado) VALUES ($1, $2, $3, $4)`,
		m.ID, m.CursoID, m.Nombre, m.Archivado)
	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrMateriaNombreDuplicado
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando materia: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarMateriaPorID(ctx context.Context, id string) (*domain.Materia, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, curso_id, nombre, archivado FROM materia WHERE id = $1`, id)
	return escanearMateria(row)
}

func escanearMateria(row pgx.Row) (*domain.Materia, error) {
	var m domain.Materia
	if err := row.Scan(&m.ID, &m.CursoID, &m.Nombre, &m.Archivado); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrMateriaNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando materia: %w", err)
	}
	return &m, nil
}

func (r *PostgresRepo) GuardarMateria(ctx context.Context, m *domain.Materia) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE materia SET nombre=$2, archivado=$3 WHERE id=$1`,
		m.ID, m.Nombre, m.Archivado)
	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrMateriaNombreDuplicado
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando materia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrMateriaNoEncontrada
	}
	return nil
}

// EliminarMateria hace cascade a docente_materia y a pedido_de_materia por las
// FK ON DELETE CASCADE de la migración.
//
// Las tres que NO son cascade —reserva_grupo, regla_recurrencia y reserva—
// frenan el borrado, y las tres quieren decir lo mismo: la materia tiene
// reservas. Ver EliminarCurso para por qué se traduce acá además de
// comprobarlo antes.
func (r *PostgresRepo) EliminarMateria(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM materia WHERE id = $1`, id)
	if err != nil {
		if esViolacionFK(err) {
			return application.ErrMateriaConReservas
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("eliminando materia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrMateriaNoEncontrada
	}
	return nil
}

func (r *PostgresRepo) ListarMateriasPorCurso(ctx context.Context, cursoID string) ([]*domain.Materia, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, curso_id, nombre, archivado FROM materia WHERE curso_id = $1 ORDER BY nombre`,
		cursoID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando materias: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Materia
	for rows.Next() {
		m, err := escanearMateria(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de materia: %w", err)
		}
		resultado = append(resultado, m)
	}
	return resultado, errorDeFilas(rows)
}

// ── DocenteMateria ──────────────────────────────────────────────────────

func (r *PostgresRepo) AsignarDocente(ctx context.Context, dm *domain.DocenteMateria) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO docente_materia (id, usuario_id, materia_id, rol) VALUES ($1, $2, $3, $4)`,
		dm.ID, dm.UsuarioID, dm.MateriaID, string(dm.Rol))
	if err != nil {
		if esViolacionUnica(err) {
			// UNIQUE(usuario_id, materia_id) — ya está asignado.
			return fmt.Errorf("el docente ya está asignado a esta materia")
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("asignando docente: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarDocenteMateria(ctx context.Context, id string) (*domain.DocenteMateria, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, usuario_id, materia_id, rol FROM docente_materia WHERE id = $1`, id)
	return escanearDocenteMateria(row)
}

func escanearDocenteMateria(row pgx.Row) (*domain.DocenteMateria, error) {
	var dm domain.DocenteMateria
	var rolStr string
	if err := row.Scan(&dm.ID, &dm.UsuarioID, &dm.MateriaID, &rolStr); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrDocenteMateriaNoEncontrado
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando docente_materia: %w", err)
	}
	rol, err := domain.ParseRolDocente(rolStr)
	if err != nil {
		return nil, fmt.Errorf("rol inválido en la base para docente_materia %s: %w", dm.ID, err)
	}
	dm.Rol = rol
	return &dm, nil
}

// GuardarDocenteMateria actualiza solo el rol: el usuario y la materia de un
// vínculo no se editan —cambiar cualquiera de los dos es otro vínculo, y el
// camino para eso es quitar y volver a asignar.
func (r *PostgresRepo) GuardarDocenteMateria(ctx context.Context, dm *domain.DocenteMateria) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE docente_materia SET rol = $2 WHERE id = $1`, dm.ID, string(dm.Rol))
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando docente_materia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrDocenteMateriaNoEncontrado
	}
	return nil
}

func (r *PostgresRepo) RemoverDocenteMateria(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM docente_materia WHERE id = $1`, id)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("removiendo docente_materia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrDocenteMateriaNoEncontrado
	}
	return nil
}

func (r *PostgresRepo) ListarDocentesDeMateria(ctx context.Context, materiaID string) ([]*domain.DocenteMateria, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, usuario_id, materia_id, rol FROM docente_materia WHERE materia_id = $1`,
		materiaID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando docentes de la materia: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.DocenteMateria
	for rows.Next() {
		dm, err := escanearDocenteMateria(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de docente_materia: %w", err)
		}
		resultado = append(resultado, dm)
	}
	return resultado, errorDeFilas(rows)
}

// nullSiVacio guarda NULL en vez de una cadena vacía en las columnas de texto
// opcionales. Para `curso.modalidad` no es cosmético: el índice único compara
// por la columna generada, que hace COALESCE del NULL a cadena vacía, así que
// guardar "" y guardar NULL tienen que terminar en el mismo lugar.
func nullSiVacio(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// CiclosDeCursos: a qué ciclo pertenece cada uno de los ids pedidos, en UNA
// consulta. Los que no existen no aparecen en el mapa.
//
// `= ANY($1)` y no un IN con placeholders armados a mano: el arreglo viaja como
// un solo parámetro, así que la consulta preparada es la misma para uno o para
// doscientos destinos y no hay SQL concatenado.
func (r *PostgresRepo) CiclosDeCursos(ctx context.Context, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, ciclo_lectivo_id FROM curso WHERE id = ANY($1)`, ids)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("leyendo los ciclos de los cursos: %w", err)
	}
	defer rows.Close()

	ciclos := make(map[string]string, len(ids))
	for rows.Next() {
		var id, cicloID string
		if err := rows.Scan(&id, &cicloID); err != nil {
			return nil, fmt.Errorf("escaneando el ciclo de un curso: %w", err)
		}
		ciclos[id] = cicloID
	}
	return ciclos, errorDeFilas(rows)
}
