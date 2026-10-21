package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/agustin-y2k/sgrc/internal/auth/application"
	"github.com/agustin-y2k/sgrc/internal/auth/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
)

// Persistencia de los códigos de recuperación de contraseña
// (migrations/001_esquema_inicial.sql).

const columnasCodigo = `id, usuario_id, codigo_hash, creado_en, expira_en, usado_en, intentos`

// CrearCodigoRecuperacion invalida los códigos anteriores de esa persona y
// guarda el nuevo.
func (r *PostgresRepo) CrearCodigoRecuperacion(ctx context.Context, c *domain.CodigoRecuperacion) error {
	err := r.EnRecuperacion(ctx, c.UsuarioID, func(ctx context.Context, _ application.Repo) error { return r.crearCodigoRecuperacion(ctx, c) })
	if errors.Is(err, application.ErrUsuarioNoEncontrado) {
		return application.ErrReferenciaInexistente
	}
	return err
}
func (r *PostgresRepo) crearCodigoRecuperacion(ctx context.Context, c *domain.CodigoRecuperacion) error {

	if _, err := r.conexion(ctx).Exec(ctx, `UPDATE codigo_recuperacion SET usado_en=$2 WHERE usuario_id=$1 AND usado_en IS NULL`, c.UsuarioID, c.CreadoEn); err != nil {
		return err
	}
	_, err := r.conexion(ctx).Exec(ctx, `INSERT INTO codigo_recuperacion(id,usuario_id,codigo_hash,creado_en,expira_en,intentos) VALUES($1,$2,$3,$4,$5,$6)`, c.ID, c.UsuarioID, c.CodigoHash, c.CreadoEn, c.ExpiraEn, c.Intentos)

	if err != nil {
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("insertando código de recuperación: %w", err)
	}
	return nil
}

// BuscarCodigoVigenteDe devuelve el último código sin usar de esa persona.
func (r *PostgresRepo) BuscarCodigoVigenteDe(ctx context.Context, usuarioID string) (*domain.CodigoRecuperacion, error) {
	row := r.conexion(ctx).QueryRow(ctx, `
		SELECT `+columnasCodigo+`
		FROM codigo_recuperacion
		WHERE usuario_id = $1 AND usado_en IS NULL
		ORDER BY creado_en DESC
		LIMIT 1
	`, usuarioID)

	var c domain.CodigoRecuperacion
	err := row.Scan(&c.ID, &c.UsuarioID, &c.CodigoHash, &c.CreadoEn, &c.ExpiraEn, &c.UsadoEn, &c.Intentos)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrCodigoNoEncontrado
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando código de recuperación: %w", err)
	}
	return &c, nil
}

// GuardarCodigoRecuperacion persiste el intento fallido o el consumo.
func (r *PostgresRepo) GuardarCodigoRecuperacion(ctx context.Context, c *domain.CodigoRecuperacion) error {
	tag, err := r.conexion(ctx).Exec(ctx, `
		UPDATE codigo_recuperacion SET usado_en = $2, intentos = $3
		WHERE id = $1 AND usado_en IS NULL
	`, c.ID, c.UsadoEn, c.Intentos)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando código de recuperación: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrCodigoNoEncontrado
	}
	return nil
}

func (r *PostgresRepo) EnRecuperacion(ctx context.Context, id string, fn func(context.Context, application.Repo) error) error {
	if r.pool == nil {
		if err := r.bloquearRecuperacion(ctx, id); err != nil {
			return err
		}
		return fn(ctx, r)
	}
	return pgtransaccion.Ejecutar(ctx, r.pool, func(ctx context.Context) error {
		tx := pgtransaccion.DeContexto(ctx)
		bound := &PostgresRepo{db: tx}
		if err := bound.bloquearRecuperacion(ctx, id); err != nil {
			return err
		}
		return fn(ctx, &PostgresRepo{db: tx})
	})
}

func (r *PostgresRepo) bloquearRecuperacion(ctx context.Context, id string) error {
	var encontrado string
	err := r.conexion(ctx).QueryRow(ctx, `SELECT id FROM usuario WHERE id=$1 FOR UPDATE`, id).Scan(&encontrado)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrUsuarioNoEncontrado
	}
	if esIDInvalido(err) {
		return application.ErrIDInvalido
	}
	return err
}
