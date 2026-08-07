import { apiFetch } from "@/lib/api-client"
import type {
  CrearAdminRequest,
  Estado,
  ListarUsuariosResponse,
  Rol,
} from "@/features/auth/types"
import type { Carro, Incidencia, PC, RespuestaLista } from "@/features/inventory/types"
import type {
  HistoricoUsoDocente,
  HistoricoUsoPC,
  ResumenIncidenciasCarro,
  ResumenIncidenciasPC,
  ResumenUsoDocente,
  ResumenUsoPC,
} from "@/features/admin/types"

// ── Usuarios (RF-01.x / RF-02.x) ──────────────────────────────────────

export function listarUsuarios(filtros?: { estado?: Estado; rol?: Rol; page?: number }) {
  const params = new URLSearchParams()
  if (filtros?.estado) params.set("estado", filtros.estado)
  if (filtros?.rol) params.set("rol", filtros.rol)
  if (filtros?.page && filtros.page > 1) params.set("page", String(filtros.page))
  const query = params.toString()
  return apiFetch<ListarUsuariosResponse>(`/api/auth/usuarios${query ? `?${query}` : ""}`)
}

export function cambiarEstadoUsuario(id: string, estado: Estado) {
  return apiFetch<void>(`/api/auth/usuarios/${id}/estado`, {
    method: "PATCH",
    body: { estado },
  })
}

/** RF-01.6 — devuelve la contraseña temporal para comunicársela a mano. */
export function resetearPassword(id: string) {
  return apiFetch<{ passwordTemporal: string }>(
    `/api/auth/usuarios/${id}/reset-password`,
    { method: "POST" }
  )
}

/** RF-01.9 — hard delete, permitido desde BAJA o RECHAZADA. Libera el email. */
export function eliminarUsuario(id: string) {
  return apiFetch<void>(`/api/auth/usuarios/${id}`, { method: "DELETE" })
}

/**
 * Le da rol ADMIN a un docente ya aprobado.
 *
 * El cambio tiene efecto en el request siguiente, sin volver a iniciar
 * sesión: el backend lee el rol de la base en cada pedido.
 */
export function promoverAAdmin(id: string) {
  return apiFetch<void>(`/api/auth/usuarios/${id}/promover-a-admin`, { method: "POST" })
}

/**
 * La inversa: le quita el rol ADMIN a un Admin y lo deja como docente, sin
 * cerrarle la cuenta. Conserva materias y reservas.
 *
 * El backend rechaza dos casos con 409 y el mensaje ya explica cuál es, así
 * que alcanza con mostrarlo: degradar al último Admin activo (RF-01.8) y
 * degradarse a uno mismo.
 */
export function degradarADocente(id: string) {
  return apiFetch<void>(`/api/auth/usuarios/${id}/degradar-a-docente`, { method: "POST" })
}

/** RF-01.4 — un Admin puede crear otros Admin (quedan auto-aprobados). */
/** RF-01.4 — el Admin nuevo queda APROBADA, sin pasar por PENDIENTE. */
export function crearAdmin(req: CrearAdminRequest) {
  return apiFetch<void>("/api/auth/admins", { method: "POST", body: req })
}

// ── Inventario (RF-03.x) ──────────────────────────────────────────────

export function crearCarro(req: { nombre: string; descripcion?: string }) {
  return apiFetch<Carro>("/api/inventory/carros", { method: "POST", body: req })
}

export function editarCarro(id: string, req: { nombre?: string; descripcion?: string }) {
  return apiFetch<void>(`/api/inventory/carros/${id}`, { method: "PATCH", body: req })
}

export function crearPC(
  carroId: string,
  req: {
    identificador: number
    numeroSerie: number
    freezado: boolean
    cpu?: string
    ram?: string
    sistemaOperativo?: string
    softwareInstalado?: string
  }
) {
  return apiFetch<PC>(`/api/inventory/carros/${carroId}/pcs`, {
    method: "POST",
    body: req,
  })
}

