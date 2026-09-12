// Package infrastructure implementa application.Repo de academic contra
// PostgreSQL real (pgx), además de los adaptadores concretos de
// ValidadorUsuario y ValidadorReservas.
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
)

const codigoViolacionUnica = "23505"

// codigoTextoInvalido: SQLSTATE 22P02 — "invalid input syntax for type X".
const codigoTextoInvalido = "22P02"

var _ application.Repo = (*PostgresRepo)(nil)

type PostgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

func esViolacionUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoViolacionUnica
}

func errorDeFilas(rows pgx.Rows) error {
	err := rows.Err()
	if err == nil {
		return nil
	}
	if esIDInvalido(err) {
		return application.ErrIDInvalido
	}
	return fmt.Errorf("iterando filas: %w", err)
}

func esIDInvalido(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoTextoInvalido
}

// codigoViolacionFK: SQLSTATE 23503 — "foreign_key_violation".
const codigoViolacionFK = "23503"

func esViolacionFK(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoViolacionFK
}

// ── Ciclo lectivo ───────────────────────────────────────────────────────

func (r *PostgresRepo) CrearCiclo(ctx context.Context, c *domain.CicloLectivo) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO ciclo_lectivo (id, anio, activo, archivado) VALUES ($1, $2, $3, $4)`,
		c.ID, c.Anio, c.Activo, c.Archivado)
	if err != nil {
		if esViolacionUnica(err) {
			// La misma constraint UNIQUE cubre dos casos distintos: año duplicado, o
			// (por el índice único parcial) ya hay un ciclo activo.
			return application.ErrCicloYaTieneAnio
		}
		return fmt.Errorf("creando ciclo lectivo: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarCicloActivo(ctx context.Context) (*domain.CicloLectivo, error) {
	row := r.pool.QueryRow(ctx, `SELECT id, anio, activo, archivado FROM ciclo_lectivo WHERE activo = true`)
	return escanearCiclo(row)
}

func (r *PostgresRepo) BuscarCicloPorID(ctx context.Context, id string) (*domain.CicloLectivo, error) {
	row := r.pool.QueryRow(ctx, `SELECT id, anio, activo, archivado FROM ciclo_lectivo WHERE id = $1`, id)
	return escanearCiclo(row)
}

func escanearCiclo(row pgx.Row) (*domain.CicloLectivo, error) {
	var c domain.CicloLectivo
	if err := row.Scan(&c.ID, &c.Anio, &c.Activo, &c.Archivado); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrCicloNoEncontrado
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando ciclo lectivo: %w", err)
	}
	return &c, nil
}

func (r *PostgresRepo) GuardarCiclo(ctx context.Context, c *domain.CicloLectivo) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE ciclo_lectivo SET anio=$2, activo=$3, archivado=$4 WHERE id=$1`,
		c.ID, c.Anio, c.Activo, c.Archivado)
	if err != nil {
		// Desde que el año se puede corregir, este UPDATE puede chocar contra el
		// único de `anio`. Sin este caso salía como 500, que es lo contrario de lo
		// que hay que contestarle a quien tipeó un año que ya existe.
		if esViolacionUnica(err) {
			return application.ErrCicloYaTieneAnio
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando ciclo lectivo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrCicloNoEncontrado
	}
	return nil
}

