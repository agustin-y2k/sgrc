package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/auth/application"
	"github.com/ramiro/sgrc/internal/auth/domain"
)

// Persistencia de los códigos de recuperación de contraseña
// (migrations/001_esquema_inicial.sql).

const columnasCodigo = `id, usuario_id, codigo_hash, creado_en, expira_en, usado_en, intentos`

// CrearCodigoRecuperacion invalida los códigos anteriores de esa persona y
// guarda el nuevo.
func (r *PostgresRepo) CrearCodigoRecuperacion(ctx context.Context, c *domain.CodigoRecuperacion) error {
	_, err := r.db.Exec(ctx, `
		WITH anteriores AS (
			UPDATE codigo_recuperacion SET usado_en = $4
			WHERE usuario_id = $2 AND usado_en IS NULL
		)
		INSERT INTO codigo_recuperacion (id, usuario_id, codigo_hash, creado_en, expira_en, intentos)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, c.ID, c.UsuarioID, c.CodigoHash, c.CreadoEn, c.ExpiraEn, c.Intentos)

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
	row := r.db.QueryRow(ctx, `
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
	tag, err := r.db.Exec(ctx, `
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
