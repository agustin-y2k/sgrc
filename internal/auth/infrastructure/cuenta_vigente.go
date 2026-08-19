package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/middleware"
)

// VerificadorCuentaVigente resuelve middleware.CuentaVigente contra la tabla
// usuario: en cada request autenticado dice si esa cuenta sigue habilitada
// para operar y con qué rol.
type VerificadorCuentaVigente struct {
	pool *pgxpool.Pool
}

func NewVerificadorCuentaVigente(pool *pgxpool.Pool) *VerificadorCuentaVigente {
	return &VerificadorCuentaVigente{pool: pool}
}

// Vigente cumple la firma de middleware.CuentaVigente — se pasa como
// v.Vigente directamente al armar la Autenticacion.
func (v *VerificadorCuentaVigente) Vigente(ctx context.Context, usuarioID string) (middleware.EstadoDeCuenta, error) {
	var rolStr, estadoStr string
	var versionSesion int
	err := v.pool.QueryRow(ctx,
		`SELECT rol, estado, version_sesion FROM usuario WHERE id = $1`, usuarioID,
	).Scan(&rolStr, &estadoStr, &versionSesion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || esIDInvalido(err) {
			return middleware.EstadoDeCuenta{}, nil
		}
		return middleware.EstadoDeCuenta{}, fmt.Errorf("verificando cuenta vigente: %w", err)
	}

	// Un rol o estado que no parsea es dato corrupto en la base, no una
	// cuenta habilitada: se niega el acceso en vez de asumir un default.
	rol, err := domain.ParseRol(rolStr)
	if err != nil {
		return middleware.EstadoDeCuenta{}, fmt.Errorf("rol inválido en la base para usuario %s: %w", usuarioID, err)
	}
	estado, err := domain.ParseEstado(estadoStr)
	if err != nil {
		return middleware.EstadoDeCuenta{}, fmt.Errorf("estado inválido en la base para usuario %s: %w", usuarioID, err)
	}

	return middleware.EstadoDeCuenta{
		Vigente:       estado == domain.EstadoAprobada,
		Rol:           string(rol),
		VersionSesion: versionSesion,
	}, nil
}
