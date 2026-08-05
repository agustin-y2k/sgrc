package main

import (
	"context"
	"testing"
)

// TestPgxUsuarioRepo_ImplementaLaInterfaz confirma en tiempo de compilación
// que pgxUsuarioRepo cumple el contrato adminseed.Repo. No ejercita
// Postgres real (para eso hace falta un entorno con Docker/testcontainers,
// ver docs/10-testing.md) — la lógica de decisión ya está probada
// sin necesitar una base en internal/shared/adminseed, y el hash/verify en
// internal/shared/security.
func TestPgxUsuarioRepo_ImplementaLaInterfaz(t *testing.T) {
	var _ interface {
		ExisteAdminActivo(ctx context.Context) (bool, error)
		CrearAdmin(ctx context.Context, email, passwordHash string) error
	} = (*pgxUsuarioRepo)(nil)
}
