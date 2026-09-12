import type {
  FiltroDeAuditoria,
  OpcionesDeAuditoria,
  RespuestaAuditoria,
} from "@/features/auditoria/types"
import { apiFetch } from "@/lib/api-client"

/**
 * El registro de auditoría, de lo más nuevo a lo más viejo (solo Admin).
 *
 * Los filtros vacíos NO se mandan: el backend desactiva un filtro cuando no
 * viene, y mandarlo vacío buscaría la cadena vacía.
 */
export function listarAuditoria(filtro: FiltroDeAuditoria, pagina: number) {
  const params = new URLSearchParams()
  for (const [clave, valor] of Object.entries(filtro)) {
    if (valor) params.set(clave, valor)
  }
  if (pagina > 1) params.set("page", String(pagina))

  const query = params.toString()
  return apiFetch<RespuestaAuditoria>(`/api/auditoria/${query ? `?${query}` : ""}`)
}

/** Qué acciones y entidades existen hoy en el registro (solo Admin). */
export function opcionesDeAuditoria() {
  return apiFetch<OpcionesDeAuditoria>("/api/auditoria/opciones")
}
