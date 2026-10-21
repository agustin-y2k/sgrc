package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/agustin-y2k/sgrc/internal/auth/application"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
)

func (r *PostgresRepo) EnBaja(ctx context.Context, id string, fn func(context.Context, application.Repo) error) error {
	if r.pool == nil {
		return fmt.Errorf("la baja debe iniciarse desde el repositorio principal")
	}
	// Antes del bloqueo por usuario y de cualquier fila: mismo orden que
	// asignaciones, inventario y reservas. Cada etapa conserva su propio commit.
	return pgtransaccion.Coordinar(ctx, r.pool, true, func(ctx context.Context) error {
		tx := pgtransaccion.DeContexto(ctx)
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "baja_usuario:"+id); err != nil {
			return err
		}
		return fn(ctx, &PostgresRepo{db: tx})
	})
}

func (r *PostgresRepo) BuscarBaja(ctx context.Context, id string) (*application.ProcesoBaja, error) {
	var p application.ProcesoBaja
	err := r.conexion(ctx).QueryRow(ctx, `SELECT completada_en IS NOT NULL FROM baja_usuario WHERE usuario_id=$1`, id).Scan(&p.Completada)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if esIDInvalido(err) {
		return nil, application.ErrIDInvalido
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.conexion(ctx).Query(ctx, `SELECT materia_id FROM baja_usuario_materia WHERE usuario_id=$1 AND completada_en IS NULL ORDER BY materia_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		p.MateriasPendientes = append(p.MateriasPendientes, id)
	}
	return &p, errorDeFilas(rows)
}
func (r *PostgresRepo) PrepararBaja(ctx context.Context, id string, materias []string, ahora time.Time) error {
	if _, err := r.conexion(ctx).Exec(ctx, `INSERT INTO baja_usuario(usuario_id,preparada_en) VALUES($1,$2)`, id, ahora); err != nil {
		return err
	}
	if len(materias) == 0 {
		return nil
	}
	_, err := r.conexion(ctx).Exec(ctx, `INSERT INTO baja_usuario_materia(usuario_id,materia_id) SELECT $1,unnest($2::uuid[]) ON CONFLICT DO NOTHING`, id, materias)
	return err
}
func (r *PostgresRepo) CompletarMateriaDeBaja(ctx context.Context, usuarioID, materiaID string, ahora time.Time) error {
	tag, err := r.conexion(ctx).Exec(ctx, `UPDATE baja_usuario_materia SET completada_en=$3 WHERE usuario_id=$1 AND materia_id=$2 AND completada_en IS NULL`, usuarioID, materiaID, ahora)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return application.ErrBajaPendiente
	}
	return nil
}
func (r *PostgresRepo) CompletarBaja(ctx context.Context, id string, ahora time.Time) error {
	tag, err := r.conexion(ctx).Exec(ctx, `UPDATE baja_usuario SET completada_en=$2 WHERE usuario_id=$1 AND completada_en IS NULL AND NOT EXISTS(SELECT 1 FROM baja_usuario_materia WHERE usuario_id=$1 AND completada_en IS NULL)`, id, ahora)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return application.ErrBajaPendiente
	}
	return nil
}
