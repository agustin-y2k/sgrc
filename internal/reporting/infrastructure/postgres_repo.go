// Package infrastructure implementa application.Repo de reporting contra
// PostgreSQL real (pgx), además de los adaptadores InfoEquipoParaSnapshot e
// InfoUsuarioParaSnapshot.
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/reporting/application"
	"github.com/ramiro/sgrc/internal/reporting/domain"
)

const codigoTextoInvalido = "22P02"

func esIDInvalido(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoTextoInvalido
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

var _ application.Repo = (*PostgresRepo)(nil)

type PostgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

// ── Snapshot histórico (persistencia propia de reporting) ──────────────

// Los dos Guardar* van con ON CONFLICT DO NOTHING sobre UNIQUE(anio, …)
// para que el archivado de un ciclo (RF-02.4) se pueda reintentar. La
// cascada cruza tres paquetes sin una transacción que los abarque, así que
// un fallo en el borrado de reservas deja el snapshot ya escrito; sin esta
// cláusula, el reintento moría en una violación de constraint.
//
// DO NOTHING y no DO UPDATE: si el snapshot del año ya existe, es el bueno
// —se calculó cuando las reservas todavía estaban vivas—. Un DO UPDATE
// pisaría esos números con los que devuelva un recálculo posterior al
// borrado, o sea con ceros. El único caso en que faltan filas es un
// snapshot interrumpido a la mitad, y ahí las reservas siguen intactas, así
// que el recálculo da lo mismo que la primera vez y solo completa lo que
// falta.
func (r *PostgresRepo) GuardarHistoricoUsoEquipo(ctx context.Context, h *domain.HistoricoUsoEquipo) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO historico_uso_equipo (id, anio, equipo_id, etiqueta_snapshot, identificador_snapshot, carro_nombre_snapshot, minutos_reservados, cantidad_reservas)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (anio, equipo_id) DO NOTHING
	`, h.ID, h.Anio, h.EquipoID, h.EtiquetaSnapshot, h.IdentificadorSnapshot, h.CarroNombreSnapshot, h.MinutosReservados, h.CantidadReservas)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("guardando histórico de uso de PC: %w", err)
	}
	return nil
}

func (r *PostgresRepo) GuardarHistoricoUsoDocente(ctx context.Context, h *domain.HistoricoUsoDocente) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO historico_uso_docente (id, anio, usuario_id, nombre_docente_snapshot, cantidad_reservas, minutos_totales)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (anio, usuario_id) DO NOTHING
	`, h.ID, h.Anio, h.UsuarioID, h.NombreDocenteSnapshot, h.CantidadReservas, h.MinutosTotales)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("guardando histórico de uso de docente: %w", err)
	}
	return nil
}

