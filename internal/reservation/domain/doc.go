// Package domain contiene las entidades y reglas de negocio puras de
// reservation — sin dependencias de infraestructura (sin *sql.DB, sin fiber.Ctx).
// ReservaGrupo + Reserva, solapamiento, recurrencia, bloqueo por evaluación, job de vencimiento. RF-04.
// Ver docs/03-diagrama-clases.md para las entidades y docs/01-requisitos.md
// para el detalle funcional.
package domain
