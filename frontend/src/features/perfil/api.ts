import type { PedidoDeMateria, RespuestaLista } from "@/features/perfil/types"
import { apiFetch } from "@/lib/api-client"
import { getToken } from "@/lib/token-store"

// ── Foto de perfil ────────────────────────────────────────────────────

/**
 * La URL de la foto de alguien.
 *
 * Lleva `v` con la fecha de actualización para que, al cambiarla, el
 * navegador no siga mostrando la vieja de su caché: el servidor manda
 * `private, max-age=300`, así que sin esto una foto recién subida tardaba
 * hasta cinco minutos en verse.
 */
export function urlDeFoto(usuarioId: string, version?: string) {
  const base = `${import.meta.env.VITE_API_URL ?? ""}/api/auth/usuarios/${usuarioId}/foto`
  return version ? `${base}?v=${encodeURIComponent(version)}` : base
}

/**
 * Sube la foto propia.
 *
 * No usa apiFetch porque esto va como multipart y aquel serializa a JSON;
 * el token se agrega igual, a mano.
 */
export async function subirMiFoto(archivo: Blob) {
  const cuerpo = new FormData()
  cuerpo.append("foto", archivo, "perfil.webp")

  const token = getToken()
  const respuesta = await fetch(`${import.meta.env.VITE_API_URL ?? ""}/api/auth/mi-foto`, {
    method: "PUT",
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
    body: cuerpo,
  })
  if (!respuesta.ok) {
    throw new Error((await respuesta.text()).trim() || "no se pudo guardar la foto")
  }
  return (await respuesta.json()) as { tipo: string; actualizadaEn: string }
}

export function eliminarMiFoto() {
  return apiFetch<void>("/api/auth/mi-foto", { method: "DELETE" })
}

// ── Pedidos para dictar una materia ───────────────────────────────────

/** Elegida de la lista. */
export function pedirMateriaExistente(materiaId: string, motivo: string) {
  return apiFetch<PedidoDeMateria>("/api/academic/pedidos-de-materia", {
    method: "POST",
    body: { materiaId, motivo },
  })
}

/** Escrita a mano porque todavía no existe (igual que en el registro). */
export function pedirMateriaNueva(materiaSolicitada: string, cursoSolicitado: string, motivo: string) {
  return apiFetch<PedidoDeMateria>("/api/academic/pedidos-de-materia", {
    method: "POST",
    body: { materiaSolicitada, cursoSolicitado, motivo },
  })
}

export function misPedidos() {
  return apiFetch<RespuestaLista<PedidoDeMateria>>("/api/academic/pedidos-de-materia/mios")
}

// ── Del lado del Admin ────────────────────────────────────────────────

export function listarPedidos(soloPendientes: boolean) {
  const query = soloPendientes ? "?pendientes=true" : ""
  return apiFetch<RespuestaLista<PedidoDeMateria>>(`/api/academic/pedidos-de-materia${query}`)
}

export function resolverPedido(
  id: string,
  datos: { aprobar: boolean; respuesta: string; cursoId?: string; rol?: string }
) {
  return apiFetch<PedidoDeMateria>(`/api/academic/pedidos-de-materia/${id}/resolver`, {
    method: "POST",
    body: datos,
  })
}
