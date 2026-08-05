import { apiFetch } from "@/lib/api-client"
import type { CalendarioPC } from "@/features/calendario/types"

// RF-04.4 — los bloques ocupados de una PC en un rango de fechas, con
// docente y materia ya resueltos por el backend.
export function calendarioDePC(pcId: string, desde: string, hasta: string) {
  const params = new URLSearchParams({ desde, hasta })
  return apiFetch<CalendarioPC>(`/api/reservation/pcs/${pcId}/calendario?${params}`)
}