// BorrarHistoricoDocentesSinCuenta corre antes de escribir el snapshot del
// año (ver application.CalcularSnapshotAnual): esas filas no las cubre el
// ON CONFLICT, así que se rehacen enteras en cada intento en vez de
// acumularse. Son las de los docentes cuya cuenta ya no existe, y su
// contenido depende solo de las reservas, que en ese momento siguen vivas.
func (r *PostgresRepo) BorrarHistoricoDocentesSinCuenta(ctx context.Context, anio int) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM historico_uso_docente WHERE anio = $1 AND usuario_id IS NULL`, anio)
	if err != nil {
		return fmt.Errorf("borrando histórico de docentes sin cuenta: %w", err)
	}
	return nil
}

func (r *PostgresRepo) ListarHistoricoUsoEquipoPorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoEquipo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, anio, equipo_id, etiqueta_snapshot,
		       COALESCE(identificador_snapshot, 0), COALESCE(carro_nombre_snapshot, ''),
		       minutos_reservados, cantidad_reservas
		FROM historico_uso_equipo WHERE anio = $1
		-- Las PCs de carro primero y por número; los equipos sueltos después,
		-- juntos y por nombre. equipo_id cierra el orden: el identificador se
		-- repite entre carros y es NULL en los sueltos.
		ORDER BY identificador_snapshot NULLS LAST, etiqueta_snapshot, equipo_id
	`, anio)
	if err != nil {
		return nil, fmt.Errorf("listando histórico de uso de PC: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.HistoricoUsoEquipo
	for rows.Next() {
		var h domain.HistoricoUsoEquipo
		if err := rows.Scan(&h.ID, &h.Anio, &h.EquipoID, &h.EtiquetaSnapshot, &h.IdentificadorSnapshot, &h.CarroNombreSnapshot, &h.MinutosReservados, &h.CantidadReservas); err != nil {
			return nil, fmt.Errorf("escaneando fila de histórico de PC: %w", err)
		}
		resultado = append(resultado, &h)
	}
	return resultado, errorDeFilas(rows)
}

func (r *PostgresRepo) ListarHistoricoUsoDocentePorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoDocente, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, anio, usuario_id, nombre_docente_snapshot, cantidad_reservas, minutos_totales
		FROM historico_uso_docente WHERE anio = $1 ORDER BY nombre_docente_snapshot
	`, anio)
	if err != nil {
		return nil, fmt.Errorf("listando histórico de uso de docente: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.HistoricoUsoDocente
	for rows.Next() {
		var h domain.HistoricoUsoDocente
		if err := rows.Scan(&h.ID, &h.Anio, &h.UsuarioID, &h.NombreDocenteSnapshot, &h.CantidadReservas, &h.MinutosTotales); err != nil {
			return nil, fmt.Errorf("escaneando fila de histórico de docente: %w", err)
		}
		resultado = append(resultado, &h)
	}
	return resultado, errorDeFilas(rows)
}

// ── Agregaciones en vivo (consultan reserva/materia/curso directamente —
// sin importar internal/reservation ni internal/academic) ─────────────

// minutosPorRango convierte la diferencia hora_fin - hora_inicio (un
// INTERVAL de Postgres) a minutos enteros. EXTRACT(EPOCH FROM ...)
// devuelve segundos; /60 y redondeando da minutos — mismo criterio que
// ROUND para evitar arrastrar decimales de segundo por horarios que no
// caen en minutos exactos (no debería pasar en la práctica, pero
// ROUND es más seguro que truncar).
//
// Va calificada con el alias de tabla porque todas las consultas de uso
// hacen JOIN (con pc/carro o con usuario, para traer los nombres) y ahí
// las columnas sueltas serían ambiguas.
func expresionMinutosDe(alias string) string {
	return fmt.Sprintf(`ROUND(EXTRACT(EPOCH FROM (%s.hora_fin - %s.hora_inicio)) / 60)::INTEGER`, alias, alias)
}

// condFechasPrefijo reescribe las condiciones de rango para que apunten a
// la tabla con alias (filtroFechas las genera sin calificar).
func condFechasPrefijo(alias, cond string) string {
	return strings.ReplaceAll(cond, " fecha ", " "+alias+".fecha ")
}

// filtroFechas agrega las condiciones de rango (RF-06.1) a una query que
// ya trae al menos un parámetro. Devuelve el fragmento SQL y los args
// actualizados.
func filtroFechas(columna string, desde, hasta *time.Time, args []any) (string, []any) {
	sql := ""
	if desde != nil {
		args = append(args, *desde)
		sql += fmt.Sprintf(" AND %s >= $%d", columna, len(args))
	}
	if hasta != nil {
		args = append(args, *hasta)
		sql += fmt.Sprintf(" AND %s <= $%d", columna, len(args))
	}
	return sql, args
}

func (r *PostgresRepo) CalcularUsoEquiposDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoEquipo, error) {
	args := []any{cicloID}
	condFechas, args := filtroFechas("fecha", desde, hasta, args)

	rows, err := r.pool.Query(ctx, `
		SELECT r.equipo_id, COALESCE(p.nombre, 'PC ' || p.identificador),
		       COALESCE(p.identificador, 0), COALESCE(ca.nombre, ''),
		       COUNT(*), COALESCE(SUM(`+expresionMinutosDe("r")+`), 0) AS minutos
		FROM reserva r
		JOIN equipo p ON p.id = r.equipo_id
		-- LEFT: un proyector reservable no está en ningún carro,
		-- y con INNER JOIN sus reservas no figuraban en el reporte de uso —
		-- el equipo más peleado de la escuela podía aparecer como sin usar.
		LEFT JOIN carro ca ON ca.id = p.carro_id
		WHERE (
			r.materia_id IN (
				SELECT m.id FROM materia m JOIN curso c ON c.id = m.curso_id WHERE c.ciclo_lectivo_id = $1
			)
			-- Los bloqueos administrativos (RF-04.7) no tienen materia, así que
			-- el filtro de arriba no los alcanza. Igual ocupan el equipo: sin
			-- esto, una máquina muy usada para exámenes figura como poco
			-- usada. Se los ata al ciclo por el año de la fecha, que es lo
			-- único que los relaciona con un ciclo lectivo.
			OR (
				r.tipo = 'BLOQUEO'
				AND EXTRACT(YEAR FROM r.fecha) = (SELECT anio FROM ciclo_lectivo WHERE id = $1)
			)
		)
		-- NO_RETIRADA queda afuera junto con CANCELADA (RF-08.10): nadie fue a
		-- buscar esa máquina, así que no fue una clase dada. Contarla infla
		-- justamente el número con el que se pide presupuesto — un carro que
		-- nadie retira nunca aparecería como el más usado de la institución.
		AND r.estado NOT IN ('CANCELADA','NO_RETIRADA')`+condFechasPrefijo("r", condFechas)+`
		GROUP BY r.equipo_id, p.nombre, p.identificador, ca.nombre
		-- Del más usado al menos usado, que es la pregunta que trae a
		-- alguien a este reporte. Sin ORDER BY, Postgres devuelve las filas
		-- en el orden en que salen del hash de agregación: no es aleatorio,
		-- pero tampoco es estable — dos llamadas iguales pueden diferir si
		-- cambia el plan, y comparar dos consultas deja de tener sentido.
		-- Los otros cuatro reportes de este archivo ya ordenaban; estos dos
		-- eran la excepción.
		-- El identificador desempata para que dos PCs con el mismo uso no se
		-- intercambien de lugar entre llamadas. No alcanza solo: se repite
		-- entre carros y es NULL en los equipos sueltos, así que
		-- r.equipo_id cierra el orden.
		ORDER BY minutos DESC, p.identificador NULLS LAST, r.equipo_id
	`, args...)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("calculando uso de equipos del ciclo: %w", err)
	}
	defer rows.Close()

	var resultado []domain.ResumenUsoEquipo
	for rows.Next() {
		var u domain.ResumenUsoEquipo
		if err := rows.Scan(&u.EquipoID, &u.Etiqueta, &u.Identificador, &u.CarroNombre, &u.CantidadReservas, &u.MinutosReservados); err != nil {
			return nil, fmt.Errorf("escaneando fila de uso de PC: %w", err)
		}
		resultado = append(resultado, u)
	}
	return resultado, errorDeFilas(rows)
}

func (r *PostgresRepo) CalcularUsoDocentesDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoDocente, error) {
	args := []any{cicloID}
	condFechas, args := filtroFechas("fecha", desde, hasta, args)

	// El JOIN a usuario va LEFT y no se filtra por creado_por IS NOT NULL: al
	// eliminar definitivamente una cuenta (RF-01.9) esa columna queda en NULL,
	// y con INNER las reservas de esa persona desaparecían del reporte y de
	// los totales del año. El nombre sale entonces del snapshot que la fila
	// guarda justamente para eso.
	//
	// Se agrupa también por nombre_docente_snapshot: sin eso, todas las
	// cuentas eliminadas caerían en un único renglón sin nombre, porque en un
	// GROUP BY los NULL se juntan entre sí.
	rows, err := r.pool.Query(ctx, `
		SELECT r.creado_por,
		       COALESCE(MAX(u.nombre || ' ' || u.apellido), r.nombre_docente_snapshot, '') AS docente,
		       COUNT(*), COALESCE(SUM(`+expresionMinutosDe("r")+`), 0) AS minutos
		FROM reserva r
		LEFT JOIN usuario u ON u.id = r.creado_por
		WHERE r.materia_id IN (
			SELECT m.id FROM materia m JOIN curso c ON c.id = m.curso_id WHERE c.ciclo_lectivo_id = $1
		)
		-- Ver CalcularUsoEquiposDeCiclo: una clase que nadie retiró no cuenta.
		AND r.estado NOT IN ('CANCELADA','NO_RETIRADA')`+condFechasPrefijo("r", condFechas)+`
		GROUP BY r.creado_por, r.nombre_docente_snapshot
		-- Mismo criterio que CalcularUsoEquiposDeCiclo; el nombre desempata.
		ORDER BY minutos DESC, docente
	`, args...)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("calculando uso de docentes del ciclo: %w", err)
	}
	defer rows.Close()

	var resultado []domain.ResumenUsoDocente
	for rows.Next() {
		var u domain.ResumenUsoDocente
		if err := rows.Scan(&u.UsuarioID, &u.NombreDocente, &u.CantidadReservas, &u.MinutosReservados); err != nil {
			return nil, fmt.Errorf("escaneando fila de uso de docente: %w", err)
		}
		resultado = append(resultado, u)
	}
	return resultado, errorDeFilas(rows)
}

// ── RF-06.3: incidencias por equipo y por carro ────────────────────────
//
// A diferencia del uso de equipos/docentes, estas consultas NO dependen del
// ciclo lectivo ni necesitan snapshot: Incidencia nunca se elimina, así
// que el dato histórico siempre está disponible en vivo (ver RF-02.4).
// El LEFT JOIN es contra las tablas de inventory, de solo lectura — mismo
// criterio que el resto de los adaptadores de este paquete.

func (r *PostgresRepo) CalcularIncidenciasPorEquipo(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasEquipo, error) {
	args := []any{}
	condFechas, args := filtroFechas("i.fecha", desde, hasta, args)

	rows, err := r.pool.Query(ctx, `
		SELECT p.id, COALESCE(p.nombre, 'PC ' || p.identificador),
		       COALESCE(p.identificador, 0), COALESCE(ca.nombre, ''),
		       COUNT(i.id),
		       COUNT(*) FILTER (WHERE i.estado = 'ABIERTA'),
		       COUNT(*) FILTER (WHERE i.estado = 'EN_REPARACION'),
		       COUNT(*) FILTER (WHERE i.estado = 'ENVIADA_A_SOPORTE'),
		       COUNT(*) FILTER (WHERE i.estado = 'RESUELTA'),
		       COUNT(*) FILTER (WHERE i.gravedad = 'GRAVE')
		FROM incidencia i
		JOIN equipo p ON p.id = i.equipo_id
		-- LEFT: al proyector también se le rompe la lámpara, y
		-- con INNER JOIN sus incidencias no llegaban a este reporte.
		LEFT JOIN carro ca ON ca.id = p.carro_id
		WHERE 1=1`+condFechas+`
		GROUP BY p.id, p.nombre, p.identificador, ca.nombre
		ORDER BY COUNT(i.id) DESC, p.identificador NULLS LAST, p.id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("calculando incidencias por PC: %w", err)
	}
	defer rows.Close()

	var resultado []domain.ResumenIncidenciasEquipo
	for rows.Next() {
		var x domain.ResumenIncidenciasEquipo
		if err := rows.Scan(&x.EquipoID, &x.Etiqueta, &x.Identificador, &x.CarroNombre,
			&x.Total, &x.Abiertas, &x.EnReparacion, &x.EnviadasASoporte, &x.Resueltas, &x.Graves); err != nil {
			return nil, fmt.Errorf("escaneando incidencias por PC: %w", err)
		}
		resultado = append(resultado, x)
	}
	return resultado, errorDeFilas(rows)
}

