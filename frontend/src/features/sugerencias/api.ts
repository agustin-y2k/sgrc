import type { RespuestaLista, Sugerencia, TipoDeMensaje } from "@/features/sugerencias/types"
import { apiFetch } from "@/lib/api-client"

/**
 * La inyecta Vite desde package.json (ver vite.config.ts). Se declara acá
 * igual que en PieDeAutoria: es una constante global de compilación, no un
 * módulo que se pueda importar.
 */
declare const __VERSION__: string

/**
 * Escribe en el buzón.
 *
 * `pantalla` la manda la interfaz, no la persona: sin saber desde dónde se
 * escribió, un "no me deja" obliga a ir a buscar a quien lo escribió para
 * preguntarle qué estaba haciendo.
 */
export function escribir(tipo: TipoDeMensaje, texto: string, pantalla: string) {
  return apiFetch<Sugerencia>("/api/sugerencias/", {
    method: "POST",
    body: { tipo, texto, pantalla, version: __VERSION__ },
  })
}

export function misSugerencias() {
  return apiFetch<RespuestaLista<Sugerencia>>("/api/sugerencias/mias")
}

// ── Del lado del Admin ────────────────────────────────────────────────

export function listar(soloAbiertas: boolean) {
  return apiFetch<RespuestaLista<Sugerencia>>(
    `/api/sugerencias/${soloAbiertas ? "?abiertas=true" : ""}`
  )
}

export function responder(id: string, respuesta: string) {
  return apiFetch<Sugerencia>(`/api/sugerencias/${id}/responder`, {
    method: "POST",
    body: { respuesta },
  })
}