// EliminarCiclo borra la fila del ciclo. El servicio ya verificó que esté
// vacío; la violación de foránea se traduce igual, porque entre esa
// verificación y este DELETE alguien pudo haber cargado un curso.
func (r *PostgresRepo) EliminarCiclo(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM ciclo_lectivo WHERE id = $1`, id)
	if err != nil {
		if esViolacionFK(err) {
			return application.ErrCicloConCursos
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("eliminando ciclo lectivo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrCicloNoEncontrado
	}
	return nil
}

func (r *PostgresRepo) ListarCiclos(ctx context.Context, filtroArchivado *bool) ([]*domain.CicloLectivo, error) {
	query := `SELECT id, anio, activo, archivado FROM ciclo_lectivo WHERE 1=1`
	args := []any{}
	if filtroArchivado != nil {
		args = append(args, *filtroArchivado)
		query += fmt.Sprintf(" AND archivado = $%d", len(args))
	}
	query += " ORDER BY anio DESC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listando ciclos: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.CicloLectivo
	for rows.Next() {
		c, err := escanearCiclo(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de ciclo: %w", err)
		}
		resultado = append(resultado, c)
	}
	return resultado, errorDeFilas(rows)
}

// ArchivarCiclo marca el ciclo y todos sus cursos/materias como archivados,
// en una sola transacción.
func (r *PostgresRepo) ArchivarCiclo(ctx context.Context, cicloID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciando transacción: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op si ya hizo Commit

	tag, err := tx.Exec(ctx,
		`UPDATE ciclo_lectivo SET activo=false, archivado=true WHERE id=$1 AND archivado=false`,
		cicloID)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("archivando ciclo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrCicloNoEncontrado
	}

	if _, err := tx.Exec(ctx,
		`UPDATE curso SET archivado=true WHERE ciclo_lectivo_id=$1`, cicloID,
	); err != nil {
		return fmt.Errorf("archivando cursos: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE materia SET archivado=true
		WHERE curso_id IN (SELECT id FROM curso WHERE ciclo_lectivo_id=$1)
	`, cicloID); err != nil {
		return fmt.Errorf("archivando materias: %w", err)
	}

	return tx.Commit(ctx)
}

