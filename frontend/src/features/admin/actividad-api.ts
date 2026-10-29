import { apiFetch } from "@/lib/api-client"
export type Metricas = {
  reservasSolicitadas: number
  reservasAtendidas: number
  reservasNoRetiradas: number
  reservasCanceladas: number
  prestamosConReserva: number
  prestamosSinReserva: number
  prestamosOrigenNoDeterminado: number
  prestamosAbiertos: number
  prestamosDevueltos: number
}
export type Actividad = {
  version: number
  anio: number
  disponible: boolean
  historico: boolean
  totales: Metricas
  equipos: (Metricas & { equipoId: string; etiqueta: string })[]
  responsables: (Metricas & { identidad: string; nombre: string; usuarioId?: string })[]
}
export function consultarActividad(cicloId: string, desde?: string, hasta?: string) {
  const params = new URLSearchParams()
  if (desde) params.set("desde", desde)
  if (hasta) params.set("hasta", hasta)
  return apiFetch<Actividad>(
    `/api/ciclos/${cicloId}/actividad${params.size ? "?" + params : ""}`
  )
}
