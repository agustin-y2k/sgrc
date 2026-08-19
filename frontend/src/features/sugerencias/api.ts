import type { RespuestaLista, Sugerencia, TipoDeMensaje } from "@/features/sugerencias/types"
import { apiFetch } from "@/lib/api-client"

/** La inyecta Vite desde package.json (ver vite.config.ts). */
declare const __VERSION__: string

/** Escribe en el buzón. */
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
