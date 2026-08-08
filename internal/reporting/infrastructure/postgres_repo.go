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

func (r *PostgresRepo) ListarHistoricoUsoEquipoPorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoEquipo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, anio, equipo_id, etiqueta_snapshot,
		       COALESCE(identificador_snapshot, 0), COALESCE(carro_nombre_snapshot, ''),
		       minutos_reservados, cantidad_reservas
		FROM historico_uso_equipo WHERE anio = $1
		-- Las PCs de carro primero y por número; los equipos sueltos después,
		-- juntos y por nombre. equipo_id cierra el orden: el identificador se
		-- repite entre carros y es NULL en los sueltos (015).
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
		-- LEFT desde la 015: un proyector reservable no está en ningún carro,
		-- y con INNER JOIN sus reservas no figuraban en el reporte de uso —
		-- el equipo más peleado de la escuela podía aparecer como sin usar.
		LEFT JOIN carro ca ON ca.id = p.carro_id
		WHERE (
			r.materia_id IN (
				SELECT m.id FROM materia m JOIN curso c ON c.id = m.curso_id WHERE c.ciclo_lectivo_id = $1
			)
			-- Los bloqueos por evaluación estatal (RF-04.7) no tienen materia,
			-- así que el filtro de arriba no los alcanza. Igual ocupan la PC:
			-- sin esto, una máquina muy usada para exámenes figura como poco
			-- usada. Se los ata al ciclo por el año de la fecha, que es lo
			-- único que los relaciona con un ciclo lectivo.
			OR (
				r.tipo = 'EVALUACION_ESTATAL'
				AND EXTRACT(YEAR FROM r.fecha) = (SELECT anio FROM ciclo_lectivo WHERE id = $1)
			)
		)
		AND r.estado != 'CANCELADA'`+condFechasPrefijo("r", condFechas)+`
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
		-- entre carros y es NULL en los equipos sueltos (015), así que
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

	rows, err := r.pool.Query(ctx, `
		SELECT r.creado_por, MAX(u.nombre || ' ' || u.apellido) AS docente,
		       COUNT(*), COALESCE(SUM(`+expresionMinutosDe("r")+`), 0) AS minutos
		FROM reserva r
		JOIN usuario u ON u.id = r.creado_por
		WHERE r.materia_id IN (
			SELECT m.id FROM materia m JOIN curso c ON c.id = m.curso_id WHERE c.ciclo_lectivo_id = $1
		)
		AND r.estado != 'CANCELADA'
		AND r.creado_por IS NOT NULL`+condFechasPrefijo("r", condFechas)+`
		GROUP BY r.creado_por
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
		       COUNT(*) FILTER (WHERE i.estado = 'ENVIADA_DGE'),
		       COUNT(*) FILTER (WHERE i.estado = 'RESUELTA'),
		       COUNT(*) FILTER (WHERE i.gravedad = 'GRAVE')
		FROM incidencia i
		JOIN equipo p ON p.id = i.equipo_id
		-- LEFT desde la 015: al proyector también se le rompe la lámpara, y
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
			&x.Total, &x.Abiertas, &x.EnReparacion, &x.EnviadasDGE, &x.Resueltas, &x.Graves); err != nil {
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