func (r *PostgresRepo) CalcularIncidenciasPorCarro(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasCarro, error) {
	args := []any{}
	condFechas, args := filtroFechas("i.fecha", desde, hasta, args)

	rows, err := r.pool.Query(ctx, `
		SELECT ca.id, ca.nombre,
		       COUNT(i.id),
		       COUNT(*) FILTER (WHERE i.estado = 'ABIERTA'),
		       COUNT(*) FILTER (WHERE i.gravedad = 'GRAVE')
		FROM incidencia i
		JOIN equipo p ON p.id = i.equipo_id
		-- INNER a propósito, al revés que los otros dos reportes: este agrupa
		-- POR CARRO, y un equipo suelto no está en ninguno. Sumarlo pediría
		-- inventar un carro "Sueltos" que no existe ni en la base ni en el
		-- pasillo. Sus incidencias se ven en el reporte por equipo.
		JOIN carro ca ON ca.id = p.carro_id
		WHERE 1=1`+condFechas+`
		GROUP BY ca.id, ca.nombre
		ORDER BY COUNT(i.id) DESC, ca.nombre
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("calculando incidencias por carro: %w", err)
	}
	defer rows.Close()

	var resultado []domain.ResumenIncidenciasCarro
	for rows.Next() {
		var x domain.ResumenIncidenciasCarro
		if err := rows.Scan(&x.CarroID, &x.CarroNombre, &x.Total, &x.Abiertas, &x.Graves); err != nil {
			return nil, fmt.Errorf("escaneando incidencias por carro: %w", err)
		}
		resultado = append(resultado, x)
	}
	return resultado, errorDeFilas(rows)
}

// ── Estado del parque de equipos (RF-06.5) ──────────────────────────────

// EstadoDelInventario cuenta equipos por estado, agrupados por carro.
//
// Los dados de baja quedan afuera de todo: ya no son parte del parque, y
// contarlos como "fuera de servicio" inflaría el número que la escuela usa
// para pedir presupuesto con máquinas que ya nadie espera recuperar.
//
// LEFT JOIN a carro para que los equipos sueltos aparezcan igual, en su
// propia fila: un proyector roto también sale de circulación.
func (r *PostgresRepo) EstadoDelInventario(ctx context.Context) ([]domain.EstadoDelInventario, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(c.id::text, ''), COALESCE(c.nombre, ''),
		       COUNT(*) FILTER (WHERE e.estado = 'DISPONIBLE'),
		       COUNT(*) FILTER (WHERE e.estado = 'EN_MANTENIMIENTO'),
		       COUNT(*) FILTER (WHERE e.estado = 'FUERA_DE_SERVICIO'),
		       COUNT(*)
		FROM equipo e
		LEFT JOIN carro c ON c.id = e.carro_id
		WHERE e.dado_de_baja = false
		GROUP BY c.id, c.nombre
		-- Los sueltos al final: el carro es la unidad con la que se piensa el
		-- inventario, y lo que no está en ninguno se lee como el resto.
		ORDER BY c.nombre IS NULL, c.nombre
	`)
	if err != nil {
		return nil, fmt.Errorf("calculando el estado del inventario: %w", err)
	}
	defer rows.Close()

	var resultado []domain.EstadoDelInventario
	for rows.Next() {
		var e domain.EstadoDelInventario
		if err := rows.Scan(&e.CarroID, &e.CarroNombre, &e.Disponibles,
			&e.EnMantenimiento, &e.FueraDeServicio, &e.Total); err != nil {
			return nil, fmt.Errorf("escaneando estado del inventario: %w", err)
		}
		resultado = append(resultado, e)
	}
	return resultado, errorDeFilas(rows)
}

