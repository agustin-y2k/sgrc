package infrastructure

import "github.com/google/uuid"

// NuevoID genera un UUID nuevo — mismo patrón que los demás paquetes.
func NuevoID() string {
	return uuid.NewString()
}
