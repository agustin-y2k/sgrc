//go:build integration

package migrations_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/shared/testdb"
)

// sinIndicePermitidas son las claves foráneas que a propósito NO llevan índice.
//
// La lista es explícita para que agregar una foránea nueva sin índice FALLE
// este test: el autor tiene que decidir —y dejar escrito— si esa columna se
// indexa o si entra acá con su motivo. Sin eso, la deuda se acumula en
// silencio, que es exactamente como llegaron las quince que encontró la
// revisión.
var sinIndicePermitidas = map[string]string{
	"historico_uso_equipo.equipo_id": "un equipo nunca se borra de verdad (la baja es lógica), " +
		"así que esta foránea no se ejerce jamás en un DELETE; y la tabla se consulta sólo por anio",
	"historico_uso_docente.usuario_id": "una fila por docente por año: cientos, no millones, " +
		"y el padre se borra muy de vez en cuando",
}

// Una clave foránea cuya columna no encabeza ningún índice obliga a Postgres a
// recorrer la tabla hija entera cada vez que se borra una fila del padre — una
// vez por fila borrada. No se nota con la base recién cargada; se nota en las
// operaciones que borran de a muchas, que son las que corren desatendidas.
func TestLasForaneasQueSeBorranTienenIndice(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)

	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatalf("no se pudo aplicar el esquema: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT c.conrelid::regclass::text, a.attname
		  FROM pg_constraint c
		  JOIN unnest(c.conkey) WITH ORDINALITY AS k(attnum, ord) ON true
		  JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
		 WHERE c.contype = 'f'
		   AND c.connamespace = 'public'::regnamespace
		   AND k.ord = 1
		   -- "Encabeza un índice": Postgres sólo puede usarlo para esto si la
		   -- columna es la PRIMERA. Un índice (anio, usuario_id) no sirve para
		   -- buscar por usuario_id.
		   AND NOT EXISTS (
		       SELECT 1 FROM pg_index i
		        WHERE i.indrelid = c.conrelid AND i.indkey[0] = k.attnum)
		 ORDER BY 1, 2`)
	if err != nil {
		t.Fatalf("consultando las foráneas: %v", err)
	}
	defer rows.Close()

	var inesperadas []string
	encontradas := map[string]bool{}
	for rows.Next() {
		var tabla, columna string
		if err := rows.Scan(&tabla, &columna); err != nil {
			t.Fatalf("escaneando: %v", err)
		}
		clave := tabla + "." + columna
		encontradas[clave] = true
		if _, permitida := sinIndicePermitidas[clave]; !permitida {
			inesperadas = append(inesperadas, clave)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterando: %v", err)
	}

	if len(inesperadas) > 0 {
		sort.Strings(inesperadas)
		t.Errorf("estas claves foráneas no encabezan ningún índice:\n  %v\n\n"+
			"Cada borrado del padre va a recorrer la tabla hija entera, una vez por fila.\n"+
			"Agregale un índice, o sumala a sinIndicePermitidas con el motivo por el que\n"+
			"no hace falta (por ejemplo: el padre nunca se borra, o la hija es diminuta).",
			inesperadas)
	}

	// Y al revés: una excepción que ya no corresponde también ensucia. Si
	// alguien indexó una de las permitidas, hay que sacarla de la lista.
	for clave, motivo := range sinIndicePermitidas {
		if !encontradas[clave] {
			t.Errorf("%s ya tiene índice, así que sobra en sinIndicePermitidas (decía: %q)",
				clave, motivo)
		}
	}
}

// Los índices de la 012 tienen que servirle al chequeo de integridad
// referencial, que es lo único para lo que existen. Son PARCIALES
// (`WHERE col IS NOT NULL`), y que un índice parcial sirva depende de que el
// planificador deduzca que `col = $1` implica `col IS NOT NULL`.
//
// Esto lo comprueba sobre la forma EXACTA que usa el trigger —sentencia
// preparada, con parámetro y FOR KEY SHARE—, porque con una constante la prueba
// sería más fácil y no diría nada del caso real.
func TestElChequeoDeForaneaUsaElIndiceParcial(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)

	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatalf("no se pudo aplicar el esquema: %v", err)
	}

	// Con las tablas vacías el planificador elige recorrerlas igual, así que se
	// lo obliga a comparar costos como si tuvieran datos.
	if _, err := pool.Exec(ctx, `SET enable_seqscan = off`); err != nil {
		t.Fatalf("no se pudo desactivar el seq scan: %v", err)
	}

	casos := []struct{ tabla, columna, indice string }{
		{"notificacion", "reserva_id", "idx_notificacion_reserva"},
		{"reserva", "cancelado_por", "idx_reserva_cancelado_por"},
		{"prestamo", "entregado_por", "idx_prestamo_entregado_por"},
		{"incidencia", "reportado_por", "idx_incidencia_reportado_por"},
		{"usuario", "aprobado_por", "idx_usuario_aprobado_por"},
	}

	for _, c := range casos {
		nombre := "plan_" + c.tabla + "_" + c.columna
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			`PREPARE %s(uuid) AS SELECT 1 FROM %s WHERE %s = $1 FOR KEY SHARE`,
			nombre, c.tabla, c.columna)); err != nil {
			t.Errorf("preparando el plan de %s.%s: %v", c.tabla, c.columna, err)
			continue
		}

		// El plan viene en varias filas: hay que juntarlas todas, porque la
		// línea del Index Scan casi nunca es la primera.
		plan, err := planCompleto(ctx, pool, nombre)
		if err != nil {
			t.Errorf("explicando %s.%s: %v", c.tabla, c.columna, err)
			continue
		}
		if !strings.Contains(plan, c.indice) {
			t.Errorf("el chequeo de %s.%s no usa %s. El plan fue:\n%s\n"+
				"Si el índice es parcial, revisá que su predicado siga siendo deducible de la igualdad.",
				c.tabla, c.columna, c.indice, plan)
		}
	}
}

func planCompleto(ctx context.Context, pool *pgxpool.Pool, sentencia string) (string, error) {
	rows, err := pool.Query(ctx, fmt.Sprintf(
		`EXPLAIN (COSTS OFF) EXECUTE %s('00000000-0000-0000-0000-000000000000')`, sentencia))
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var lineas []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return "", err
		}
		lineas = append(lineas, l)
	}
	return strings.Join(lineas, "\n"), rows.Err()
}
