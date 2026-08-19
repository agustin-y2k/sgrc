package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/notification/application"
	"github.com/ramiro/sgrc/internal/notification/domain"
)

var _ application.PreferenciasEmail = (*PreferenciasEmailPostgres)(nil)

// PreferenciasEmailPostgres guarda a qué copias por correo se suscribió cada
// Admin (RF-05.13). La tabla solo tiene las encendidas.
type PreferenciasEmailPostgres struct {
	pool *pgxpool.Pool
}

func NewPreferenciasEmailPostgres(pool *pgxpool.Pool) *PreferenciasEmailPostgres {
	return &PreferenciasEmailPostgres{pool: pool}
}

func (p *PreferenciasEmailPostgres) ElegidasDe(ctx context.Context, usuarioID string) (map[domain.CategoriaEmail]bool, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT categoria, activa FROM preferencia_email WHERE usuario_id = $1`, usuarioID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("leyendo preferencias de correo: %w", err)
	}
	defer rows.Close()

	elegidas := map[domain.CategoriaEmail]bool{}
	for rows.Next() {
		var c string
		var activa bool
		if err := rows.Scan(&c, &activa); err != nil {
			return nil, fmt.Errorf("escaneando preferencia de correo: %w", err)
		}
		categoria, err := domain.ParseCategoriaEmail(c)
		if err != nil {
			// Una categoría que la aplicación ya no conoce se ignora en vez de
			// romper la pantalla: es una fila vieja, no un error de quien mira.
			continue
		}
		elegidas[categoria] = activa
	}
	return elegidas, errorDeFilas(rows)
}

// Reemplazar escribe una fila por categoría —encendida o apagada— en una sola
// transacción: entre el DELETE y los INSERT no puede quedar una ventana en la
// que el barrido lea otra cosa. Se guardan también las apagadas porque
// destildar una categoría que arranca encendida tiene que poder distinguirse
// de no haber elegido nunca.
func (p *PreferenciasEmailPostgres) Reemplazar(ctx context.Context, usuarioID string, decisiones map[domain.CategoriaEmail]bool) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciando transacción: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op si ya hizo Commit

	if _, err := tx.Exec(ctx, `DELETE FROM preferencia_email WHERE usuario_id = $1`, usuarioID); err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("borrando preferencias de correo: %w", err)
	}

	// En el orden canónico y no el del mapa: dos guardadas iguales tienen que
	// producir el mismo INSERT, aunque más no sea para leer un log.
	for _, c := range domain.CategoriasDeEmail() {
		activa, decidio := decisiones[c]
		if !decidio {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO preferencia_email (usuario_id, categoria, activa) VALUES ($1, $2, $3)`,
			usuarioID, string(c), activa,
		); err != nil {
			if esIDInvalido(err) {
				return application.ErrIDInvalido
			}
			return fmt.Errorf("guardando la preferencia %s: %w", c, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmando preferencias de correo: %w", err)
	}
	return nil
}

// RecibePorEmail mira la preferencia de quien tenga esa dirección. Si no hay
// cuenta con ese email —pasa con las entregas a nombre de alguien que no está
// en el sistema— no hay nada elegido y vale el valor por defecto.
func (p *PreferenciasEmailPostgres) RecibePorEmail(ctx context.Context, email string, categoria domain.CategoriaEmail) (bool, error) {
	var activa bool
	err := p.pool.QueryRow(ctx, `
		SELECT COALESCE(pref.activa, $3)
		FROM usuario u
		LEFT JOIN preferencia_email pref
		       ON pref.usuario_id = u.id AND pref.categoria = $2
		WHERE u.email = $1
	`, email, string(categoria), categoria.ActivaPorDefecto()).Scan(&activa)
	if errors.Is(err, pgx.ErrNoRows) {
		return categoria.ActivaPorDefecto(), nil
	}
	if err != nil {
		return false, fmt.Errorf("consultando la preferencia de %s: %w", categoria, err)
	}
	return activa, nil
}
