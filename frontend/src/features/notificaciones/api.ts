import type {
  CategoriaEmail,
  EstadoNotificacion,
  Notificacion,
  PreferenciaEmail,
  RespuestaLista,
  RespuestaPaginada,
} from "@/features/notificaciones/types"
import { apiFetch } from "@/lib/api-client"

/** Las notificaciones del usuario autenticado. */
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

/**
 * Sacar un aviso de la lista. Falla con 409 si todavía no se leyó: un aviso sin
 * leer es algo que no pasó por los ojos de nadie, y borrarlo haría desaparecer
 * la tarea sin que nadie sepa que existió.
 */
export function borrar(id: string) {
  return apiFetch<void>(`/api/notifications/${id}`, { method: "DELETE" })
}

/** Vaciar de una vez lo ya leído. Devuelve cuántos se fueron. */
export function borrarLeidas() {
  return apiFetch<{ borradas: number }>("/api/notifications/leidas", {
    method: "DELETE",
  })
}

/** Las categorías que le corresponden a quien pregunta (RF-05.13). */
export function listarPreferenciasEmail() {
  return apiFetch<RespuestaLista<PreferenciaEmail>>(
    "/api/notifications/preferencias-email"
  )
}

/**
 * Reemplaza la selección entera: lo que no va en la lista queda apagado. Las
 * de la cuenta no se mandan nunca — salen siempre y el backend las rechaza.
 */
export function guardarPreferenciasEmail(categorias: CategoriaEmail[]) {
  return apiFetch<RespuestaLista<PreferenciaEmail>>(
    "/api/notifications/preferencias-email",
    {
      method: "PUT",
      body: { categorias },
    }
  )
}
