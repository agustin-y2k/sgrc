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

// ── ValidadorPC (puerto hacia inventory) ────────────────────────────────

var _ application.ValidadorPC = (*ValidadorPCPostgres)(nil)

// ValidadorPCPostgres consulta la tabla pc directamente — a propósito NO
// importa internal/inventory.
type ValidadorPCPostgres struct {
	pool *pgxpool.Pool
}

func NewValidadorPCPostgres(pool *pgxpool.Pool) *ValidadorPCPostgres {
	return &ValidadorPCPostgres{pool: pool}
}

func (v *ValidadorPCPostgres) PCDisponibleParaReservar(ctx context.Context, pcID string) (bool, error) {
	var estado string
	var dadaDeBaja bool
	err := v.pool.QueryRow(ctx,
		`SELECT estado, dada_de_baja FROM pc WHERE id = $1`, pcID,
	).Scan(&estado, &dadaDeBaja)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // no existe → no disponible, pero no es un error
		}
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando disponibilidad de PC: %w", err)
	}
	return estado == "DISPONIBLE" && !dadaDeBaja, nil
}

// IdentificadoresDePCs: el número visible de cada PC, para los avisos de
// cancelación. Una sola consulta con = ANY en vez de una por PC — un
// bloqueo por evaluación sobre un carro entero puede tocar treinta.
func (v *ValidadorPCPostgres) IdentificadoresDePCs(ctx context.Context, pcIDs []string) (map[string]int, error) {
	identificadores := make(map[string]int, len(pcIDs))
	if len(pcIDs) == 0 {
		return identificadores, nil
	}

	rows, err := v.pool.Query(ctx, `SELECT id, identificador FROM pc WHERE id = ANY($1)`, pcIDs)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("resolviendo identificadores de PC: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var identificador int
		if err := rows.Scan(&id, &identificador); err != nil {
			return nil, fmt.Errorf("escaneando identificador de PC: %w", err)
		}
		identificadores[id] = identificador
	}
	return identificadores, rows.Err()
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
