package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/auth/domain"
)

// VerificadorCuentaVigente resuelve middleware.CuentaVigente contra la tabla
// usuario: en cada request autenticado dice si esa cuenta sigue habilitada
// para operar y con qué rol.
//
// Vive acá y no en application/ porque no tiene ninguna regla de negocio
// propia — es la misma lectura por PK que hace BuscarPorID, reducida a las
// dos columnas que el middleware necesita. Se inyecta desde cmd/main.go
// porque internal/shared/middleware no puede importar internal/auth (ver
// docs/06-arquitectura.md §3).
type VerificadorCuentaVigente struct {
	pool *pgxpool.Pool
}

func NewVerificadorCuentaVigente(pool *pgxpool.Pool) *VerificadorCuentaVigente {
	return &VerificadorCuentaVigente{pool: pool}
}

// Vigente cumple la firma de middleware.CuentaVigente — se pasa como
// v.Vigente directamente al armar la Autenticacion.
//
// Los tres casos de "no vigente" se responden igual (false, sin error) a
// propósito: la cuenta no existe (RF-01.9 la eliminó), el ID del token no
// es un UUID (token viejo de otra base, o manipulado), o el estado no es
// APROBADA (PENDIENTE / RECHAZADA / BAJA). Ninguno es una falla del
// sistema; los tres significan lo mismo para quien pregunta.
func (v *VerificadorCuentaVigente) Vigente(ctx context.Context, usuarioID string) (bool, string, error) {
	var rolStr, estadoStr string
	err := v.pool.QueryRow(ctx,
		`SELECT rol, estado FROM usuario WHERE id = $1`, usuarioID,
	).Scan(&rolStr, &estadoStr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || esIDInvalido(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("verificando cuenta vigente: %w", err)
	}

	// Un rol o estado que no parsea es dato corrupto en la base, no una
	// cuenta habilitada: se niega el acceso en vez de asumir un default.
	rol, err := domain.ParseRol(rolStr)
	if err != nil {
		return false, "", fmt.Errorf("rol inválido en la base para usuario %s: %w", usuarioID, err)
	}
	estado, err := domain.ParseEstado(estadoStr)
	if err != nil {
		return false, "", fmt.Errorf("estado inválido en la base para usuario %s: %w", usuarioID, err)
	}

	return estado == domain.EstadoAprobada, string(rol), nil
}
