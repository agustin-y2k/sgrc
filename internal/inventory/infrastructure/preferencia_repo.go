package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/agustin-y2k/sgrc/internal/inventory/application"
	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
)

// Las columnas `*_norm` no se escriben nunca desde acá: son generadas por la
// base a partir de materia_nombre, modalidad y división.
const columnasPreferencia = `id, equipo_id, materia_nombre, modalidad, anio, division, prioridad, curso_exacto`

// Las mismas columnas calificadas, para las consultas que hacen JOIN con
// equipo y ahí `id` es ambiguo. Existe para que las dos listas no se puedan
// desincronizar: escanearPreferencia lee en este orden, y un orden distinto
// falla recién al escanear, en runtime.
const columnasPreferenciaEP = `ep.id, ep.equipo_id, ep.materia_nombre, ep.modalidad, ep.anio, ep.division, ep.prioridad, ep.curso_exacto`

// CrearPreferencia devuelve `false` —sin error— cuando ese equipo ya tenía esa
// marca. Por el mismo motivo que CrearLicencia: una sentencia fallida aborta la
// transacción entera, y el marcado de un lote de equipos tiene que poder
// saltear los que ya la tienen y seguir.
//
// El ON CONFLICT nombra las seis columnas del índice `ux_equipo_preferencia`,
// que es NULLS NOT DISTINCT: dos marcas sin año y sin división son la misma
// marca, y así es como se comparan.
func (r *PostgresRepo) CrearPreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) (bool, error) {
	tag, err := r.conexion(ctx).Exec(ctx, `
		INSERT INTO equipo_preferencia (`+columnasPreferencia+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (equipo_id, materia_norm, modalidad_norm, anio, division_norm, curso_exacto) DO NOTHING
	`, p.ID, p.EquipoID, p.MateriaNombre, p.Modalidad, p.Anio, p.Division, p.Prioridad, p.CursoExacto)
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
	tag, err := r.conexion(ctx).Exec(ctx, `
		UPDATE equipo_preferencia
		   SET modalidad = $2, anio = $3, division = $4, prioridad = $5, curso_exacto = $6
		 WHERE id = $1
	`, p.ID, p.Modalidad, p.Anio, p.Division, p.Prioridad, p.CursoExacto)
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
	row := r.conexion(ctx).QueryRow(ctx, `SELECT `+columnasPreferencia+` FROM equipo_preferencia WHERE id = $1`, id)
	return escanearPreferencia(row)
}

func (r *PostgresRepo) BorrarPreferencia(ctx context.Context, id string) error {
	tag, err := r.conexion(ctx).Exec(ctx, `DELETE FROM equipo_preferencia WHERE id = $1`, id)
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

// ListarPreferenciasPorEquipo devuelve las marcas de un equipo por PRIORIDAD,
// de la más fuerte a la más débil, y recién dentro de cada prioridad agrupa por
// materia y va de lo más acotado a lo que vale para todo.
//
// Ojo con leer este orden como el de la resolución: cuál marca gana al reservar
// lo decide primero la especificidad y después la prioridad, y además depende
// del curso que se esté reservando, que esta lista no conoce. Acá el criterio
// es otro: la ficha del equipo muestra arriba lo que la escuela declaró más
// importante.
func (r *PostgresRepo) ListarPreferenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.PreferenciaDeEquipo, error) {
	rows, err := r.conexion(ctx).Query(ctx, `
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

// ListarPreferenciasPorCarro: las marcas de todos los equipos de un carro.
//
// La pide el alta masiva para avisar, ANTES de guardar, que la prioridad
// elegida ya la usa otra materia en esas mismas máquinas. Por equipo ya se
// podía preguntar; pedirlo de a uno para un carro de veinte eran veinte
// consultas para contestar una sola pregunta.
func (r *PostgresRepo) ListarPreferenciasPorCarro(ctx context.Context, carroID string) ([]*domain.PreferenciaDeEquipo, error) {
	rows, err := r.conexion(ctx).Query(ctx, `
		SELECT `+columnasPreferenciaEP+`
		  FROM equipo_preferencia ep
		  JOIN equipo e ON e.id = ep.equipo_id
		 WHERE e.carro_id = $1
		 ORDER BY e.identificador NULLS LAST, ep.prioridad, ep.materia_nombre
	`, carroID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando preferencias del carro: %w", err)
	}
	defer rows.Close()

	resultado := []*domain.PreferenciaDeEquipo{}
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
	rows, err := r.conexion(ctx).Query(ctx, `
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
		&p.Division, &p.Prioridad, &p.CursoExacto); err != nil {
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

// sqlLeAplicaAAlgunaMateria contesta «¿la marca `ep` le alcanza a alguna
// materia cargada?», con las mismas cuatro condiciones que aplica la consulta
// de reservas al armar el tramo propio (RF-03.21).
//
// Vive una sola vez porque la comparten las dos preguntas que se hacen sobre
// una marca —si quedó huérfana, y cuántas mueren al renombrar—. Separadas,
// cada pantalla medía algo distinto y las dos se contradecían.
//
// Dos cosas que tuvo que aprender:
//
//   - **Mira el ALCANCE, no sólo el nombre.** Una marca de "Matemática de 3°B"
//     cuando no hay ningún 3°B no le aplica a nadie, por más que exista una
//     Matemática en 5°A. Sin esto esas marcas muertas eran invisibles.
//   - **Compara por `nombre_norm` y no por `clave_texto`.** Es la columna que
//     usa el cruce real al reservar. `clave_texto` además colapsa los espacios,
//     así que daba por viva una marca con un espacio de más que el sistema no
//     iba a usar nunca.
const sqlLeAplicaAAlgunaMateria = `
		SELECT 1
		  FROM materia m
		  JOIN contenedor_de_materia cu ON cu.materia_id = m.id
		 WHERE m.nombre_norm = ep.materia_norm
		   AND (CASE WHEN ep.curso_exacto THEN (ep.anio IS NOT DISTINCT FROM cu.anio AND ep.modalidad_norm = cu.modalidad_norm AND ep.division_norm = cu.division_norm) ELSE ((ep.anio IS NULL OR ep.anio IS NOT DISTINCT FROM cu.anio) AND (ep.modalidad_norm = '' OR ep.modalidad_norm = cu.modalidad_norm) AND (ep.division_norm = '' OR ep.division_norm = cu.division_norm)) END)`

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
// El cruce usa las columnas normalizadas del alcance, igual que al reservar.
func (r *PostgresRepo) ListarPreferenciasHuerfanas(ctx context.Context) ([]*application.PreferenciaHuerfana, error) {
	rows, err := r.conexion(ctx).Query(ctx, `
		SELECT ep.id, ep.materia_nombre, ep.modalidad, ep.anio, ep.division, ep.prioridad,
		       COALESCE(e.nombre, 'PC ' || e.identificador), COALESCE(c.nombre, ''),
		       e.dado_de_baja, ep.curso_exacto
		  FROM equipo_preferencia ep
		  JOIN equipo e ON e.id = ep.equipo_id
		  LEFT JOIN carro c ON c.id = e.carro_id
		 WHERE NOT EXISTS (`+sqlLeAplicaAAlgunaMateria+`)
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
			&h.Prioridad, &h.EquipoEtiqueta, &h.CarroNombre, &h.EquipoDadoDeBaja, &h.CursoExacto); err != nil {
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

// ContarPreferenciasQueDejarianDeAplicar: cuántas marcas se quedan sin NINGUNA
// materia si se renombra ésta.
//
// Es la pregunta que hay que poder contestar ANTES de renombrar, para avisar en
// vez de romper en silencio. Se corre con el nombre viejo todavía en la base.
//
// Cuenta las que mueren de verdad, y para eso hacen falta las dos mitades:
//
//  1. que la marca le aplicara a ESTA materia (si estaba acotada a otro curso,
//     el renombre no la toca);
//  2. que después no le quede ninguna otra. Es lo que descarta a las marcas sin
//     alcance cuando la escuela tiene la misma materia en varios cursos:
//     renombrar la de 3°A no las deja sin trabajo, siguen sirviendo en 5°B.
//
// Sin (2) el número era casi siempre "todas", y contradecía a la lista de
// huérfanas, que sí veía que la marca seguía cruzando.
func (r *PostgresRepo) ContarPreferenciasQueDejarianDeAplicar(ctx context.Context, materiaID, materiaNombre string) (int, error) {
	var n int
	err := r.conexion(ctx).QueryRow(ctx, `
		SELECT count(*)
		  FROM equipo_preferencia ep
		  JOIN contenedor_de_materia obj ON obj.materia_id = $1
		 WHERE ep.materia_norm = translate(lower(btrim($2::text)), 'áéíóúüñ', 'aeiouun')
		   AND (CASE WHEN ep.curso_exacto THEN (ep.anio IS NOT DISTINCT FROM obj.anio AND ep.modalidad_norm = obj.modalidad_norm AND ep.division_norm = obj.division_norm) ELSE ((ep.anio IS NULL OR ep.anio IS NOT DISTINCT FROM obj.anio) AND (ep.modalidad_norm = '' OR ep.modalidad_norm = obj.modalidad_norm) AND (ep.division_norm = '' OR ep.division_norm = obj.division_norm)) END)
		   AND NOT EXISTS (`+sqlLeAplicaAAlgunaMateria+` AND m.id <> $1)`,
		materiaID, materiaNombre).Scan(&n)
	if err != nil {
		if esIDInvalido(err) {
			return 0, application.ErrIDInvalido
		}
		return 0, fmt.Errorf("contando las marcas que dejarían de aplicar: %w", err)
	}
	return n, nil
}
