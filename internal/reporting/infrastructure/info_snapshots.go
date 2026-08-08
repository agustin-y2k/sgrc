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

// El JOIN a carro es LEFT desde la 015: un proyector no está en ninguno, y
// con INNER esta consulta devolvía "no encontrada" y abortaba el archivado
// del ciclo entero.
func (i *InfoPCPostgres) EtiquetaYCarroDe(ctx context.Context, pcID string) (string, int, string, error) {
	var etiqueta, carroNombre string
	var identificador int
	err := i.pool.QueryRow(ctx, `
		SELECT COALESCE(p.nombre, 'PC ' || p.identificador),
		       COALESCE(p.identificador, 0), COALESCE(c.nombre, '')
		FROM pc p
		LEFT JOIN carro c ON c.id = p.carro_id
		WHERE p.id = $1
	`, pcID).Scan(&etiqueta, &identificador, &carroNombre)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, "", fmt.Errorf("PC %s no encontrada", pcID)
		}
		if esIDInvalido(err) {
			return "", 0, "", application.ErrIDInvalido
		}
		return "", 0, "", fmt.Errorf("obteniendo etiqueta/carro de la PC: %w", err)
	}
	return etiqueta, identificador, carroNombre, nil
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
