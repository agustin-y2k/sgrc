import type {
  RespuestaLista,
  Sugerencia,
  TipoDeMensaje,
} from "@/features/sugerencias/types"
import { apiFetch } from "@/lib/api-client"

/** La inyecta Vite desde package.json (ver vite.config.ts). */
declare const __VERSION__: string

/** Abre una conversación nueva con el equipo de administración. */
export function escribir(
  tipo: TipoDeMensaje,
  asunto: string,
  texto: string,
  pantalla: string
) {
  return apiFetch<Sugerencia>("/api/sugerencias/", {
    method: "POST",
    body: { tipo, asunto, texto, pantalla, version: __VERSION__ },
  })
}

export function misSugerencias(pagina = 1) {
  const query = pagina > 1 ? `?page=${pagina}` : ""
  return apiFetch<RespuestaLista<Sugerencia>>(`/api/mis-sugerencias${query}`)
}

/**
 * Escribe en un hilo. Lo usan los dos lados: el servidor sabe por el token si
 * el mensaje es de administración o de quien preguntó.
 */
export function responder(id: string, texto: string) {
  return apiFetch<Sugerencia>(`/api/sugerencias/${id}/mensajes`, {
    method: "POST",
    body: { texto },
  })
}

// ── Del lado del Admin ────────────────────────────────────────────────

/**
 * El buzón entero, de a 50 por página. La página se manda solo a partir de la
 * segunda: el backend ya devuelve la primera cuando no viene, y agregarla
 * cambiaría la URL de la consulta más común sin cambiar la respuesta.
 */
export function listar(soloAbiertas: boolean, pagina = 1) {
  const params = new URLSearchParams()
  if (soloAbiertas) params.set("abiertas", "true")
  if (pagina > 1) params.set("page", String(pagina))
  const query = params.toString()
  return apiFetch<RespuestaLista<Sugerencia>>(
    `/api/sugerencias/${query ? `?${query}` : ""}`
  )
}

/** Da el tema por terminado. Contestar ya no cierra: son dos actos. */
export function resolver(id: string) {
  return apiFetch<Sugerencia>(`/api/sugerencias/${id}/resolver`, { method: "POST" })
}