export function editarPC(
  id: string,
  req: {
    carroId?: string
    freezado?: boolean
    cpu?: string
    ram?: string
    sistemaOperativo?: string
    softwareInstalado?: string
  }
) {
  return apiFetch<void>(`/api/inventory/pcs/${id}`, { method: "PATCH", body: req })
}

/**
 * RF-03.8 — pasar una PC a EN_MANTENIMIENTO o FUERA_DE_SERVICIO cancela en
 * cascada sus reservas futuras. El motivo es opcional; si no se manda, el
 * backend arma uno por defecto para la notificación al docente.
 */
export function cambiarEstadoPC(id: string, estado: PC["estado"], motivo?: string) {
  return apiFetch<void>(`/api/inventory/pcs/${id}/estado`, {
    method: "PATCH",
    body: { estado, motivo },
  })
}

/** RF-03.9 — dar de baja dispara la misma cascada que RF-03.8. */
export function darDeBajaPC(id: string) {
  return apiFetch<void>(`/api/inventory/pcs/${id}`, { method: "DELETE" })
}

// Listar y reportar incidencias viven en features/inventory/api.ts: las
// puede hacer cualquier usuario autenticado. Editarlas es solo de Admin.
export function editarIncidencia(
  id: string,
  req: { estado?: Incidencia["estado"]; marcarEnviadaDGE?: boolean }
) {
  return apiFetch<void>(`/api/inventory/incidencias/${id}`, {
    method: "PATCH",
    body: { estado: req.estado, marcarEnviadaDGE: req.marcarEnviadaDGE ?? false },
  })
}

// ── Reportes (RF-06.x) ────────────────────────────────────────────────

function conRango(base: string, desde?: string, hasta?: string) {
  const params = new URLSearchParams()
  if (desde) params.set("desde", desde)
  if (hasta) params.set("hasta", hasta)
  const query = params.toString()
  return query ? `${base}?${query}` : base
}

/** RF-06.1 — uso por PC del ciclo, filtrable por rango de fechas. */
export function reporteUsoPCs(cicloId: string, desde?: string, hasta?: string) {
  return apiFetch<RespuestaLista<ResumenUsoPC>>(
    conRango(`/api/reporting/ciclos/${cicloId}/uso-pcs`, desde, hasta)
  )
}

/** RF-06.2 */
export function reporteUsoDocentes(cicloId: string, desde?: string, hasta?: string) {
  return apiFetch<RespuestaLista<ResumenUsoDocente>>(
    conRango(`/api/reporting/ciclos/${cicloId}/uso-docentes`, desde, hasta)
  )
}

/**
 * RF-06.4 — el snapshot anual, que es lo único que queda de un ciclo
 * archivado: sus reservas se borran físicamente al archivarlo (RF-02.4).
 *
 * Va por año y no por ciclo porque el snapshot se guarda bajo el año, no
 * bajo el ID del ciclo — que puede no existir más. No admite filtro por
 * rango de fechas: los números ya vienen agregados.
 */
export function historicoUsoPCs(anio: number) {
  return apiFetch<RespuestaLista<HistoricoUsoPC>>(
    `/api/reporting/historico/${anio}/uso-pcs`
  )
}

export function historicoUsoDocentes(anio: number) {
  return apiFetch<RespuestaLista<HistoricoUsoDocente>>(
    `/api/reporting/historico/${anio}/uso-docentes`
  )
}

/** RF-06.3 — no depende del ciclo: Incidencia sobrevive al archivado. */
export function reporteIncidenciasPorPC(desde?: string, hasta?: string) {
  return apiFetch<RespuestaLista<ResumenIncidenciasPC>>(
    conRango("/api/reporting/incidencias/pcs", desde, hasta)
  )
}

export function reporteIncidenciasPorCarro(desde?: string, hasta?: string) {
  return apiFetch<RespuestaLista<ResumenIncidenciasCarro>>(
    conRango("/api/reporting/incidencias/carros", desde, hasta)
  )
}

export function listarCiclos() {
  return apiFetch<
    RespuestaLista<{ id: string; anio: number; activo: boolean; archivado: boolean }>
  >("/api/academic/ciclos")
}
