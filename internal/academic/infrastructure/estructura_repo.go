package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/academic/application"
)

// RF-02.12 — leer y escribir la estructura de un ciclo entero (cursos con sus
// materias) sin una consulta por curso, y copiar las materias de un curso a
// otros.
//
// Las tres viven acá y no en el servicio por el mismo motivo que ClonarCicloA:
// son multi-tabla y tienen que ser atómicas. Una importación que crea la mitad
// de los cursos y falla deja al Admin sin saber qué entró y qué no, y el camino
// para averiguarlo es revisar la pantalla curso por curso, que es exactamente el
// trabajo que la importación venía a evitar.

// clave es cómo se comparan dos textos para decidir si nombran la misma cosa:
// llamando a `clave_texto()`, la función que define la migración 011 y que
// sostiene TODOS los índices únicos de texto del sistema (ver
// docs/07-modelo-datos.md §5) — la materia dentro de su curso, y desde la
// migración 015 también el curso dentro de su ciclo.
//
// Se llama a la función y no se replica su expresión, que es lo que hacía esta
// misma constante para el curso mientras su índice dependía de las columnas
// generadas `*_norm`. Una expresión copiada funciona hasta que alguien toca una
// de las copias, y ésta la usan tres lugares: el índice único, el dedup de la
// migración y esta consulta. Con la función, que se separen deja de ser
// posible — y el síntoma de que se separaran sería una carga masiva que
// revienta contra el índice en vez de saltear la fila repetida.
//
// El `::text` no es decorativo. Cuando el MISMO parámetro se usa como valor a
// insertar y adentro de esta llamada, Postgres lo deduce varchar por la columna
// destino y text por la función, y rechaza la sentencia entera con un 42P08
// «inconsistent types deduced for parameter». El cast le da un solo tipo.
//
// El COALESCE va en la consulta y no acá porque sólo hace falta donde el texto
// puede ser NULL —la división y la modalidad de un curso—, y `clave_texto()` es
// STRICT: sobre NULL devuelve NULL, que no es igual a nada, ni siquiera a otro
// NULL.
const clave = `clave_texto($%d::text)`

