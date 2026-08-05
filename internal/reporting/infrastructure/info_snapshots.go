package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/reporting/application"
)

var _ application.InfoPCParaSnapshot = (*InfoPCPostgres)(nil)

// InfoPCPostgres implementa el puerto hacia inventory — consulta pc/carro
// directamente, sin importar internal/inventory.
type InfoPCPostgres struct {
	pool *pgxpool.Pool
}

func NewInfoPCPostgres(pool *pgxpool.Pool) *InfoPCPostgres {
	return &InfoPCPostgres{pool: pool}
}

func (i *InfoPCPostgres) IdentificadorYCarroDe(ctx context.Context, pcID string) (int, string, error) {
	var identificador int
	var carroNombre string
	err := i.pool.QueryRow(ctx, `
		SELECT p.identificador, c.nombre FROM pc p JOIN carro c ON c.id = p.carro_id WHERE p.id = $1
	`, pcID).Scan(&identificador, &carroNombre)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", fmt.Errorf("PC %s no encontrada", pcID)
		}
		if esIDInvalido(err) {
			return 0, "", application.ErrIDInvalido
		}
		return 0, "", fmt.Errorf("obteniendo identificador/carro de la PC: %w", err)
	}
	return identificador, carroNombre, nil
}

var _ application.InfoUsuarioParaSnapshot = (*InfoUsuarioPostgres)(nil)

// InfoUsuarioPostgres implementa el puerto hacia auth — consulta usuario
// directamente, sin importar internal/auth.
type InfoUsuarioPostgres struct {
	pool *pgxpool.Pool
}

func NewInfoUsuarioPostgres(pool *pgxpool.Pool) *InfoUsuarioPostgres {
	return &InfoUsuarioPostgres{pool: pool}
}

func (i *InfoUsuarioPostgres) NombreCompletoDe(ctx context.Context, usuarioID string) (string, error) {
	var nombre, apellido string
	err := i.pool.QueryRow(ctx,
		`SELECT nombre, apellido FROM usuario WHERE id = $1`, usuarioID,
	).Scan(&nombre, &apellido)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("usuario %s no encontrado", usuarioID)
		}
		if esIDInvalido(err) {
			return "", application.ErrIDInvalido
		}
		return "", fmt.Errorf("obteniendo nombre del usuario: %w", err)
	}
	return nombre + " " + apellido, nil
}
