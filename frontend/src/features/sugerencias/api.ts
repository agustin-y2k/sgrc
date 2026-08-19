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

export function misSugerencias() {
  return apiFetch<RespuestaLista<Sugerencia>>("/api/sugerencias/mias")
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

export function listar(soloAbiertas: boolean) {
  return apiFetch<RespuestaLista<Sugerencia>>(
    `/api/sugerencias/${soloAbiertas ? "?abiertas=true" : ""}`
  )
}

/** Da el tema por terminado. Contestar ya no cierra: son dos actos. */
export function resolver(id: string) {
  return apiFetch<Sugerencia>(`/api/sugerencias/${id}/resolver`, { method: "POST" })
}