// EquiposFueraDeCirculacion lista lo que hoy no se puede reservar, con la
// última incidencia de cada máquina.
//
// DISTINCT ON resuelve "la más reciente de cada equipo" en una sola pasada;
// con una subconsulta correlacionada serían tantas consultas como equipos
// listados. El LEFT JOIN es lo que deja aparecer a los que NO tienen ninguna
// incidencia cargada — que es un caso a mostrar, no a esconder: alguien sacó
// la máquina de circulación sin escribir por qué.
func (r *PostgresRepo) EquiposFueraDeCirculacion(ctx context.Context) ([]domain.EquipoFueraDeCirculacion, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id,
		       COALESCE(e.nombre, 'PC ' || e.identificador),
		       COALESCE(c.nombre, ''),
		       e.estado,
		       COALESCE(ult.categoria, ''),
		       COALESCE(ult.descripcion, ''),
		       COALESCE(ult.estado, '')
		FROM equipo e
		LEFT JOIN carro c ON c.id = e.carro_id
		LEFT JOIN LATERAL (
			SELECT i.categoria, i.descripcion, i.estado
			FROM incidencia i
			WHERE i.equipo_id = e.id
			ORDER BY i.fecha DESC
			LIMIT 1
		) ult ON true
		WHERE e.dado_de_baja = false
		  AND e.estado IN ('EN_MANTENIMIENTO', 'FUERA_DE_SERVICIO')
		ORDER BY e.estado, c.nombre IS NULL, c.nombre, e.identificador NULLS LAST, e.id
	`)
	if err != nil {
		return nil, fmt.Errorf("listando equipos fuera de circulación: %w", err)
	}
	defer rows.Close()

	var resultado []domain.EquipoFueraDeCirculacion
	for rows.Next() {
		var e domain.EquipoFueraDeCirculacion
		if err := rows.Scan(&e.EquipoID, &e.Etiqueta, &e.CarroNombre, &e.Estado,
			&e.Categoria, &e.UltimaFalla, &e.EstadoIncidencia); err != nil {
			return nil, fmt.Errorf("escaneando equipo fuera de circulación: %w", err)
		}
		resultado = append(resultado, e)
	}
	return resultado, errorDeFilas(rows)
}

// CalcularIncidenciasPorCategoria responde "qué se rompe acá".
//
// Agrupa por lower(categoria) y no por el texto tal cual: la categoría es
// libre, así que "Batería" y "batería" son la misma falla escrita por
// dos personas distintas. Se muestra MIN(categoria) como etiqueta, que es
// estable entre corridas.
//
// Las no clasificadas caen todas en una fila con categoría vacía en vez de
// quedar afuera: cuántas fallas nadie pudo diagnosticar es uno de los
// números que el reporte tiene que dar.
func (r *PostgresRepo) CalcularIncidenciasPorCategoria(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenPorCategoriaDeFalla, error) {
	args := []any{}
	condFechas, args := filtroFechas("i.fecha", desde, hasta, args)

	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(MIN(i.categoria), ''),
		       COUNT(*),
		       COUNT(*) FILTER (WHERE i.estado = 'ABIERTA'),
		       COUNT(DISTINCT i.equipo_id)
		FROM incidencia i
		WHERE 1=1`+condFechas+`
		GROUP BY lower(i.categoria)
		-- De lo más frecuente a lo menos, que es la pregunta que trae a
		-- alguien acá. El segundo criterio desempata para que dos categorías
		-- con la misma cuenta no se intercambien entre llamadas.
		ORDER BY COUNT(*) DESC, lower(i.categoria) NULLS LAST
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("calculando incidencias por categoría: %w", err)
	}
	defer rows.Close()

	var resultado []domain.ResumenPorCategoriaDeFalla
	for rows.Next() {
		var x domain.ResumenPorCategoriaDeFalla
		if err := rows.Scan(&x.Categoria, &x.Total, &x.Abiertas, &x.EquiposAlcanzados); err != nil {
			return nil, fmt.Errorf("escaneando incidencias por categoría: %w", err)
		}
		resultado = append(resultado, x)
	}
	return resultado, errorDeFilas(rows)
}
