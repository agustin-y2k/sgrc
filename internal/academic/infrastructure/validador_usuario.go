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
// importa internal/auth (ni su domain ni su application), porque eso violaría
// el límite de dominio entre paquetes (ver docs/06-arquitectura.md §3).
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

// AlgunoAprobado contesta si queda al menos uno de esos usuarios con la cuenta
// APROBADA, en una sola consulta.
//
// Es un EXISTS y no un conteo: la pregunta es «¿queda alguno?», y contar
// obligaría a Postgres a recorrer todas las filas cuando con la primera
// coincidencia ya alcanza.
func (v *ValidadorUsuarioPostgres) AlgunoAprobado(ctx context.Context, usuarioIDs []string) (bool, error) {
	if len(usuarioIDs) == 0 {
		return false, nil
	}

	var alguno bool
	err := v.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM usuario WHERE id = ANY($1) AND estado = 'APROBADA')`,
		usuarioIDs).Scan(&alguno)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando si queda algún usuario aprobado: %w", err)
	}
	return alguno, nil
}
