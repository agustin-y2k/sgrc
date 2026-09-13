package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
)

// Espacios: los lugares de la institución que no son cursos (RF-02.13).
//
// Casi todo es igual que un curso, con una diferencia que se nota en cada
// consulta: un espacio NO tiene nombre generado. Su nombre es el dato.

const columnasEspacio = `id, ciclo_lectivo_id, nombre, archivado`

func escanearEspacio(fila pgx.Row) (*domain.Espacio, error) {
	var e domain.Espacio
	if err := fila.Scan(&e.ID, &e.CicloLectivoID, &e.Nombre, &e.Archivado); err != nil {
		return nil, err
	}
	return &e, nil
}

// CrearEspacio crea el lugar Y la materia con la que se reserva para él, en una
// sola transacción.
//
// **Por qué hay una materia que nadie ve.** Dirección, Preceptoría y la
// Biblioteca no dictan materias — el Admin no debería tener que inventar una
// para poder reservar. Pero toda reserva del sistema apunta a una materia:
// `reserva.materia_id` y `regla_recurrencia.materia_id` son NOT NULL, y de ahí
// cuelgan los reportes, las validaciones y el orden por preferencia. Cambiar
// eso sería rehacer el núcleo de reservas para un caso que se resuelve con una
// fila.
//
// Así que el espacio trae su propia materia, con SU MISMO nombre, y la interfaz
// no la menciona en ningún lado: para quien usa el sistema, se reserva "para la
// Biblioteca" y listo. Renombrar el espacio renombra las dos (ver
// GuardarEspacio), así que no pueden desincronizarse.
func (r *PostgresRepo) CrearEspacio(ctx context.Context, e *domain.Espacio) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abriendo transacción para crear el espacio: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`INSERT INTO espacio (id, ciclo_lectivo_id, nombre, archivado) VALUES ($1, $2, $3, $4)`,
		e.ID, e.CicloLectivoID, e.Nombre, e.Archivado,
	); err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		// Misma regla que el resto: dos nombres son el mismo sin tildes, sin
		// mayúsculas y sin espacios de más (migración 011).
		if esViolacionUnica(err) {
			return application.ErrNombreEspacioDuplicado
		}
		return fmt.Errorf("creando espacio: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO materia (id, espacio_id, nombre, archivado) VALUES ($1, $2, $3, false)`,
		uuidNuevo(), e.ID, e.Nombre,
	); err != nil {
		return fmt.Errorf("creando la materia del espacio: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmando el alta del espacio: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarEspacioPorID(ctx context.Context, id string) (*domain.Espacio, error) {
	e, err := escanearEspacio(r.pool.QueryRow(ctx,
		`SELECT `+columnasEspacio+` FROM espacio WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrEspacioNoEncontrado
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("buscando espacio: %w", err)
	}
	return e, nil
}

func (r *PostgresRepo) ListarEspaciosPorCiclo(ctx context.Context, cicloID string) ([]*domain.Espacio, error) {
	filas, err := r.pool.Query(ctx,
		`SELECT `+columnasEspacio+` FROM espacio WHERE ciclo_lectivo_id = $1
		 ORDER BY clave_texto(nombre)`, cicloID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando espacios: %w", err)
	}
	defer filas.Close()

	var out []*domain.Espacio
	for filas.Next() {
		e, err := escanearEspacio(filas)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de espacio: %w", err)
		}
		out = append(out, e)
	}
	return out, errorDeFilas(filas)
}

// GuardarEspacio renombra el lugar y, con él, la materia con la que se reserva.
//
// Las dos juntas y en una transacción: la materia lleva el nombre del espacio y
// es lo que el docente ve en la lista al reservar. Si sólo se renombrara el
// espacio, la Biblioteca pasaría a llamarse «Biblioteca central» en Académico y
// seguiría diciendo «Biblioteca» en la pantalla de reservar, sin que nada lo
// explique.
func (r *PostgresRepo) GuardarEspacio(ctx context.Context, e *domain.Espacio) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abriendo transacción para guardar el espacio: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx,
		`UPDATE espacio SET nombre = $2, archivado = $3 WHERE id = $1`,
		e.ID, e.Nombre, e.Archivado)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		if esViolacionUnica(err) {
			return application.ErrNombreEspacioDuplicado
		}
		return fmt.Errorf("guardando espacio: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrEspacioNoEncontrado
	}

	// Sólo la materia que lleva el nombre viejo: si alguien cargó otras a mano
	// en la base, no se las toca.
	if _, err := tx.Exec(ctx,
		`UPDATE materia SET nombre = $2 WHERE espacio_id = $1`, e.ID, e.Nombre,
	); err != nil {
		return fmt.Errorf("renombrando la materia del espacio: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmando el cambio del espacio: %w", err)
	}
	return nil
}

// EliminarEspacio lo borra con sus materias (la FK es ON DELETE CASCADE).
//
// Quien llama tiene que haber comprobado antes que ninguna de esas materias
// tenga reservas: mismo criterio que eliminar un curso (RF-02.11). Acá no se
// vuelve a mirar porque esa pregunta la contesta el módulo de reservas, no
// éste.
func (r *PostgresRepo) EliminarEspacio(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM espacio WHERE id = $1`, id)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("eliminando espacio: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrEspacioNoEncontrado
	}
	return nil
}

// ListarMateriasPorEspacio: las materias que cuelgan de un espacio.
func (r *PostgresRepo) ListarMateriasPorEspacio(ctx context.Context, espacioID string) ([]*domain.Materia, error) {
	filas, err := r.pool.Query(ctx,
		`SELECT id, COALESCE(curso_id::text, ''), COALESCE(espacio_id::text, ''), nombre, archivado
		   FROM materia WHERE espacio_id = $1
		  ORDER BY clave_texto(nombre)`, espacioID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando materias del espacio: %w", err)
	}
	defer filas.Close()

	var out []*domain.Materia
	for filas.Next() {
		var m domain.Materia
		if err := filas.Scan(&m.ID, &m.CursoID, &m.EspacioID, &m.Nombre, &m.Archivado); err != nil {
			return nil, fmt.Errorf("escaneando materia del espacio: %w", err)
		}
		out = append(out, &m)
	}
	return out, errorDeFilas(filas)
}
