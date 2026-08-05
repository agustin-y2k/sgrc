package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/academic/application"
)

var _ application.ValidadorUsuario = (*ValidadorUsuarioPostgres)(nil)

// ValidadorUsuarioPostgres implementa el puerto application.ValidadorUsuario
// con una query mínima y directa a la tabla `usuario` — a propósito NO
// importa internal/auth (ni su domain ni su application), porque eso
// violaría el límite de dominio entre paquetes (ver docs/06-arquitectura.md
// §3). Es la misma tabla física, pero academic solo necesita saber una
// cosa (¿existe y está APROBADA?), no el resto de las reglas de negocio
// de auth — así que una columna leída acá es más simple y más aislado que
// llamar a través de application.Service de auth.
type ValidadorUsuarioPostgres struct {
	pool *pgxpool.Pool
}

func NewValidadorUsuarioPostgres(pool *pgxpool.Pool) *ValidadorUsuarioPostgres {
	return &ValidadorUsuarioPostgres{pool: pool}
}

func (v *ValidadorUsuarioPostgres) ExisteYAprobado(ctx context.Context, usuarioID string) (bool, error) {
	var estado string
	err := v.pool.QueryRow(ctx, `SELECT estado FROM usuario WHERE id = $1`, usuarioID).Scan(&estado)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // no existe → no es válido, pero no es un error
		}
		return false, fmt.Errorf("verificando usuario: %w", err)
	}
	return estado == "APROBADA", nil
}
