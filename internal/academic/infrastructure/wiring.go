package infrastructure

import "github.com/google/uuid"

// uuidNuevo se usa internamente en este paquete (ej. al clonar cursos y
// materias, donde se generan varios IDs dentro de la misma transacción,
// sin pasar por application.IDGenerator). NuevoID es la versión pública
// para wiring desde cmd/main.go.
func uuidNuevo() string {
	return uuid.NewString()
}

// NuevoID es el generador de IDs inyectado en application.Service (mismo
// patrón que auth.NuevoID).
func NuevoID() string {
	return uuid.NewString()
}
