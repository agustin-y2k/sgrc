import { apiFetch } from "@/lib/api-client"
import type { CalendarioEquipo } from "@/features/calendario/types"

// RF-04.4 — los bloques ocupados de un equipo en un rango de fechas, con
// docente y materia ya resueltos por el backend.
export function calendarioDeEquipo(equipoId: string, desde: string, hasta: string) {
  const params = new URLSearchParams({ desde, hasta })
  return apiFetch<CalendarioEquipo>(
    `/api/reservation/equipos/${equipoId}/calendario?${params}`
  )
}
