package infrastructure

import "github.com/google/uuid"

// NuevoID genera un UUID nuevo — mismo patrón que auth/academic.
func NuevoID() string {
	return uuid.NewString()
}
