package main

import (
	"context"
	"testing"
)

// TestPgxUsuarioRepo_ImplementaLaInterfaz confirma en tiempo de compilación
// que pgxUsuarioRepo cumple el contrato adminseed.Repo.
func TestPgxUsuarioRepo_ImplementaLaInterfaz(t *testing.T) {
	var _ interface {
		ExisteAdminActivo(ctx context.Context) (bool, error)
		CrearAdmin(ctx context.Context, email, passwordHash string) error
	} = (*pgxUsuarioRepo)(nil)
}
