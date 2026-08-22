import type {
  ActualizarMisDatosRequest,
  ActualizarMisDatosResponse,
} from "@/features/auth/types"
import type { PedidoDeMateria, RespuestaLista } from "@/features/perfil/types"
import { apiFetch } from "@/lib/api-client"
import { getToken } from "@/lib/token-store"

// ── Mis datos ─────────────────────────────────────────────────────────

/**
 * Cambia el propio nombre y apellido. Devuelve un token nuevo: el actual
 * lleva el nombre viejo en los claims (ver ActualizarMisDatos en
 * internal/auth/application/service_perfil.go).
 */
export function actualizarMisDatos(req: ActualizarMisDatosRequest) {
  return apiFetch<ActualizarMisDatosResponse>("/api/auth/mi-perfil", {
    method: "PATCH",
    body: req,
  })
}

// ── Foto de perfil ────────────────────────────────────────────────────

/**
 * Descarga la foto de alguien, autenticada.
 *
 * Va por fetch y devuelve el Blob en vez de una URL para un <img src>, y ese
 * es el punto: el token vive en localStorage, y una etiqueta <img> no manda
 * ningún header. Con la URL pelada el pedido llegaba sin autenticar a una
 * ruta que exige estarlo, así que TODA foto daba 401 y el avatar caía a las
 * iniciales — silenciosamente, porque el fallback se ve intencional.
 *
 * Tampoco se resuelve mandando el token en la query: nginx registra la URL
 * completa en su log de acceso, así que cada carga de página dejaría un JWT
 * escrito en disco.
 *
 * `version` no viaja al servidor: solo entra en la clave de caché de quien
 * llama, para que cambiar la foto propia no siga mostrando la vieja.
 */
export async function descargarFoto(usuarioId: string): Promise<Blob> {
  const token = getToken()
  const respuesta = await fetch(
    `${import.meta.env.VITE_API_URL ?? ""}/api/auth/usuarios/${usuarioId}/foto`,
    { headers: token ? { Authorization: `Bearer ${token}` } : undefined }
  )
  if (!respuesta.ok) {
    // 404 es el caso normal —esa persona no subió foto— y no un error que
    // haya que mostrar: quien llama dibuja las iniciales.
    throw new Error(`sin foto (${respuesta.status})`)
  }
  return await respuesta.blob()
}

/** Sube la foto propia. */
export async function subirMiFoto(archivo: Blob) {
  const cuerpo = new FormData()
  cuerpo.append("foto", archivo, "perfil.webp")

  const token = getToken()
  const respuesta = await fetch(
    `${import.meta.env.VITE_API_URL ?? ""}/api/auth/mi-foto`,
    {
      method: "PUT",
      headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      body: cuerpo,
    }
  )
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
export function pedirMateriaNueva(
  materiaSolicitada: string,
  cursoSolicitado: string,
  motivo: string
) {
  return apiFetch<PedidoDeMateria>("/api/academic/pedidos-de-materia", {
    method: "POST",
    body: { materiaSolicitada, cursoSolicitado, motivo },
  })
}

export function misPedidos() {
  return apiFetch<RespuestaLista<PedidoDeMateria>>(
    "/api/academic/pedidos-de-materia/mios"
  )
}

// ── Del lado del Admin ────────────────────────────────────────────────

export function listarPedidos(soloPendientes: boolean) {
  const query = soloPendientes ? "?pendientes=true" : ""
  return apiFetch<RespuestaLista<PedidoDeMateria>>(
    `/api/academic/pedidos-de-materia${query}`
  )
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
