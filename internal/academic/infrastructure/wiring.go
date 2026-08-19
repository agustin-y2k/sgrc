package infrastructure

import "github.com/google/uuid"

// uuidNuevo se usa internamente en este paquete (ej.
func uuidNuevo() string {
	return uuid.NewString()
}

// NuevoID es el generador de IDs inyectado en application.Service (mismo
// patrón que auth.NuevoID).
func NuevoID() string {
	return uuid.NewString()
}