// ListarEstructuraDeCiclo trae los cursos del ciclo con sus materias en UNA
// consulta, no en una más otra por curso.
//
// El array de materias lo arma Postgres con array_agg sobre el LEFT JOIN: un
// curso sin materias viene con el array vacío y no se pierde de la lista, que
// es justo el curso que hay que ver al mirar lo que falta cargar.
func (r *PostgresRepo) ListarEstructuraDeCiclo(ctx context.Context, cicloID string) ([]application.CursoConMaterias, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.anio,
		       COALESCE(c.division, ''),
		       COALESCE(c.modalidad, ''),
		       COALESCE(array_agg(m.nombre ORDER BY m.nombre) FILTER (WHERE m.id IS NOT NULL), '{}')
		FROM curso c
		LEFT JOIN materia m ON m.curso_id = c.id
		WHERE c.ciclo_lectivo_id = $1
		GROUP BY c.id, c.anio, c.division, c.modalidad
		-- ordenDeCurso nombra sus columnas sin calificar, y acá sirve igual:
		-- anio, division y modalidad existen sólo en curso, así que el JOIN con
		-- materia no las hace ambiguas.
		ORDER BY `+ordenDeCurso,
		cicloID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("leyendo la estructura del ciclo: %w", err)
	}
	defer rows.Close()

	var resultado []application.CursoConMaterias
	for rows.Next() {
		var c application.CursoConMaterias
		if err := rows.Scan(&c.Anio, &c.Division, &c.Modalidad, &c.Materias); err != nil {
			return nil, fmt.Errorf("escaneando fila de la estructura: %w", err)
		}
		resultado = append(resultado, c)
	}
	return resultado, errorDeFilas(rows)
}

// ImportarEstructura crea lo que falta y no toca lo que ya está, en una sola
// transacción.
//
// El "ya está" se decide por la terna normalizada del curso y por el nombre
// normalizado de la materia, que es como los compara la base. Buscar por el
// nombre exacto haría que «Matemática» y «Matematica» convivan en el mismo
// curso: el UNIQUE no lo impide y el docente que después elige su materia en un
// selector ve las dos.
func (r *PostgresRepo) ImportarEstructura(ctx context.Context, cicloID string, cursos []application.CursoConMaterias) (application.ResultadoImportacion, error) {
	var res application.ResultadoImportacion

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("iniciando transacción: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, curso := range cursos {
		cursoID, creado, err := buscarOCrearCurso(ctx, tx, cicloID, curso)
		if err != nil {
			return application.ResultadoImportacion{}, err
		}
		if creado {
			res.CursosCreados++
		} else {
			res.CursosExistentes++
		}

		for _, nombre := range curso.Materias {
			creada, err := crearMateriaSiFalta(ctx, tx, cursoID, nombre)
			if err != nil {
				return application.ResultadoImportacion{}, err
			}
			if creada {
				res.MateriasCreadas++
			} else {
				res.MateriasExistentes++
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return application.ResultadoImportacion{}, fmt.Errorf("confirmando la importación: %w", err)
	}
	return res, nil
}

// buscarOCrearCurso devuelve el ID del curso y si hubo que crearlo.
func buscarOCrearCurso(ctx context.Context, tx pgx.Tx, cicloID string, curso application.CursoConMaterias) (string, bool, error) {
	var id string
	err := tx.QueryRow(ctx, `
		SELECT id FROM curso
		 WHERE ciclo_lectivo_id = $1
		   AND anio = $2
		   AND clave_texto(COALESCE(division, ''))  = `+fmt.Sprintf(clave, 3)+`
		   AND clave_texto(COALESCE(modalidad, '')) = `+fmt.Sprintf(clave, 4),
		cicloID, curso.Anio, curso.Division, curso.Modalidad,
	).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		if esIDInvalido(err) {
			return "", false, application.ErrIDInvalido
		}
		return "", false, fmt.Errorf("buscando el curso %d°%s: %w", curso.Anio, curso.Division, err)
	}

	id = uuidNuevo()
	if _, err := tx.Exec(ctx,
		`INSERT INTO curso (id, ciclo_lectivo_id, anio, division, modalidad, archivado)
		 VALUES ($1, $2, $3, $4, $5, false)`,
		id, cicloID, curso.Anio, nullSiVacio(curso.Division), nullSiVacio(curso.Modalidad),
	); err != nil {
		if esViolacionFK(err) {
			return "", false, application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return "", false, application.ErrIDInvalido
		}
		return "", false, fmt.Errorf("creando el curso %d°%s: %w", curso.Anio, curso.Division, err)
	}
	return id, true, nil
}

// crearMateriaSiFalta inserta la materia salvo que el curso ya tenga una con el
// mismo nombre normalizado. El WHERE NOT EXISTS va DENTRO del INSERT para que la
// decisión y la escritura sean la misma sentencia: separadas, dos importaciones
// simultáneas pueden mirar las dos, encontrar el hueco las dos y chocar contra
// el UNIQUE la segunda.
func crearMateriaSiFalta(ctx context.Context, tx pgx.Tx, cursoID, nombre string) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO materia (id, curso_id, nombre, archivado)
		SELECT $1, $2, $3::text, false
		 WHERE NOT EXISTS (
		       SELECT 1 FROM materia
		        WHERE curso_id = $2
		          AND clave_texto(nombre) = `+fmt.Sprintf(clave, 3)+`)`,
		uuidNuevo(), cursoID, nombre)
	if err != nil {
		if esViolacionUnica(err) {
			return false, application.ErrMateriaNombreDuplicado
		}
		if esViolacionFK(err) {
			return false, application.ErrReferenciaInexistente
		}
		return false, fmt.Errorf("creando la materia «%s»: %w", nombre, err)
	}
	return tag.RowsAffected() == 1, nil
}

// CopiarMateriasA duplica los nombres de materia del curso de origen en cada
// destino, salteando las que el destino ya tenga.
//
// Copia NOMBRES y no filas: una materia es propia de su curso (la Matemática de
// 1°1 no es la de 1°2), así que lo que se comparte es cómo se llama. Las
// asignaciones de docentes no viajan, por el mismo motivo que no viajan al
// clonar un ciclo (RF-02.5): quién dicta qué es una decisión por curso.
func (r *PostgresRepo) CopiarMateriasA(ctx context.Context, cursoOrigenID string, cursosDestinoIDs []string) (application.ResultadoCopia, error) {
	res := application.ResultadoCopia{CursosDestino: len(cursosDestinoIDs)}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return application.ResultadoCopia{}, fmt.Errorf("iniciando transacción: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx, `SELECT nombre FROM materia WHERE curso_id = $1 ORDER BY nombre`, cursoOrigenID)
	if err != nil {
		if esIDInvalido(err) {
			return application.ResultadoCopia{}, application.ErrIDInvalido
		}
		return application.ResultadoCopia{}, fmt.Errorf("leyendo las materias a copiar: %w", err)
	}
	var nombres []string
	for rows.Next() {
		var nombre string
		if err := rows.Scan(&nombre); err != nil {
			rows.Close()
			return application.ResultadoCopia{}, fmt.Errorf("escaneando materia a copiar: %w", err)
		}
		nombres = append(nombres, nombre)
	}
	rows.Close()
	if err := errorDeFilas(rows); err != nil {
		return application.ResultadoCopia{}, err
	}

	for _, destinoID := range cursosDestinoIDs {
		for _, nombre := range nombres {
			creada, err := crearMateriaSiFalta(ctx, tx, destinoID, nombre)
			if err != nil {
				return application.ResultadoCopia{}, err
			}
			if creada {
				res.MateriasCreadas++
			} else {
				res.MateriasExistentes++
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return application.ResultadoCopia{}, fmt.Errorf("confirmando la copia de materias: %w", err)
	}
	return res, nil
}