// ClonarCicloA crea el nuevo ciclo lectivo y clona cursos+materias del
// ciclo origen (sin docente_materia, RF-02.5), todo en una transacción.
func (r *PostgresRepo) ClonarCicloA(ctx context.Context, cicloOrigenID string, nuevoCiclo *domain.CicloLectivo) (int, int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("iniciando transacción: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`INSERT INTO ciclo_lectivo (id, anio, activo, archivado) VALUES ($1, $2, $3, $4)`,
		nuevoCiclo.ID, nuevoCiclo.Anio, nuevoCiclo.Activo, nuevoCiclo.Archivado,
	); err != nil {
		if esViolacionUnica(err) {
			return 0, 0, application.ErrCicloYaTieneAnio
		}
		return 0, 0, fmt.Errorf("creando ciclo clonado: %w", err)
	}

	// Los tres datos viajan con el curso. Sin esto la escuela perdería sus
	// divisiones y sus modalidades cada 31 de diciembre, en silencio. `nombre`
	// no se clona: la base lo recalcula de las partes.
	rows, err := tx.Query(ctx,
		`SELECT id, nombre, anio, division, modalidad FROM curso WHERE ciclo_lectivo_id=$1`, cicloOrigenID)
	if err != nil {
		if esIDInvalido(err) {
			return 0, 0, application.ErrIDInvalido
		}
		return 0, 0, fmt.Errorf("leyendo cursos a clonar: %w", err)
	}

	type cursoOrigen struct {
		id, nombre          string
		anio                int
		division, modalidad *string
	}
	var cursos []cursoOrigen
	for rows.Next() {
		var c cursoOrigen
		if err := rows.Scan(&c.id, &c.nombre, &c.anio, &c.division, &c.modalidad); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("escaneando curso a clonar: %w", err)
		}
		cursos = append(cursos, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		if esIDInvalido(err) {
			return 0, 0, application.ErrIDInvalido
		}
		return 0, 0, fmt.Errorf("iterando cursos a clonar: %w", err)
	}

	cursosClonados := 0
	materiasClonadas := 0

	for _, co := range cursos {
		nuevoCursoID := uuidNuevo()
		if _, err := tx.Exec(ctx,
			`INSERT INTO curso (id, ciclo_lectivo_id, anio, division, modalidad, archivado)
			 VALUES ($1, $2, $3, $4, $5, false)`,
			nuevoCursoID, nuevoCiclo.ID, co.anio, co.division, co.modalidad,
		); err != nil {
			return 0, 0, fmt.Errorf("clonando curso %s: %w", co.nombre, err)
		}
		cursosClonados++

		materiaRows, err := tx.Query(ctx, `SELECT nombre FROM materia WHERE curso_id=$1`, co.id)
		if err != nil {
			return 0, 0, fmt.Errorf("leyendo materias a clonar: %w", err)
		}
		var nombresMaterias []string
		for materiaRows.Next() {
			var nombre string
			if err := materiaRows.Scan(&nombre); err != nil {
				materiaRows.Close()
				return 0, 0, fmt.Errorf("escaneando materia a clonar: %w", err)
			}
			nombresMaterias = append(nombresMaterias, nombre)
		}
		materiaRows.Close()
		if err := materiaRows.Err(); err != nil {
			return 0, 0, fmt.Errorf("iterando materias a clonar: %w", err)
		}

		for _, nombreMateria := range nombresMaterias {
			if _, err := tx.Exec(ctx,
				`INSERT INTO materia (id, curso_id, nombre, archivado) VALUES ($1, $2, $3, false)`,
				uuidNuevo(), nuevoCursoID, nombreMateria,
			); err != nil {
				return 0, 0, fmt.Errorf("clonando materia %s: %w", nombreMateria, err)
			}
			materiasClonadas++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("confirmando clonado: %w", err)
	}
	return cursosClonados, materiasClonadas, nil
}

// ListarMateriasReservables: materias de ciclos y cursos no archivados,
// filtradas opcionalmente por el docente asignado (RF-04.1).
// ListarAsignaciones: quién dicta qué en un ciclo, en una sola consulta.
//
// El ciclo es obligatorio a propósito. La relación docente-materia se recrea
// entera cada año (RF-02.5), así que sin acotar por ciclo la respuesta mezcla
// asignaciones de años archivados con las de ahora, y las dos pantallas que la
// usan hablan siempre de un ciclo concreto.
func (r *PostgresRepo) ListarAsignaciones(ctx context.Context, cicloID string) ([]application.AsignacionDocente, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT dm.id, dm.usuario_id, u.nombre || ' ' || u.apellido, dm.rol,
		       m.id, m.nombre, c.id, c.nombre, COALESCE(c.modalidad, '')
		FROM docente_materia dm
		JOIN usuario u ON u.id = dm.usuario_id
		JOIN materia m ON m.id = dm.materia_id
		JOIN curso   c ON c.id = m.curso_id
		WHERE c.ciclo_lectivo_id = $1
		ORDER BY c.modalidad NULLS FIRST, c.anio NULLS LAST, c.nombre, m.nombre, u.apellido, u.nombre`, cicloID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando asignaciones docente-materia: %w", err)
	}
	defer rows.Close()

	var resultado []application.AsignacionDocente
	for rows.Next() {
		var a application.AsignacionDocente
		if err := rows.Scan(&a.ID, &a.UsuarioID, &a.DocenteNombre, &a.Rol,
			&a.MateriaID, &a.MateriaNombre, &a.CursoID, &a.CursoNombre,
			&a.CursoModalidad); err != nil {
			return nil, fmt.Errorf("escaneando asignación docente-materia: %w", err)
		}
		resultado = append(resultado, a)
	}
	return resultado, errorDeFilas(rows)
}

func (r *PostgresRepo) ListarMateriasReservables(ctx context.Context, soloDelDocente *string) ([]application.MateriaReservable, error) {
	query := `
		SELECT m.id, m.nombre, c.id, c.nombre, COALESCE(c.modalidad, ''), cl.id, cl.anio
		FROM materia m
		JOIN curso c ON c.id = m.curso_id
		JOIN ciclo_lectivo cl ON cl.id = c.ciclo_lectivo_id
		WHERE m.archivado = false AND c.archivado = false AND cl.archivado = false`
	args := []any{}

	if soloDelDocente != nil {
		args = append(args, *soloDelDocente)
		query += `
		  AND EXISTS (
			SELECT 1 FROM docente_materia dm
			WHERE dm.materia_id = m.id AND dm.usuario_id = $1
		  )`
	}
	query += ` ORDER BY cl.anio DESC, c.modalidad NULLS FIRST, c.anio NULLS LAST, c.nombre, m.nombre`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando materias reservables: %w", err)
	}
	defer rows.Close()

	var resultado []application.MateriaReservable
	for rows.Next() {
		var m application.MateriaReservable
		if err := rows.Scan(&m.MateriaID, &m.MateriaNombre, &m.CursoID, &m.CursoNombre,
			&m.CursoModalidad, &m.CicloID, &m.CicloAnio); err != nil {
			return nil, fmt.Errorf("escaneando materia reservable: %w", err)
		}
		resultado = append(resultado, m)
	}
	return resultado, errorDeFilas(rows)
}
