import { apiFetch } from "@/lib/api-client"
import type {
  CrearAdminRequest,
  Estado,
  ListarUsuariosResponse,
  Rol,
} from "@/features/auth/types"
import type {
  AltaMasivaLicencias,
  Carro,
  Incidencia,
  Licencia,
  Equipo,
  RenovacionLicencias,
  RespuestaLista,
  VencimientoDeclarado,
} from "@/features/inventory/types"
import type {
  HistoricoUsoDocente,
  HistoricoUsoEquipo,
  ResultadoCascada,
  ResumenIncidenciasCarro,
  ResumenIncidenciasEquipo,
  ResumenUsoDocente,
  ResumenUsoEquipo,
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

export function crearEquipoDeCarro(
  carroId: string,
  req: {
    identificador: number
    numeroSerie: string
    freezado: boolean
    cpu?: string
    ram?: string
    sistemaOperativo?: string
    softwareInstalado?: string
  }
) {
  return apiFetch<Equipo>(`/api/inventory/carros/${carroId}/equipos`, {
    method: "POST",
    body: req,
  })
}

/**
 * RF-03.15 — dar de alta algo prestable que no es una computadora de un
 * carro.
 *
 * `reservable` separa el proyector de los cargadores: solo lo reservable
 * aparece en la lista de equipos libres cuando un docente va a reservar.
 */
export function crearEquipoSuelto(req: {
  tipo: string
  nombre: string
  reservable: boolean
}) {
  return apiFetch<Equipo>("/api/inventory/equipos/sueltos", { method: "POST", body: req })
}

export function editarEquipo(
  id: string,
  req: {
    carroId?: string
    freezado?: boolean
    cpu?: string
    ram?: string
    sistemaOperativo?: string
    softwareInstalado?: string
    tipo?: string
    nombre?: string
    reservable?: boolean
  }
) {
  return apiFetch<void>(`/api/inventory/equipos/${id}`, { method: "PATCH", body: req })
}

/**
 * RF-03.8 — pasar un equipo a EN_MANTENIMIENTO o FUERA_DE_SERVICIO cancela en
 * cascada sus reservas futuras. El motivo es opcional; si no se manda, el
 * backend arma uno por defecto para la notificación al docente.
 */
export function cambiarEstadoEquipo(id: string, estado: Equipo["estado"], motivo?: string) {
  return apiFetch<ResultadoCascada>(`/api/inventory/equipos/${id}/estado`, {
    method: "PATCH",
    body: { estado, motivo },
  })
}

/** RF-03.9 — dar de baja dispara la misma cascada que RF-03.8. */
export function darDeBajaEquipo(id: string) {
  return apiFetch<ResultadoCascada>(`/api/inventory/equipos/${id}`, { method: "DELETE" })
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

/** RF-06.1 — uso por Equipo del ciclo, filtrable por rango de fechas. */
export function reporteUsoEquipos(cicloId: string, desde?: string, hasta?: string) {
  return apiFetch<RespuestaLista<ResumenUsoEquipo>>(
    conRango(`/api/reporting/ciclos/${cicloId}/uso-equipos`, desde, hasta)
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
export function historicoUsoEquipos(anio: number) {
  return apiFetch<RespuestaLista<HistoricoUsoEquipo>>(
    `/api/reporting/historico/${anio}/uso-equipos`
  )
}

export function historicoUsoDocentes(anio: number) {
  return apiFetch<RespuestaLista<HistoricoUsoDocente>>(
    `/api/reporting/historico/${anio}/uso-docentes`
  )
}

/** RF-06.3 — no depende del ciclo: Incidencia sobrevive al archivado. */
export function reporteIncidenciasPorEquipo(desde?: string, hasta?: string) {
  return apiFetch<RespuestaLista<ResumenIncidenciasEquipo>>(
    conRango("/api/reporting/incidencias/equipos", desde, hasta)
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

// ── Licencias de software (RF-03.11 a RF-03.14) ───────────────────────
//
// Todo solo-Admin, incluidas las lecturas: el docente elige Equipo por
// `softwareInstalado`, que ya ve en la pantalla de reserva; cuándo vence
// una licencia es trabajo administrativo.

export function listarLicencias() {
  return apiFetch<RespuestaLista<Licencia>>("/api/inventory/licencias")
}

export function listarLicenciasDeEquipo(equipoId: string) {
  return apiFetch<RespuestaLista<Licencia>>(`/api/inventory/equipos/${equipoId}/licencias`)
}

/**
 * Alta de la MISMA licencia en varias equipos de una vez: el caso real es
 * "AutoCAD, 30 días, en estas ocho máquinas".
 *
 * Responde 201 aunque algún equipo ya la tuviera; cuáles se saltearon viene en
 * `equiposQueYaLaTenian`. Eso hace que reintentar el mismo request sea seguro:
 * completa lo que falta sin duplicar lo que ya entró.
 */
export function crearLicencias(req: {
  equipoIds: string[]
  nombre: string
  diasDuracion: number
  diasAviso?: number
} & VencimientoDeclarado) {
  return apiFetch<AltaMasivaLicencias>("/api/inventory/licencias", {
    method: "POST",
    body: req,
  })
}

/**
 * `renovadaEl` ausente significa "hoy", que es el botón que se aprieta el
 * 99% de las veces. Con fecha es el caso del olvido: se renovó el martes y
 * se carga el jueves.
 */
export function renovarLicencias(req: { licenciaIds: string[]; renovadaEl?: string }) {
  return apiFetch<RenovacionLicencias>("/api/inventory/licencias/renovar", {
    method: "POST",
    body: req,
  })
}

/**
 * El "editar el contador en cualquier momento": corregir la fecha, cambiar
 * la duración de 30 a 60 días, o cargar el vencimiento de una licencia que
 * se dio de alta sin él.
 *
 * Cambiar `diasDuracion` NO mueve el vencimiento vigente — eso se pide
 * aparte, mandando además `renovadaEl` con la última renovación conocida.
 */
export function editarLicencia(
  id: string,
  req: {
    nombre?: string
    diasDuracion?: number
    diasAviso?: number
  } & VencimientoDeclarado
) {
  return apiFetch<void>(`/api/inventory/licencias/${id}`, { method: "PATCH", body: req })
}

export function borrarLicencia(id: string) {
  return apiFetch<void>(`/api/inventory/licencias/${id}`, { method: "DELETE" })
}
