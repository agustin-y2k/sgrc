// Package adminseed contiene la lógica de decisión de "sembrar el primer
// Admin si hace falta" (RF-01.4), aislada a propósito sin ninguna dependencia
// externa (ni pgx, ni argon2) — solo stdlib.
package adminseed

import (
	"context"
	"errors"
	"fmt"
)

// ErrEnvFaltante: SEED_ADMIN_EMAIL o SEED_ADMIN_PASSWORD no están seteados.
var ErrEnvFaltante = errors.New("SEED_ADMIN_EMAIL / SEED_ADMIN_PASSWORD no están configurados")

// ErrPasswordCorta: la contraseña inicial no cumple el mínimo de seguridad.
var ErrPasswordCorta = errors.New("la contraseña del admin inicial debe tener al menos 8 caracteres")

const minPasswordLen = 8

// Repo es el único contrato que necesita este paquete — cmd/seed_admin.go
// lo implementa contra Postgres real.
type Repo interface {
	// ExisteAdminActivo: si hay al menos un ADMIN que pueda entrar hoy.
	ExisteAdminActivo(ctx context.Context) (bool, error)

	// CrearAdmin siembra el Admin inicial. Solo se llama cuando no hay
	// ninguno activo.
	CrearAdmin(ctx context.Context, email, passwordHash string) error
}

// HashFunc hashea una contraseña en texto plano.
type HashFunc func(password string) (string, error)

// SembrarSiHaceFalta es la lógica completa de RF-01.4, sin tocar Postgres ni
// argon2 directamente — todo eso llega inyectado.
func SembrarSiHaceFalta(ctx context.Context, repo Repo, hash HashFunc, email, password string) error {
	existe, err := repo.ExisteAdminActivo(ctx)
	if err != nil {
		return fmt.Errorf("verificando si ya existe un admin activo: %w", err)
	}
	if existe {
		return nil
	}

	if email == "" || password == "" {
		return ErrEnvFaltante
	}
	if len(password) < minPasswordLen {
		return ErrPasswordCorta
	}

	hashed, err := hash(password)
	if err != nil {
		return fmt.Errorf("hasheando password del admin inicial: %w", err)
	}

	if err := repo.CrearAdmin(ctx, email, hashed); err != nil {
		return fmt.Errorf("creando admin inicial: %w", err)
	}
	return nil
}
