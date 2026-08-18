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
		// Un id vacío o con cualquier cosa adentro es un pedido mal armado, no
		// una falla del servidor: Postgres lo rechaza con 22P02 y sin esta
		// traducción el error salía envuelto, mapearError no lo reconocía y
		// asignar un docente respondía 500 "error interno" en vez de decir que
		// el id no tiene formato válido. Mismo criterio que el resto del
		// paquete (ver esIDInvalido en postgres_repo.go).
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando usuario: %w", err)
	}
	return estado == "APROBADA", nil
}
