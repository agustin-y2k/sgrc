import type {
  EstadoNotificacion,
  Notificacion,
  RespuestaPaginada,
} from "@/features/notificaciones/types"
import { apiFetch } from "@/lib/api-client"

/**
 * Las notificaciones del usuario autenticado. El backend siempre filtra
 * por el `sub` del token, así que no hay forma de pedir las de otro.
 */
export function listarNotificaciones(estado?: EstadoNotificacion, page?: number) {
  const params = new URLSearchParams()
  if (estado) params.set("estado", estado)
  if (page && page > 1) params.set("page", String(page))
  const query = params.toString()
  return apiFetch<RespuestaPaginada<Notificacion>>(
    `/api/notifications/${query ? `?${query}` : ""}`
  )
}

export function marcarLeida(id: string) {
  return apiFetch<void>(`/api/notifications/${id}/leida`, { method: "PATCH" })
}

export function marcarTodasLeidas() {
  return apiFetch<void>("/api/notifications/leer-todas", { method: "POST" })
}
