package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/reservation/application"
)

// ── ValidadorMateria (puerto hacia academic) ───────────────────────────

var _ application.ValidadorMateria = (*ValidadorMateriaPostgres)(nil)

// ValidadorMateriaPostgres consulta docente_materia directamente — a
// propósito NO importa internal/academic (ver docs/06-arquitectura.md §3,
// mismo criterio que ValidadorUsuarioPostgres de academic hacia auth).
type ValidadorMateriaPostgres struct {
	pool *pgxpool.Pool
}

func NewValidadorMateriaPostgres(pool *pgxpool.Pool) *ValidadorMateriaPostgres {
	return &ValidadorMateriaPostgres{pool: pool}
}

func (v *ValidadorMateriaPostgres) DocenteEstaAsignado(ctx context.Context, materiaID, usuarioID string) (bool, error) {
	var existe bool
	err := v.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM docente_materia WHERE materia_id = $1 AND usuario_id = $2)`,
		materiaID, usuarioID,
	).Scan(&existe)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando asignación docente-materia: %w", err)
	}
	return existe, nil
}

// ── ValidadorEquipo (puerto hacia inventory) ────────────────────────────

var _ application.ValidadorEquipo = (*ValidadorEquipoPostgres)(nil)

// ValidadorEquipoPostgres consulta la tabla pc directamente — a propósito NO
// importa internal/inventory.
type ValidadorEquipoPostgres struct {
	pool *pgxpool.Pool
}

func NewValidadorEquipoPostgres(pool *pgxpool.Pool) *ValidadorEquipoPostgres {
	return &ValidadorEquipoPostgres{pool: pool}
}

func (v *ValidadorEquipoPostgres) EquipoDisponibleParaReservar(ctx context.Context, equipoID string) (bool, error) {
	var estado string
	var dadoDeBaja, reservable bool
	err := v.pool.QueryRow(ctx,
		`SELECT estado, dado_de_baja, reservable FROM equipo WHERE id = $1`, equipoID,
	).Scan(&estado, &dadoDeBaja, &reservable)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // no existe → no disponible, pero no es un error
		}
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando disponibilidad del equipo: %w", err)
	}
	// `reservable` es la mitad que agrega la 015: la lista de disponibles ya
	// lo filtra, pero un pedido armado a mano no pasa por esa lista, y sin
	// este chequeo se podría reservar un cargador igual.
	return estado == "DISPONIBLE" && !dadoDeBaja && reservable, nil
}

// EquipoEstaEnInventario: existe y no está dada de baja, sin mirar el estado.
// Ver el comentario del puerto: entregar no es lo mismo que reservar.
func (v *ValidadorEquipoPostgres) EquipoEstaEnInventario(ctx context.Context, equipoID string) (bool, error) {
	var dadoDeBaja bool
	err := v.pool.QueryRow(ctx,
		`SELECT dado_de_baja FROM equipo WHERE id = $1`, equipoID,
	).Scan(&dadoDeBaja)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando si el equipo está en el inventario: %w", err)
	}
	return !dadoDeBaja, nil
}

// EtiquetasDeEquipos: cómo se nombra cada equipo, para los avisos de
// cancelación. Una sola consulta con = ANY en vez de una por PC — un
// bloqueo por evaluación sobre un carro entero puede tocar treinta.
func (v *ValidadorEquipoPostgres) EtiquetasDeEquipos(ctx context.Context, equipoIDs []string) (map[string]string, error) {
	etiquetas := make(map[string]string, len(equipoIDs))
	if len(equipoIDs) == 0 {
		return etiquetas, nil
	}

	// COALESCE en este orden: el nombre manda cuando existe (un proyector),
	// y si no, el número. Es la misma regla que domain.Equipo.Etiqueta, resuelta
	// en SQL para no traer la fila entera solo por el rótulo.
	rows, err := v.pool.Query(ctx,
		`SELECT id, COALESCE(nombre, 'PC ' || identificador) FROM equipo WHERE id = ANY($1)`, equipoIDs)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("resolviendo las etiquetas de los equipos: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, etiqueta string
		if err := rows.Scan(&id, &etiqueta); err != nil {
			return nil, fmt.Errorf("escaneando la etiqueta de un equipo: %w", err)
		}
		etiquetas[id] = etiqueta
	}
	return etiquetas, rows.Err()
}

// ── ObtenedorNombreDocente (puerto hacia auth) ──────────────────────────

var _ application.ObtenedorNombreDocente = (*ObtenedorNombrePostgres)(nil)

// ObtenedorNombrePostgres consulta la tabla usuario directamente — a
// propósito NO importa internal/auth.
type ObtenedorNombrePostgres struct {
	pool *pgxpool.Pool
}

func NewObtenedorNombrePostgres(pool *pgxpool.Pool) *ObtenedorNombrePostgres {
	return &ObtenedorNombrePostgres{pool: pool}
}

func (o *ObtenedorNombrePostgres) NombreCompletoDe(ctx context.Context, usuarioID string) (string, error) {
	var nombre, apellido string
	err := o.pool.QueryRow(ctx,
		`SELECT nombre, apellido FROM usuario WHERE id = $1`, usuarioID,
	).Scan(&nombre, &apellido)
	if err != nil {
		if esIDInvalido(err) {
			return "", application.ErrIDInvalido
		}
		return "", fmt.Errorf("obteniendo nombre del usuario: %w", err)
	}
	return nombre + " " + apellido, nil
}

// MateriaAceptaReservas implementa la mitad "no archivada" de RF-04.1. Se
// mira también el curso y el ciclo: archivar un ciclo marca los tres
// niveles (ver ArchivarCiclo en academic/infrastructure), pero basta con
// que cualquiera de ellos esté archivado para que la materia ya no admita
// reservas nuevas.
func (v *ValidadorMateriaPostgres) MateriaAceptaReservas(ctx context.Context, materiaID string) (bool, error) {
	var acepta bool
	err := v.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM materia m
			JOIN curso c ON c.id = m.curso_id
			JOIN ciclo_lectivo cl ON cl.id = c.ciclo_lectivo_id
			WHERE m.id = $1
			  AND m.archivado = false
			  AND c.archivado = false
			  AND cl.archivado = false
		)
	`, materiaID).Scan(&acepta)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando si la materia acepta reservas: %w", err)
	}
	return acepta, nil
}
